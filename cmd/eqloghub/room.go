package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var errUnauthorized = errors.New("unauthorized")

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
	done chan struct{}
	once sync.Once
}

func newWSClient(conn *websocket.Conn) *wsClient {
	return &wsClient{conn: conn, send: make(chan []byte, 64), done: make(chan struct{})}
}

func (c *wsClient) close() {
	c.once.Do(func() {
		close(c.done)
		close(c.send)
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

func (c *wsClient) enqueueBytes(b []byte) bool {
	select {
	case <-c.done:
		return false
	case c.send <- b:
		return true
	default:
		return false
	}
}

func (c *wsClient) enqueueJSON(v any) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return c.enqueueBytes(b)
}

type Room struct {
	id    string
	token string

	mu   sync.Mutex
	subs map[*wsClient]struct{}

	lastSeenUnixMs         int64
	lastFlushedBucketStart int64

	// PublisherID -> last seen server time (unix ms). TTL-based.
	publishers map[string]int64

	// Dedupe key -> last seen server time (unix ms). TTL-based.
	dedupeLastSeen map[string]int64

	// PublisherID -> smoothed offset state.
	publisherOffsets map[string]*offsetState

	// Rolling buckets.
	bucketSec  int64
	maxBuckets int
	buckets    map[int64]*bucketAgg
	order      []int64
}

func newRoom(id string, token string) *Room {
	return &Room{
		id:                     id,
		token:                  token,
		subs:                   make(map[*wsClient]struct{}),
		lastSeenUnixMs:         0,
		lastFlushedBucketStart: -1,
		publishers:             make(map[string]int64),
		dedupeLastSeen:         make(map[string]int64),
		publisherOffsets:       make(map[string]*offsetState),
		bucketSec:              5,
		maxBuckets:             120,
		buckets:                make(map[int64]*bucketAgg),
		order:                  nil,
	}
}

func (r *Room) authorize(token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.token == "" {
		r.token = token
		return nil
	}
	if r.token != token {
		return errUnauthorized
	}
	return nil
}

func (r *Room) addSub(c *wsClient) {
	r.mu.Lock()
	r.subs[c] = struct{}{}
	r.lastSeenUnixMs = time.Now().UnixMilli()
	r.mu.Unlock()
}

func (r *Room) FlushCompletedBucket(nowUnixMs int64) *BucketUpdateMessage {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.subs) == 0 {
		return nil
	}
	if r.bucketSec <= 0 {
		r.bucketSec = 5
	}
	bucketMs := r.bucketSec * 1000
	curStart := nowUnixMs - (nowUnixMs % bucketMs)
	if curStart == r.lastFlushedBucketStart {
		return nil
	}
	// Publish the most recently completed bucket.
	publishStart := curStart - bucketMs
	if publishStart < 0 {
		publishStart = 0
	}

	r.lastFlushedBucketStart = curStart

	agg := r.buckets[publishStart]
	if agg == nil {
		return &BucketUpdateMessage{
			Type:              "bucket_update",
			BucketSec:         r.bucketSec,
			BucketStartUnixMs: publishStart,
			DamageByActor:     map[string]int64{},
			TotalDamage:       0,
		}
	}

	copyMap := make(map[string]int64, len(agg.damageByActor))
	for a, v := range agg.damageByActor {
		copyMap[a] = v
	}
	return &BucketUpdateMessage{
		Type:              "bucket_update",
		BucketSec:         r.bucketSec,
		BucketStartUnixMs: publishStart,
		DamageByActor:     copyMap,
		TotalDamage:       agg.totalDamage,
	}
}

func (r *Room) removeSub(c *wsClient) {
	r.mu.Lock()
	delete(r.subs, c)
	r.lastSeenUnixMs = time.Now().UnixMilli()
	r.mu.Unlock()
}

func (r *Room) broadcastJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	r.mu.Lock()
	for c := range r.subs {
		if ok := c.enqueueBytes(b); !ok {
			c.close()
			delete(r.subs, c)
		}
	}
	r.mu.Unlock()
}

type offsetState struct {
	init   bool
	offset float64
}

func (s *offsetState) update(sample float64) float64 {
	const alpha = 0.2
	if !s.init {
		s.init = true
		s.offset = sample
		return s.offset
	}
	s.offset = (1-alpha)*s.offset + alpha*sample
	return s.offset
}

type bucketAgg struct {
	bucketStartUnixMs int64
	damageByActor     map[string]int64
	totalDamage       int64
}

func (r *Room) ingestBatch(serverRecvUnixMs int64, req PublishBatchRequest) []*BucketUpdateMessage {
	// Assumes room is already authorized.
	r.lastSeenUnixMs = serverRecvUnixMs
	if req.PublisherID != "" {
		if r.publishers == nil {
			r.publishers = make(map[string]int64)
		}
		r.publishers[req.PublisherID] = serverRecvUnixMs
	}
	if r.bucketSec <= 0 {
		r.bucketSec = 5
	}
	if r.maxBuckets <= 0 {
		r.maxBuckets = 120
	}

	ofsSample := float64(serverRecvUnixMs - req.SentAtUnixMs)
	st := r.publisherOffsets[req.PublisherID]
	if st == nil {
		st = &offsetState{}
		r.publisherOffsets[req.PublisherID] = st
	}
	offsetMs := st.update(ofsSample)

	updatesByBucket := make(map[int64]*BucketUpdateMessage)
	for _, ev := range req.Events {
		// Adjust timestamp by smoothed offset.
		tsAdj := int64(float64(ev.TsUnixMs) + offsetMs)
		tRoundedMs := (tsAdj / 1000) * 1000
		key := dedupeKey(ev, tRoundedMs)
		if last, ok := r.dedupeLastSeen[key]; ok {
			if serverRecvUnixMs-last <= 30_000 {
				continue
			}
		}
		r.dedupeLastSeen[key] = serverRecvUnixMs

		bucketSizeMs := r.bucketSec * 1000
		bucketStart := tsAdj - (tsAdj % bucketSizeMs)
		agg := r.buckets[bucketStart]
		if agg == nil {
			agg = &bucketAgg{bucketStartUnixMs: bucketStart, damageByActor: make(map[string]int64)}
			r.buckets[bucketStart] = agg
			r.order = append(r.order, bucketStart)
			sort.Slice(r.order, func(i, j int) bool { return r.order[i] < r.order[j] })
		}

		agg.damageByActor[ev.Actor] += ev.Amount
		agg.totalDamage += ev.Amount

		msg := updatesByBucket[bucketStart]
		if msg == nil {
			msg = &BucketUpdateMessage{
				Type:              "bucket_update",
				BucketSec:         r.bucketSec,
				BucketStartUnixMs: bucketStart,
				DamageByActor:     make(map[string]int64),
				TotalDamage:       0,
			}
			updatesByBucket[bucketStart] = msg
		}
		msg.DamageByActor[ev.Actor] = agg.damageByActor[ev.Actor]
		msg.TotalDamage = agg.totalDamage
	}

	r.pruneLocked(serverRecvUnixMs)

	if len(updatesByBucket) == 0 {
		return nil
	}
	// Return updates ordered from oldest->newest (deterministic ordering) so tests are stable.
	out := make([]*BucketUpdateMessage, 0, len(updatesByBucket))
	for _, bs := range r.order {
		if m := updatesByBucket[bs]; m != nil {
			out = append(out, m)
		}
	}
	return out
}

func (r *Room) Snapshot() BucketSnapshotMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *Room) snapshotLocked() BucketSnapshotMessage {
	actorsTotals := make(map[string]int64)
	buckets := make([]BucketSnapshotEntry, 0, len(r.order))

	// Newest-first buckets.
	for i := len(r.order) - 1; i >= 0; i-- {
		bs := r.order[i]
		agg := r.buckets[bs]
		if agg == nil {
			continue
		}
		copyMap := make(map[string]int64, len(agg.damageByActor))
		for a, v := range agg.damageByActor {
			copyMap[a] = v
			actorsTotals[a] += v
		}
		buckets = append(buckets, BucketSnapshotEntry{
			BucketStartUnixMs: bs,
			DamageByActor:     copyMap,
			TotalDamage:       agg.totalDamage,
		})
	}

	actors := make([]string, 0, len(actorsTotals))
	for a := range actorsTotals {
		actors = append(actors, a)
	}
	sort.Slice(actors, func(i, j int) bool {
		ai := actors[i]
		aj := actors[j]
		if actorsTotals[ai] == actorsTotals[aj] {
			return ai < aj
		}
		return actorsTotals[ai] > actorsTotals[aj]
	})

	return BucketSnapshotMessage{
		Type:      "bucket_snapshot",
		BucketSec: r.bucketSec,
		Buckets:   buckets,
		Actors:    actors,
	}
}

func (r *Room) pruneLocked(nowUnixMs int64) {
	// Dedupe TTL (30s).
	for k, last := range r.dedupeLastSeen {
		if nowUnixMs-last > 30_000 {
			delete(r.dedupeLastSeen, k)
		}
	}

	// Rolling window by count.
	if r.maxBuckets > 0 && len(r.order) > r.maxBuckets {
		excess := len(r.order) - r.maxBuckets
		for i := 0; i < excess; i++ {
			bs := r.order[i]
			delete(r.buckets, bs)
		}
		r.order = append([]int64(nil), r.order[excess:]...)
	}

	// Rolling window by age (approx last maxBuckets buckets).
	if r.bucketSec > 0 && r.maxBuckets > 0 {
		minStart := nowUnixMs - int64(r.maxBuckets)*r.bucketSec*1000
		idx := 0
		for idx < len(r.order) && r.order[idx] < minStart {
			delete(r.buckets, r.order[idx])
			idx++
		}
		if idx > 0 {
			r.order = append([]int64(nil), r.order[idx:]...)
		}
	}

	// Publisher TTL (60s).
	for pid, last := range r.publishers {
		if nowUnixMs-last > 60_000 {
			delete(r.publishers, pid)
		}
	}
}

func (r *Room) summary(nowUnixMs int64, publisherTTL time.Duration) RoomSummary {
	r.mu.Lock()
	defer r.mu.Unlock()

	if publisherTTL <= 0 {
		publisherTTL = 60 * time.Second
	}
	cutoff := nowUnixMs - publisherTTL.Milliseconds()
	pubCount := 0
	for _, last := range r.publishers {
		if last >= cutoff {
			pubCount++
		}
	}

	return RoomSummary{
		RoomID:          r.id,
		LastSeenUnixMs:  r.lastSeenUnixMs,
		PublisherCount:  pubCount,
		SubscriberCount: len(r.subs),
		BucketSec:       int(r.bucketSec),
	}
}

func (r *Room) IngestBatch(serverRecvUnixMs int64, req PublishBatchRequest) []*BucketUpdateMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ingestBatch(serverRecvUnixMs, req)
}

func dedupeKey(ev DamageEvent, tRoundedMs int64) string {
	h := sha256.New()
	_, _ = h.Write([]byte(ev.Actor))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(ev.Target))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(ev.Kind))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(ev.Verb))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strconv.FormatInt(ev.Amount, 10)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strconv.FormatInt(tRoundedMs/1000, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

type RoomRegistry struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewRoomRegistry() *RoomRegistry {
	return &RoomRegistry{rooms: make(map[string]*Room)}
}

func (rr *RoomRegistry) GetOrCreate(roomID string, token string) (*Room, error) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	r, ok := rr.rooms[roomID]
	if !ok {
		r = newRoom(roomID, token)
		rr.rooms[roomID] = r
		return r, nil
	}
	if err := r.authorize(token); err != nil {
		return nil, err
	}
	return r, nil
}

func (rr *RoomRegistry) Get(roomID string) (*Room, bool) {
	rr.mu.RLock()
	r, ok := rr.rooms[roomID]
	rr.mu.RUnlock()
	return r, ok
}

func (rr *RoomRegistry) ListRooms(nowUnixMs int64, activeOnly bool) []RoomSummary {
	rr.mu.RLock()
	rooms := make([]*Room, 0, len(rr.rooms))
	for _, r := range rr.rooms {
		rooms = append(rooms, r)
	}
	rr.mu.RUnlock()

	activeCutoff := nowUnixMs - (30 * time.Minute).Milliseconds()
	out := make([]RoomSummary, 0, len(rooms))
	for _, r := range rooms {
		s := r.summary(nowUnixMs, 60*time.Second)
		if activeOnly {
			if s.LastSeenUnixMs <= 0 || s.LastSeenUnixMs < activeCutoff {
				continue
			}
		}
		out = append(out, s)
	}
	// sort by last seen desc
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastSeenUnixMs == out[j].LastSeenUnixMs {
			return out[i].RoomID < out[j].RoomID
		}
		return out[i].LastSeenUnixMs > out[j].LastSeenUnixMs
	})
	return out
}

func (rr *RoomRegistry) Rooms() []*Room {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	out := make([]*Room, 0, len(rr.rooms))
	for _, r := range rr.rooms {
		out = append(out, r)
	}
	return out
}
