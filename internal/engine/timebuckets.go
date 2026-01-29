package engine

import (
	"sort"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

type BucketKey struct {
	Start time.Time
}

type PlayerBucket struct {
	BucketStart   string           `json:"bucketStart"`
	BucketSec     int64            `json:"bucketSec"`
	DamageByActor map[string]int64 `json:"damageByActor"`
	TotalDamage   int64            `json:"totalDamage"`
}

type PlayersSeries struct {
	Now        string         `json:"now"`
	BucketSec  int64          `json:"bucketSec"`
	MaxBuckets int            `json:"maxBuckets"`
	Actors     []string       `json:"actors"`
	Buckets    []PlayerBucket `json:"buckets"`
}

type playersBucketAgg struct {
	bucketSec  int64
	maxBuckets int
	buckets    map[int64]map[string]int64
	totals     map[int64]int64
	order      []int64
}

func newPlayersBucketAgg(bucketSec int64, maxBuckets int) *playersBucketAgg {
	if bucketSec <= 0 {
		bucketSec = 5
	}
	if maxBuckets <= 0 {
		maxBuckets = 100
	}
	return &playersBucketAgg{
		bucketSec:  bucketSec,
		maxBuckets: maxBuckets,
		buckets:    make(map[int64]map[string]int64),
		totals:     make(map[int64]int64),
		order:      nil,
	}
}

func (a *playersBucketAgg) add(ts time.Time, actor string, amount int64) {
	if amount <= 0 {
		return
	}
	unix := ts.Unix()
	bucketStart := unix - (unix % a.bucketSec)
	m := a.buckets[bucketStart]
	if m == nil {
		m = make(map[string]int64)
		a.buckets[bucketStart] = m
		a.order = append(a.order, bucketStart)
	}
	m[actor] += amount
	a.totals[bucketStart] += amount
	if len(a.order) > a.maxBuckets {
		a.evict()
	}
}

func (a *playersBucketAgg) evict() {
	if len(a.order) <= a.maxBuckets {
		return
	}
	max := a.order[0]
	for _, v := range a.order[1:] {
		if v > max {
			max = v
		}
	}
	cut := max - (int64(a.maxBuckets)-1)*a.bucketSec
	kept := a.order[:0]
	for _, bs := range a.order {
		if bs < cut {
			delete(a.buckets, bs)
			delete(a.totals, bs)
			continue
		}
		kept = append(kept, bs)
	}
	a.order = kept
}

func (a *playersBucketAgg) buildSeries(now time.Time, actorOrder []string, mode string, local string) PlayersSeries {
	bucketStartList := make([]int64, 0, len(a.order))
	seen := make(map[int64]struct{}, len(a.order))
	for _, bs := range a.order {
		if _, ok := seen[bs]; ok {
			continue
		}
		seen[bs] = struct{}{}
		bucketStartList = append(bucketStartList, bs)
	}
	sort.Slice(bucketStartList, func(i, j int) bool { return bucketStartList[i] > bucketStartList[j] })

	buckets := make([]PlayerBucket, 0, len(bucketStartList))
	for _, bs := range bucketStartList {
		total := a.totals[bs]
		if total <= 0 {
			continue
		}
		row := PlayerBucket{
			BucketStart:   time.Unix(bs, 0).In(time.Local).Format(time.RFC3339),
			BucketSec:     a.bucketSec,
			DamageByActor: make(map[string]int64),
			TotalDamage:   total,
		}
		for _, actor := range actorOrder {
			if mode == "me" && local != "" && actor != local {
				continue
			}
			if m := a.buckets[bs]; m != nil {
				if v := m[actor]; v != 0 {
					row.DamageByActor[actor] = v
				}
			}
		}
		buckets = append(buckets, row)
	}

	actors := actorOrder
	if mode == "me" && local != "" {
		actors = []string{local}
	}

	return PlayersSeries{
		Now:        now.In(time.Local).Format(time.RFC3339),
		BucketSec:  a.bucketSec,
		MaxBuckets: a.maxBuckets,
		Actors:     actors,
		Buckets:    buckets,
	}
}

func (s *EncounterSegmenter) observeIdentityEvent(ev model.Event) {
	s.identityEvents = append(s.identityEvents, ev)
	if len(s.identityEvents) > 8192 {
		s.identityEvents = s.identityEvents[len(s.identityEvents)-4096:]
	}
	s.identityDirty = true
}

func (s *EncounterSegmenter) refreshIdentityIfNeeded(force bool) {
	if !s.identityDirty && !force {
		return
	}
	if len(s.identityEvents) == 0 {
		return
	}
	scores := ClassifyNames(s.identityEvents)
	ApplyIdentityOverrides(scores, DefaultPCThreshold, nil, nil)
	s.identityScores = scores
	s.identityDirty = false
}

func (s *EncounterSegmenter) isLikelyPCActor(name string) bool {
	if name == "" {
		return false
	}
	if s.identityDirty || s.identityScores == nil {
		s.refreshIdentityIfNeeded(true)
	}
	sc, ok := s.identityScores[name]
	if !ok {
		return false
	}
	return sc.Class == IdentityLikelyPC
}

func (s *EncounterSegmenter) BuildPlayersSeries(now time.Time, bucketSec int64, maxBuckets int, mode string) PlayersSeries {
	if mode != "me" {
		mode = "all"
	}

	agg := newPlayersBucketAgg(bucketSec, maxBuckets)

	for _, ev := range s.recentDamageEvents {
		if ev.Kind != model.KindMeleeDamage && ev.Kind != model.KindNonMeleeDamage {
			continue
		}
		if !ev.AmountKnown {
			continue
		}
		if !s.isLikelyPCActor(ev.Actor) {
			continue
		}
		agg.add(ev.Timestamp, ev.Actor, ev.Amount)
	}
	agg.evict()

	totals := make(map[string]int64)
	for _, bs := range agg.order {
		m := agg.buckets[bs]
		for actor, v := range m {
			totals[actor] += v
		}
	}

	actors := make([]string, 0, len(totals))
	for a := range totals {
		actors = append(actors, a)
	}

	local := s.PlayerName
	sort.Slice(actors, func(i, j int) bool {
		a := actors[i]
		b := actors[j]
		if local != "" {
			if a == local && b != local {
				return true
			}
			if b == local && a != local {
				return false
			}
		}
		if totals[a] == totals[b] {
			return a < b
		}
		return totals[a] > totals[b]
	})

	return agg.buildSeries(now, actors, mode, local)
}
