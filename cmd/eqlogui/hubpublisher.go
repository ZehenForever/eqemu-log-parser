package main

import (
	"bytes"
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
	enabled    bool
	lastError  string
	sentEvents int64
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

	p.stopCh = make(chan struct{})
	p.doneCh = make(chan struct{})
	p.st.enabled = true
	p.st.lastError = ""
	p.mu.Unlock()

	go p.loop()
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
	p.st.enabled = false
	p.stopCh = nil
	p.doneCh = nil
	p.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		<-doneCh
	}
}

func (p *hubPublisher) Status() PublishingStatusUI {
	p.mu.Lock()
	enabled := p.st.enabled
	lastError := p.st.lastError
	sent := p.st.sentEvents
	p.mu.Unlock()

	return PublishingStatusUI{Enabled: enabled, LastError: lastError, SentEvents: sent}
}

func (p *hubPublisher) loop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		p.mu.Lock()
		if p.doneCh != nil {
			close(p.doneCh)
		}
		p.mu.Unlock()
	}()

	for {
		select {
		case <-ticker.C:
			p.flushOnce(500)
			p.maybeLogStats()
		case <-p.stopCh:
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

func (p *hubPublisher) flushOnce(maxEvents int) {
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
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
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
