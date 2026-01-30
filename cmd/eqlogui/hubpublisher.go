package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type HubDamageEvent struct {
	TsUnixMs int64  `json:"tsUnixMs"`
	Actor    string `json:"actor"`
	Target   string `json:"target"`
	Kind     string `json:"kind"` // "melee" | "nonmelee"
	Verb     string `json:"verb"`
	Amount   int64  `json:"amount"`
	Crit     bool   `json:"crit"`
}

type hubPublishBatchRequest struct {
	PublisherID  string           `json:"publisherId"`
	SentAtUnixMs int64            `json:"sentAtUnixMs"`
	Events       []HubDamageEvent `json:"events"`
}

type hubPublisherConfig struct {
	HubURL      string
	RoomID      string
	RoomToken   string
	PublisherID string
}

type hubPublisherStatus struct {
	enabled                 bool
	lastError               string
	sentEvents              int64
	droppedNonPcActorEvents int64
}

type hubPublisher struct {
	mu sync.Mutex

	cfg hubPublisherConfig
	st  hubPublisherStatus

	// bounded ring buffer
	head int
	size int
	buf  []HubDamageEvent

	stopCh chan struct{}
	doneCh chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	httpClient *http.Client

	lastStatsAt time.Time
	actorSeen   map[string]time.Time
}

func newHubPublisher() *hubPublisher {
	return &hubPublisher{
		buf: make([]HubDamageEvent, 4096),
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
		actorSeen: make(map[string]time.Time),
	}
}

func (p *hubPublisher) Configure(cfg hubPublisherConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cfg.HubURL == "" {
		cfg.HubURL = "https://sync.dpslogs.com"
	}
	_, err := url.Parse(cfg.HubURL)
	if err != nil {
		return err
	}
	cfg.HubURL = strings.TrimRight(cfg.HubURL, "/")

	if cfg.PublisherID == "" {
		cfg.PublisherID = "pub-" + randHex(8)
	}

	p.cfg = cfg
	return nil
}

func (p *hubPublisher) Enqueue(ev HubDamageEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.buf) == 0 {
		return
	}
	// drop oldest if full
	if p.size == len(p.buf) {
		p.head = (p.head + 1) % len(p.buf)
		p.size--
	}
	idx := (p.head + p.size) % len(p.buf)
	p.buf[idx] = ev
	p.size++
	if ev.Actor != "" {
		p.actorSeen[ev.Actor] = time.Now()
	}
}

func (p *hubPublisher) NoteDroppedNonPcActor(actor string) {
	p.mu.Lock()
	p.st.droppedNonPcActorEvents++
	p.mu.Unlock()
}

func (p *hubPublisher) Start() error {
	p.mu.Lock()
	if p.st.enabled {
		p.mu.Unlock()
		return nil
	}
	if p.cfg.RoomID == "" {
		p.mu.Unlock()
		return errors.New("roomId required")
	}
	if p.cfg.RoomToken == "" {
		p.mu.Unlock()
		return errors.New("roomToken required")
	}
	if p.cfg.HubURL == "" {
		p.cfg.HubURL = "http://127.0.0.1:8787"
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	p.stopCh = stopCh
	p.doneCh = doneCh
	p.ctx = ctx
	p.cancel = cancel
	p.st.enabled = true
	p.st.lastError = ""
	p.mu.Unlock()

	go p.loop(ctx, stopCh, doneCh)
	return nil
}

func (p *hubPublisher) Stop() {
	p.mu.Lock()
	if !p.st.enabled {
		p.mu.Unlock()
		return
	}
	stopCh := p.stopCh
	doneCh := p.doneCh
	cancel := p.cancel
	p.st.enabled = false
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		<-doneCh
	}

	p.mu.Lock()
	if p.stopCh == stopCh {
		p.stopCh = nil
	}
	if p.doneCh == doneCh {
		p.doneCh = nil
	}
	p.ctx = nil
	p.cancel = nil
	p.mu.Unlock()
}

func (p *hubPublisher) Status() PublishingStatusUI {
	p.mu.Lock()
	enabled := p.st.enabled
	lastError := p.st.lastError
	sent := p.st.sentEvents
	dropped := p.st.droppedNonPcActorEvents
	unique := p.uniqueActorsSeenLocked(60 * time.Second)
	p.mu.Unlock()

	return PublishingStatusUI{Enabled: enabled, LastError: lastError, SentEvents: sent, DroppedNonPcActorEvents: dropped, UniquePcActorsSeenLast60s: unique}
}

func (p *hubPublisher) uniqueActorsSeenLocked(window time.Duration) int64 {
	if window <= 0 {
		window = 60 * time.Second
	}
	cutoff := time.Now().Add(-window)
	unique := int64(0)
	for a, t := range p.actorSeen {
		if t.Before(cutoff) {
			delete(p.actorSeen, a)
			continue
		}
		if a != "" {
			unique++
		}
	}
	return unique
}

func (p *hubPublisher) loop(ctx context.Context, stopCh <-chan struct{}, doneCh chan struct{}) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		close(doneCh)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.flushOnce(ctx, 500)
			p.maybeLogStats()
		case <-stopCh:
			return
		}
	}
}

func (p *hubPublisher) maybeLogStats() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if !p.lastStatsAt.IsZero() && now.Sub(p.lastStatsAt) < 10*time.Second {
		return
	}
	p.lastStatsAt = now

	cutoff := now.Add(-10 * time.Second)
	unique := 0
	for a, t := range p.actorSeen {
		if t.Before(cutoff) {
			delete(p.actorSeen, a)
			continue
		}
		unique++
	}

	log.Printf("hub publisher stats: sentEventsTotal=%d uniqueActorsLast10s=%d", p.st.sentEvents, unique)
}

func (p *hubPublisher) flushOnce(ctx context.Context, maxEvents int) {
	cfg, batch := p.drain(maxEvents)
	if len(batch) == 0 {
		return
	}

	sentAt := time.Now().UnixMilli()
	payload := hubPublishBatchRequest{PublisherID: cfg.PublisherID, SentAtUnixMs: sentAt, Events: batch}

	b, err := json.Marshal(payload)
	if err != nil {
		p.setError(err.Error())
		return
	}

	u := cfg.HubURL + "/v1/rooms/" + url.PathEscape(cfg.RoomID) + "/events"
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		p.setError(err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-EQLog-Token", cfg.RoomToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.setError(err.Error())
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.setError("hub returned " + resp.Status)
		return
	}

	p.mu.Lock()
	p.st.sentEvents += int64(len(batch))
	p.st.lastError = ""
	p.mu.Unlock()
}

func (p *hubPublisher) drain(maxEvents int) (hubPublisherConfig, []HubDamageEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg := p.cfg
	if !p.st.enabled {
		return cfg, nil
	}
	if p.size == 0 {
		return cfg, nil
	}
	if maxEvents <= 0 {
		maxEvents = 500
	}
	if maxEvents > p.size {
		maxEvents = p.size
	}

	out := make([]HubDamageEvent, 0, maxEvents)
	for i := 0; i < maxEvents; i++ {
		idx := (p.head + i) % len(p.buf)
		out = append(out, p.buf[idx])
	}
	p.head = (p.head + maxEvents) % len(p.buf)
	p.size -= maxEvents
	return cfg, out
}

func (p *hubPublisher) setError(msg string) {
	p.mu.Lock()
	p.st.lastError = msg
	p.mu.Unlock()
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
