package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type subscribeConfig struct {
	HubURL  string
	RoomID  string
	Token   string
	MaxKeep int
}

type subscribeStatus struct {
	enabled   bool
	connected bool
	lastError string
}

type remoteBucket struct {
	bucketStartUnixMs int64
	damageByActor     map[string]int64
	totalDamage       int64
	bucketSec         int64
}

type remotePlayersSeries struct {
	mu sync.Mutex

	bucketSec int64
	actors    []string
	buckets   []remoteBucket // newest-first

	connected bool
	lastError string
	updatedAt time.Time
}

type wsHubSubscriber struct {
	mu sync.Mutex

	cfg subscribeConfig
	st  subscribeStatus

	gen    uint64
	ctx    context.Context
	cancel context.CancelFunc
	doneCh chan struct{}

	conn        *websocket.Conn
	reconnectAt time.Time

	series remotePlayersSeries
}

func newHubSubscriber() *wsHubSubscriber {
	return &wsHubSubscriber{cfg: subscribeConfig{HubURL: "https://sync.dpslogs.com", MaxKeep: 100}}
}

func (s *wsHubSubscriber) Configure(cfg subscribeConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.HubURL == "" {
		cfg.HubURL = "https://sync.dpslogs.com"
	}
	_, err := url.Parse(cfg.HubURL)
	if err != nil {
		return err
	}
	cfg.HubURL = strings.TrimRight(cfg.HubURL, "/")
	if cfg.MaxKeep <= 0 {
		cfg.MaxKeep = 100
	}

	s.cfg = cfg
	return nil
}

func (s *wsHubSubscriber) Status() SubscribeStatusUI {
	s.mu.Lock()
	enabled := s.st.enabled
	connected := s.st.connected
	lastError := s.st.lastError
	roomID := s.cfg.RoomID
	reconnectAt := s.reconnectAt
	s.mu.Unlock()

	reconnectInMs := int64(0)
	if enabled && !connected && !reconnectAt.IsZero() {
		d := time.Until(reconnectAt)
		if d < 0 {
			d = 0
		}
		reconnectInMs = d.Milliseconds()
	}
	return SubscribeStatusUI{Enabled: enabled, Connected: connected, LastError: lastError, RoomID: roomID, ReconnectInMs: reconnectInMs}
}

func (s *wsHubSubscriber) Start() error {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()

	if cfg.RoomID == "" {
		return errors.New("roomId required")
	}
	if cfg.Token == "" {
		return errors.New("token required")
	}

	s.mu.Lock()
	// Hard requirement: each Start forces a fresh attempt.
	s.gen++
	gen := s.gen
	prevCancel := s.cancel
	prevDone := s.doneCh
	prevConn := s.conn
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel
	s.doneCh = make(chan struct{})
	s.conn = nil
	s.reconnectAt = time.Time{}
	s.st.enabled = true
	s.st.connected = false
	s.st.lastError = ""
	s.mu.Unlock()

	if prevCancel != nil {
		prevCancel()
	}
	if prevConn != nil {
		_ = prevConn.Close()
	}
	if prevDone != nil {
		select {
		case <-prevDone:
		default:
		}
	}

	go s.manageLoop(gen)
	return nil
}

func (s *wsHubSubscriber) Stop() error {
	s.mu.Lock()
	if !s.st.enabled {
		s.mu.Unlock()
		return nil
	}
	prevCancel := s.cancel
	doneCh := s.doneCh
	c := s.conn
	s.cancel = nil
	s.doneCh = nil
	s.conn = nil
	s.reconnectAt = time.Time{}
	s.st.enabled = false
	s.st.connected = false
	s.mu.Unlock()

	if prevCancel != nil {
		prevCancel()
	}
	if c != nil {
		_ = c.Close()
	}
	if doneCh != nil {
		<-doneCh
	}
	return nil
}

func (s *wsHubSubscriber) manageLoop(gen uint64) {
	defer func() {
		s.mu.Lock()
		if s.gen == gen {
			if s.doneCh != nil {
				close(s.doneCh)
			}
		}
		s.mu.Unlock()
	}()

	// Deterministic jitter per generation; avoids global rand races.
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(gen)))

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		s.mu.Lock()
		if s.gen != gen {
			s.mu.Unlock()
			return
		}
		ctx := s.ctx
		cfg := s.cfg
		enabled := s.st.enabled
		s.mu.Unlock()
		if !enabled {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		wsURL, err := buildHubWSURL(cfg.HubURL, cfg.RoomID, cfg.Token)
		if err != nil {
			s.onDisconnected(gen, "invalid ws url: "+err.Error())
			s.sleepBeforeReconnect(ctx, gen, backoff, rng)
			backoff = nextBackoff(backoff, maxBackoff)
			continue
		}

		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			s.onDisconnected(gen, err.Error())
			s.sleepBeforeReconnect(ctx, gen, backoff, rng)
			backoff = nextBackoff(backoff, maxBackoff)
			continue
		}

		connStart := time.Now()
		s.onConnected(gen, c)

		err = s.readLoop(ctx, gen, c, cfg.MaxKeep)
		_ = c.Close()
		if err != nil && ctx.Err() == nil {
			s.onDisconnected(gen, err.Error())
		} else {
			s.onDisconnected(gen, "")
		}

		if time.Since(connStart) > 10*time.Second {
			backoff = time.Second
		} else {
			backoff = nextBackoff(backoff, maxBackoff)
		}
		if ctx.Err() != nil {
			return
		}
		s.sleepBeforeReconnect(ctx, gen, backoff, rng)
	}
}

func nextBackoff(cur time.Duration, max time.Duration) time.Duration {
	n := cur * 2
	if n > max {
		n = max
	}
	if n < time.Second {
		n = time.Second
	}
	return n
}

func (s *wsHubSubscriber) sleepBeforeReconnect(ctx context.Context, gen uint64, backoff time.Duration, rng *rand.Rand) {
	// +/-20% jitter.
	if backoff <= 0 {
		return
	}
	jitter := 0.2
	f := 1 + ((rng.Float64()*2 - 1) * jitter)
	d := time.Duration(float64(backoff) * f)
	if d < 0 {
		d = 0
	}

	reconnectAt := time.Now().Add(d)

	s.mu.Lock()
	if s.gen == gen && s.st.enabled && !s.st.connected {
		s.reconnectAt = reconnectAt
		s.logStateChangeLocked("reconnecting")
	}
	s.mu.Unlock()

	t := time.NewTimer(d)
	defer func() {
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		return
	case <-t.C:
		return
	}
}

func (s *wsHubSubscriber) onConnected(gen uint64, c *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		_ = c.Close()
		return
	}
	if prev := s.conn; prev != nil {
		_ = prev.Close()
	}
	s.conn = c
	s.reconnectAt = time.Time{}
	wasConnected := s.st.connected
	s.st.connected = true
	if s.st.lastError != "" {
		s.st.lastError = ""
	}
	if !wasConnected {
		s.logStateChangeLocked("connected")
	}
}

func (s *wsHubSubscriber) onDisconnected(gen uint64, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	wasConnected := s.st.connected
	s.st.connected = false
	if errMsg != "" {
		s.st.lastError = errMsg
	}
	s.conn = nil
	if wasConnected {
		s.logStateChangeLocked("disconnected")
	} else if errMsg != "" && s.st.enabled {
		// Only log the first disconnect/error while already disconnected.
		s.logStateChangeLocked("disconnected")
	}
}

func (s *wsHubSubscriber) logStateChangeLocked(state string) {
	// One line per state change attempt; avoid per-message spam.
	room := strings.TrimSpace(s.cfg.RoomID)
	if room == "" {
		room = "(no-room)"
	}
	if state == "connected" {
		log.Printf("subscribe %s: connected", room)
		return
	}
	if state == "reconnecting" {
		if !s.reconnectAt.IsZero() {
			ms := time.Until(s.reconnectAt).Milliseconds()
			if ms < 0 {
				ms = 0
			}
			log.Printf("subscribe %s: reconnecting in %dms", room, ms)
			return
		}
		log.Printf("subscribe %s: reconnecting", room)
		return
	}
	if state == "disconnected" {
		if strings.TrimSpace(s.st.lastError) != "" {
			log.Printf("subscribe %s: disconnected: %s", room, s.st.lastError)
			return
		}
		log.Printf("subscribe %s: disconnected", room)
		return
	}
}

func (s *wsHubSubscriber) GetSeries(maxBuckets int) PlayersSeriesUI {
	if maxBuckets <= 0 {
		maxBuckets = 100
	}

	s.series.mu.Lock()
	bucketSec := s.series.bucketSec
	actors := append([]string(nil), s.series.actors...)
	buckets := append([]remoteBucket(nil), s.series.buckets...)
	s.series.mu.Unlock()

	out := PlayersSeriesUI{
		Now:        time.Now().Format(time.RFC3339),
		BucketSec:  bucketSec,
		MaxBuckets: maxBuckets,
		Actors:     actors,
		Buckets:    make([]PlayerBucketUI, 0, len(buckets)),
	}

	if len(buckets) > maxBuckets {
		buckets = buckets[:maxBuckets]
	}
	for _, b := range buckets {
		if b.bucketStartUnixMs <= 0 {
			continue
		}
		out.Buckets = append(out.Buckets, PlayerBucketUI{
			BucketStart:   time.UnixMilli(b.bucketStartUnixMs).Format(time.RFC3339),
			BucketSec:     b.bucketSec,
			DamageByActor: b.damageByActor,
			TotalDamage:   b.totalDamage,
		})
	}

	if out.BucketSec <= 0 {
		out.BucketSec = 5
	}
	return out
}

type bucketSnapshotMsg struct {
	Type      string                 `json:"type"`
	BucketSec int64                  `json:"bucketSec"`
	Actors    []string               `json:"actors"`
	Buckets   []bucketSnapshotBucket `json:"buckets"`
}

type bucketSnapshotBucket struct {
	BucketStartUnixMs int64            `json:"bucketStartUnixMs"`
	TotalDamage       int64            `json:"totalDamage"`
	DamageByActor     map[string]int64 `json:"damageByActor"`
}

type bucketUpdateMsg struct {
	Type              string           `json:"type"`
	BucketSec         int64            `json:"bucketSec"`
	BucketStartUnixMs int64            `json:"bucketStartUnixMs"`
	TotalDamage       int64            `json:"totalDamage"`
	DamageByActor     map[string]int64 `json:"damageByActor"`
}

func (s *wsHubSubscriber) readLoop(ctx context.Context, gen uint64, c *websocket.Conn, maxKeep int) error {
	// Server pings should keep this alive; pong handler extends read deadline.
	readTimeout := 60 * time.Second
	_ = c.SetReadDeadline(time.Now().Add(readTimeout))
	c.SetPongHandler(func(string) error {
		_ = c.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		_, data, err := c.ReadMessage()
		if err != nil {
			return err
		}

		// If this generation is no longer current, drop messages quickly.
		s.mu.Lock()
		stale := s.gen != gen
		s.mu.Unlock()
		if stale {
			return nil
		}

		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &header); err != nil {
			continue
		}

		switch header.Type {
		case "bucket_snapshot":
			var m bucketSnapshotMsg
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			s.onSnapshot(m, maxKeep)
		case "bucket_update":
			var m bucketUpdateMsg
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			s.onUpdate(m, maxKeep)
		default:
		}
	}
}

func (s *wsHubSubscriber) onSnapshot(m bucketSnapshotMsg, maxKeep int) {
	if maxKeep <= 0 {
		maxKeep = 100
	}

	actors := make([]string, 0, len(m.Actors))
	for _, a := range m.Actors {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		actors = append(actors, a)
	}

	buckets := make([]remoteBucket, 0, len(m.Buckets))
	for _, b := range m.Buckets {
		if b.BucketStartUnixMs <= 0 {
			continue
		}
		dba := b.DamageByActor
		if dba == nil {
			dba = map[string]int64{}
		}
		buckets = append(buckets, remoteBucket{
			bucketStartUnixMs: b.BucketStartUnixMs,
			damageByActor:     dba,
			totalDamage:       b.TotalDamage,
			bucketSec:         m.BucketSec,
		})
	}

	// newest-first
	sortBucketsNewestFirst(buckets)
	if len(buckets) > maxKeep {
		buckets = buckets[:maxKeep]
	}

	s.series.mu.Lock()
	s.series.bucketSec = m.BucketSec
	s.series.actors = actors
	s.series.buckets = buckets
	s.series.connected = true
	s.series.lastError = ""
	s.series.updatedAt = time.Now()
	s.series.mu.Unlock()
}

func (s *wsHubSubscriber) onUpdate(m bucketUpdateMsg, maxKeep int) {
	if maxKeep <= 0 {
		maxKeep = 100
	}
	if m.BucketStartUnixMs <= 0 {
		return
	}
	if m.DamageByActor == nil {
		m.DamageByActor = map[string]int64{}
	}

	s.series.mu.Lock()
	defer s.series.mu.Unlock()

	s.series.bucketSec = m.BucketSec

	// update actors list if missing
	actorSet := make(map[string]struct{}, len(s.series.actors))
	for _, a := range s.series.actors {
		actorSet[a] = struct{}{}
	}
	for a := range m.DamageByActor {
		if _, ok := actorSet[a]; !ok {
			s.series.actors = append(s.series.actors, a)
			actorSet[a] = struct{}{}
		}
	}

	// upsert bucket
	idx := -1
	for i := range s.series.buckets {
		if s.series.buckets[i].bucketStartUnixMs == m.BucketStartUnixMs {
			idx = i
			break
		}
	}
	b := remoteBucket{bucketStartUnixMs: m.BucketStartUnixMs, damageByActor: m.DamageByActor, totalDamage: m.TotalDamage, bucketSec: m.BucketSec}
	if idx >= 0 {
		s.series.buckets[idx] = b
	} else {
		s.series.buckets = append([]remoteBucket{b}, s.series.buckets...)
	}

	sortBucketsNewestFirst(s.series.buckets)
	if len(s.series.buckets) > maxKeep {
		s.series.buckets = s.series.buckets[:maxKeep]
	}

	s.series.connected = true
	s.series.lastError = ""
	s.series.updatedAt = time.Now()
}

func (s *wsHubSubscriber) setError(msg string) {
	s.mu.Lock()
	s.st.lastError = msg
	s.mu.Unlock()
}

func buildHubWSURL(hubURL string, roomID string, token string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(hubURL))
	if err != nil {
		return "", err
	}
	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	} else if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", errors.New("unsupported scheme")
	}
	u.Path = "/v1/rooms/" + url.PathEscape(strings.TrimSpace(roomID)) + "/ws"
	q := u.Query()
	q.Set("token", strings.TrimSpace(token))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func sortBucketsNewestFirst(b []remoteBucket) {
	for i := 0; i < len(b); i++ {
		for j := i + 1; j < len(b); j++ {
			if b[j].bucketStartUnixMs > b[i].bucketStartUnixMs {
				b[i], b[j] = b[j], b[i]
			}
		}
	}
}
