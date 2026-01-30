package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/engine"
	"github.com/ZehenForever/eqemu-log-parser/internal/model"
	"github.com/ZehenForever/eqemu-log-parser/internal/parse"
	"github.com/ZehenForever/eqemu-log-parser/internal/tail"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context

	mu sync.RWMutex

	filePath  string
	tailing   bool
	lastHours float64

	playerName string
	pctx       *model.ParseContext
	seg        *engine.EncounterSegmenter
	timeFilter engine.TimeFilter

	tlr    *tail.Tailer
	cancel context.CancelFunc

	includePCTargets bool

	encListCacheAt      time.Time
	encListCacheTTL     time.Duration
	encListCacheLimit   int
	encListCacheIncPC   bool
	encListCacheFile    string
	encListCacheTailing bool
	encListCacheLastH   float64
	encListCache        SnapshotUI

	hub *hubPublisher
	sub *wsHubSubscriber

	config     AppConfig
	configPath string
	configErr  string
}

func NewApp() *App {
	return &App{encListCacheTTL: 250 * time.Millisecond, hub: newHubPublisher(), sub: newHubSubscriber()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfg, p, err := LoadConfig()
	a.config = cfg
	a.configPath = p
	if err != nil {
		a.configErr = err.Error()
	} else {
		a.configErr = ""
	}
}

func (a *App) GetConfigDefaults() ConfigDefaultsUI {
	// Read-only defaults for frontend init.
	cfg := a.config
	if strings.TrimSpace(cfg.Hub.URL) == "" {
		cfg = DefaultConfig()
	}
	return ConfigDefaultsUI{
		HubURL:      cfg.Hub.URL,
		RoomID:      cfg.Hub.RoomID,
		Token:       cfg.Hub.Token,
		ConfigPath:  a.configPath,
		ConfigError: a.configErr,
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.StopPublishing()
	_ = a.StopSubscribe()
	_ = a.Stop()
}

func (a *App) SelectLogFile() string {
	p, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select EverQuest Log File",
	})
	if err != nil {
		return ""
	}
	return p
}

func (a *App) SetIncludePCTargets(include bool) {
	a.mu.Lock()
	a.includePCTargets = include
	a.mu.Unlock()
}

func (a *App) SetLastHours(hours float64) {
	if hours < 0 {
		hours = 0
	}
	a.mu.Lock()
	a.lastHours = hours
	a.mu.Unlock()
}

func (a *App) GetLastHours() float64 {
	a.mu.RLock()
	h := a.lastHours
	a.mu.RUnlock()
	return h
}

func (a *App) Start(path string, startAtEnd bool) error {
	if path == "" {
		return errors.New("empty path")
	}

	a.mu.Lock()
	if a.tailing {
		a.mu.Unlock()
		return errors.New("already tailing")
	}
	playerName, _ := parse.PlayerNameFromLogPath(path)
	a.filePath = path
	a.playerName = playerName
	a.pctx = &model.ParseContext{LocalActorName: playerName}
	a.seg = engine.NewEncounterSegmenter(8*time.Second, playerName)
	lastHours := a.lastHours
	tf := engine.NewTimeFilterLastHours(lastHours, time.Now())
	a.timeFilter = tf
	a.tailing = true
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.mu.Unlock()

	seg := a.seg
	pctx := a.pctx
	if lastHours > 0 {
		f, err := os.Open(path)
		if err != nil {
			cancel()
			a.mu.Lock()
			a.tailing = false
			a.cancel = nil
			a.tlr = nil
			a.mu.Unlock()
			return err
		}
		defer func() { _ = f.Close() }()

		it := parse.ParseFile(f, pctx, time.Local)
		for it.Next() {
			ev := it.Event()
			if !tf.Allow(ev.Timestamp) {
				continue
			}
			if playerName != "" {
				if ev.Actor == "YOU" {
					ev.Actor = playerName
				}
				if ev.Target == "YOU" {
					ev.Target = playerName
				}
			}
			seg.Process(ev)
			a.maybeEnqueueHubDamage(ev, playerName)
		}
		if err := it.Err(); err != nil {
			cancel()
			a.mu.Lock()
			a.tailing = false
			a.cancel = nil
			a.tlr = nil
			a.mu.Unlock()
			return err
		}
	}

	startTailAtEnd := startAtEnd
	if lastHours > 0 {
		startTailAtEnd = true
	}

	tlr, err := tail.NewTailer(path, tail.TailOptions{StartAtEnd: startTailAtEnd})
	if err != nil {
		a.mu.Lock()
		a.tailing = false
		a.cancel = nil
		a.mu.Unlock()
		return err
	}

	a.mu.Lock()
	a.tlr = tlr
	a.mu.Unlock()

	go func() {
		_ = tlr.Run(ctx, func(line string) {
			a.onLine(line)
		})
		a.mu.Lock()
		a.tailing = false
		a.tlr = nil
		a.cancel = nil
		a.mu.Unlock()
	}()

	return nil
}

func (a *App) onLine(line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seg == nil || a.pctx == nil {
		return
	}
	ev, ok := parse.ParseLine(a.pctx, line, time.Local)
	if !ok {
		return
	}
	if !a.timeFilter.Allow(ev.Timestamp) {
		return
	}
	if a.playerName != "" {
		if ev.Actor == "YOU" {
			ev.Actor = a.playerName
		}
		if ev.Target == "YOU" {
			ev.Target = a.playerName
		}
	}
	a.seg.Process(ev)
	a.maybeEnqueueHubDamage(ev, a.playerName)
}

func (a *App) Stop() error {
	a.mu.Lock()
	cancel := a.cancel
	tlr := a.tlr
	a.cancel = nil
	a.tlr = nil
	a.tailing = false
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if tlr != nil {
		_ = tlr.Stop()
	}
	return nil
}

func (a *App) ConfigureHub(hubURL string, roomId string, token string, publisherId string) error {
	a.mu.RLock()
	playerName := a.playerName
	a.mu.RUnlock()

	if publisherId == "" {
		host, _ := os.Hostname()
		host = strings.TrimSpace(host)
		if host == "" {
			host = goruntime.GOOS
		}
		if playerName != "" {
			publisherId = playerName + "@" + host
		} else {
			publisherId = "eqlogui@" + host
		}
	}

	return a.hub.Configure(hubPublisherConfig{HubURL: hubURL, RoomID: roomId, RoomToken: token, PublisherID: publisherId})
}

func (a *App) StartPublishing() error {
	return a.hub.Start()
}

func (a *App) StopPublishing() {
	if a.hub == nil {
		return
	}
	a.hub.Stop()
}

func (a *App) PublishingStatus() PublishingStatusUI {
	if a.hub == nil {
		return PublishingStatusUI{}
	}
	return a.hub.Status()
}

func (a *App) ConfigureSubscribe(hubURL string, roomID string, token string) error {
	if a.sub == nil {
		return errors.New("subscriber not available")
	}
	return a.sub.Configure(subscribeConfig{HubURL: hubURL, RoomID: roomID, Token: token, MaxKeep: 100})
}

func (a *App) StartSubscribe() error {
	if a.sub == nil {
		return errors.New("subscriber not available")
	}
	return a.sub.Start()
}

func (a *App) StopSubscribe() error {
	if a.sub == nil {
		return nil
	}
	return a.sub.Stop()
}

func (a *App) SubscribeStatus() SubscribeStatusUI {
	if a.sub == nil {
		return SubscribeStatusUI{}
	}
	return a.sub.Status()
}

func (a *App) GetRemotePlayersSeries() (PlayersSeriesUI, error) {
	if a.sub == nil {
		return PlayersSeriesUI{Now: time.Now().Format(time.RFC3339), BucketSec: 5, MaxBuckets: 100}, nil
	}
	st := a.sub.Status()
	if !st.Connected {
		return PlayersSeriesUI{Now: time.Now().Format(time.RFC3339), BucketSec: 5, MaxBuckets: 100}, nil
	}
	return a.sub.GetSeries(100), nil
}

type hubRoomsListResponse struct {
	Rooms []hubRoomSummary `json:"rooms"`
}

type hubRoomSummary struct {
	RoomID          string `json:"roomId"`
	LastSeenUnixMs  int64  `json:"lastSeenUnixMs"`
	PublisherCount  int    `json:"publisherCount"`
	SubscriberCount int    `json:"subscriberCount"`
	BucketSec       int    `json:"bucketSec"`
}

func (a *App) ListHubRooms(hubURL string) (RoomListUI, error) {
	hubURL = strings.TrimSpace(hubURL)
	if hubURL == "" {
		hubURL = "https://sync.dpslogs.com"
	}
	u, err := url.Parse(hubURL)
	if err != nil {
		return RoomListUI{}, err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/rooms"
	u.RawQuery = ""

	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return RoomListUI{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return RoomListUI{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RoomListUI{}, errors.New("hub returned " + resp.Status)
	}

	var raw hubRoomsListResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&raw); err != nil {
		return RoomListUI{}, err
	}

	out := RoomListUI{Rooms: make([]RoomSummaryUI, 0, len(raw.Rooms))}
	for _, r := range raw.Rooms {
		lastSeen := ""
		if r.LastSeenUnixMs > 0 {
			lastSeen = time.UnixMilli(r.LastSeenUnixMs).Format(time.RFC3339)
		}
		out.Rooms = append(out.Rooms, RoomSummaryUI{
			RoomID:          r.RoomID,
			LastSeen:        lastSeen,
			PublisherCount:  r.PublisherCount,
			SubscriberCount: r.SubscriberCount,
			BucketSec:       r.BucketSec,
		})
	}
	return out, nil
}

func (a *App) maybeEnqueueHubDamage(ev model.Event, localPlayer string) {
	if a.hub == nil {
		return
	}
	if !ev.AmountKnown {
		return
	}
	actor := ev.Actor
	if actor == "YOU" && localPlayer != "" {
		actor = localPlayer
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return
	}
	if !isPCLikeActorName(actor) {
		a.hub.NoteDroppedNonPcActor(actor)
		return
	}

	kind := ""
	switch ev.Kind {
	case model.KindMeleeDamage:
		kind = "melee"
	case model.KindNonMeleeDamage:
		kind = "nonmelee"
	default:
		return
	}

	a.hub.Enqueue(HubDamageEvent{
		TsUnixMs: ev.Timestamp.UnixMilli(),
		Actor:    actor,
		Target:   ev.Target,
		Kind:     kind,
		Verb:     ev.Verb,
		Amount:   ev.Amount,
		Crit:     ev.Crit,
	})
}

func isPCLikeActorName(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 3 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return false
		}
	}
	r0 := s[0]
	if r0 < 'A' || r0 > 'Z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '\'' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func (a *App) GetSnapshot() SnapshotUI {
	a.mu.RLock()
	seg := a.seg
	filePath := a.filePath
	tailing := a.tailing
	includePCTargets := a.includePCTargets
	lastHours := a.lastHours
	a.mu.RUnlock()

	if seg == nil {
		out := SnapshotToUI(engine.Snapshot{Now: time.Now(), FilePath: filePath, Tailing: tailing})
		out.LastHours = lastHours
		return out
	}
	snap := seg.BuildSnapshot(time.Now(), filePath, tailing, engine.SnapshotOptions{IncludePCTargets: includePCTargets, LimitEncounters: 100, CoalesceTargets: true})
	out := SnapshotToUI(snap)
	out.LastHours = lastHours
	return out
}

func (a *App) GetEncounterList(limit int) (SnapshotUI, error) {
	if limit <= 0 {
		limit = 100
	}

	now := time.Now()

	a.mu.RLock()
	seg := a.seg
	filePath := a.filePath
	tailing := a.tailing
	includePCTargets := a.includePCTargets
	lastHours := a.lastHours
	cacheAt := a.encListCacheAt
	cacheTTL := a.encListCacheTTL
	if !cacheAt.IsZero() && cacheTTL > 0 {
		if now.Sub(cacheAt) <= cacheTTL &&
			a.encListCacheLimit == limit &&
			a.encListCacheIncPC == includePCTargets &&
			a.encListCacheFile == filePath &&
			a.encListCacheTailing == tailing &&
			a.encListCacheLastH == lastHours {
			out := a.encListCache
			a.mu.RUnlock()
			return out, nil
		}
	}
	a.mu.RUnlock()

	if seg == nil {
		out := SnapshotToUISummary(engine.Snapshot{Now: now, FilePath: filePath, Tailing: tailing})
		out.LastHours = lastHours
		a.mu.Lock()
		a.encListCacheAt = now
		a.encListCacheLimit = limit
		a.encListCacheIncPC = includePCTargets
		a.encListCacheFile = filePath
		a.encListCacheTailing = tailing
		a.encListCacheLastH = lastHours
		a.encListCache = out
		a.mu.Unlock()
		return out, nil
	}

	snap := seg.BuildSnapshotSummary(now, filePath, tailing, engine.SnapshotOptions{IncludePCTargets: includePCTargets, LimitEncounters: limit, CoalesceTargets: true})
	out := SnapshotToUISummary(snap)
	out.LastHours = lastHours

	a.mu.Lock()
	a.encListCacheAt = now
	a.encListCacheLimit = limit
	a.encListCacheIncPC = includePCTargets
	a.encListCacheFile = filePath
	a.encListCacheTailing = tailing
	a.encListCacheLastH = lastHours
	a.encListCache = out
	a.mu.Unlock()

	return out, nil
}

func (a *App) GetEncounter(target string) (EncounterViewUI, error) {
	if target == "" {
		return EncounterViewUI{}, errors.New("empty target")
	}

	a.mu.RLock()
	seg := a.seg
	filePath := a.filePath
	tailing := a.tailing
	includePCTargets := a.includePCTargets
	a.mu.RUnlock()

	if seg == nil {
		return EncounterViewUI{}, errors.New("not started")
	}

	view, ok := seg.BuildEncounterView(time.Now(), filePath, tailing, engine.SnapshotOptions{IncludePCTargets: includePCTargets, LimitEncounters: 0, CoalesceTargets: true}, target)
	if !ok {
		return EncounterViewUI{}, errors.New("encounter not found")
	}
	return EncounterViewToUI(view), nil
}

func (a *App) GetDamageBreakdown(encounterId string, actor string) (DamageBreakdownViewUI, error) {
	if encounterId == "" {
		return DamageBreakdownViewUI{}, errors.New("empty encounterId")
	}
	if actor == "" {
		return DamageBreakdownViewUI{}, errors.New("empty actor")
	}

	a.mu.RLock()
	seg := a.seg
	a.mu.RUnlock()

	if seg == nil {
		return DamageBreakdownViewUI{}, errors.New("not started")
	}

	view, ok := seg.GetDamageBreakdown(encounterId, actor)
	if !ok {
		return DamageBreakdownViewUI{}, errors.New("breakdown not found")
	}
	return DamageBreakdownViewToUI(view), nil
}

func (a *App) GetDamageBreakdownByKey(encounterKey string, actor string) (DamageBreakdownViewUI, error) {
	if encounterKey == "" {
		return DamageBreakdownViewUI{}, errors.New("empty encounterKey")
	}
	if actor == "" {
		return DamageBreakdownViewUI{}, errors.New("empty actor")
	}

	a.mu.RLock()
	seg := a.seg
	a.mu.RUnlock()

	if seg == nil {
		return DamageBreakdownViewUI{}, errors.New("not started")
	}

	view, ok := seg.GetDamageBreakdownByKey(encounterKey, actor)
	if !ok {
		return DamageBreakdownViewUI{}, errors.New("breakdown not found")
	}
	return DamageBreakdownViewToUI(view), nil
}

func (a *App) GetPlayersSeries(bucketSec int, maxBuckets int, mode string) (PlayersSeriesUI, error) {
	if bucketSec <= 0 {
		bucketSec = 5
	}
	if maxBuckets <= 0 {
		maxBuckets = 100
	}
	if mode != "me" {
		mode = "all"
	}

	a.mu.RLock()
	seg := a.seg
	a.mu.RUnlock()

	if seg == nil {
		return PlayersSeriesUI{}, errors.New("not started")
	}

	series := seg.BuildPlayersSeries(time.Now(), int64(bucketSec), maxBuckets, mode)
	out := PlayersSeriesUI{
		Now:        series.Now,
		BucketSec:  series.BucketSec,
		MaxBuckets: series.MaxBuckets,
		Actors:     append([]string(nil), series.Actors...),
		Buckets:    make([]PlayerBucketUI, 0, len(series.Buckets)),
	}
	for _, b := range series.Buckets {
		out.Buckets = append(out.Buckets, PlayerBucketUI{
			BucketStart:   b.BucketStart,
			BucketSec:     b.BucketSec,
			DamageByActor: b.DamageByActor,
			TotalDamage:   b.TotalDamage,
		})
	}
	return out, nil
}

func (a *App) GetEncounterByID(encounterId string) (EncounterViewUI, error) {
	parts := strings.Split(encounterId, "|")
	if len(parts) != 3 {
		return EncounterViewUI{}, errors.New("invalid encounterId")
	}
	target := parts[0]
	start, err := time.Parse(time.RFC3339, parts[1])
	if err != nil {
		return EncounterViewUI{}, errors.New("invalid encounterId start")
	}
	end, err := time.Parse(time.RFC3339, parts[2])
	if err != nil {
		return EncounterViewUI{}, errors.New("invalid encounterId end")
	}

	a.mu.RLock()
	seg := a.seg
	filePath := a.filePath
	tailing := a.tailing
	includePCTargets := a.includePCTargets
	a.mu.RUnlock()

	if seg == nil {
		return EncounterViewUI{}, errors.New("not started")
	}

	view, ok := seg.BuildEncounterViewExact(time.Now(), filePath, tailing, engine.SnapshotOptions{IncludePCTargets: includePCTargets, LimitEncounters: 0, CoalesceTargets: true}, target, start, end)
	if !ok {
		return EncounterViewUI{}, errors.New("encounter not found")
	}
	return EncounterViewToUI(view), nil
}

func (a *App) GetEncounterByKey(encounterKey string) (EncounterViewUI, error) {
	parts := strings.Split(encounterKey, "|")
	if len(parts) != 2 {
		return EncounterViewUI{}, errors.New("invalid encounterKey")
	}
	target := parts[0]
	ms, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return EncounterViewUI{}, errors.New("invalid encounterKey start")
	}
	start := time.UnixMilli(ms).In(time.UTC)

	a.mu.RLock()
	seg := a.seg
	filePath := a.filePath
	tailing := a.tailing
	includePCTargets := a.includePCTargets
	a.mu.RUnlock()

	if seg == nil {
		return EncounterViewUI{}, errors.New("not started")
	}

	view, ok := seg.BuildEncounterViewByKey(time.Now(), filePath, tailing, engine.SnapshotOptions{IncludePCTargets: includePCTargets, LimitEncounters: 0, CoalesceTargets: true}, target, start)
	if !ok {
		return EncounterViewUI{}, errors.New("encounter not found")
	}
	return EncounterViewToUI(view), nil
}
