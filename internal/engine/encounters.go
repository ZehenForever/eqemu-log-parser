package engine

import (
	"sort"
	"strings"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

type EncounterActorStats struct {
	Actor       string
	Melee       int64
	NonMelee    int64
	Total       int64
	Breakdown   map[model.DamageClass]*DamageBreakdownStats
	Hits        int64
	CritHits    int64
	MaxHit      int64
	CritDmgSum  int64
	FirstDamage time.Time
	LastDamage  time.Time
}

func (s *EncounterActorStats) ActiveSeconds() float64 {
	if s.FirstDamage.IsZero() || s.LastDamage.IsZero() {
		return 0
	}
	d := s.LastDamage.Sub(s.FirstDamage).Seconds()
	if d < 0 {
		return 0
	}
	d = d + 1
	if d < 1 {
		return 1
	}
	return d
}

type Encounter struct {
	Target string
	Start  time.Time
	End    time.Time

	ByActor map[string]*EncounterActorStats
	Total   int64
}

func (e *Encounter) DurationSeconds() float64 {
	if e.Start.IsZero() || e.End.IsZero() {
		return 0
	}
	d := e.End.Sub(e.Start).Seconds()
	if d < 0 {
		return 0
	}
	d = d + 1
	if d < 1 {
		return 1
	}
	return d
}

func (e *Encounter) DPS() float64 {
	d := e.DurationSeconds()
	if d <= 0 {
		return 0
	}
	return float64(e.Total) / d
}

func (e *Encounter) ActorsSortedByTotal() []*EncounterActorStats {
	out := make([]*EncounterActorStats, 0, len(e.ByActor))
	for _, st := range e.ByActor {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total == out[j].Total {
			return out[i].Actor < out[j].Actor
		}
		return out[i].Total > out[j].Total
	})
	return out
}

type EncounterSegmenter struct {
	IdleTimeout     time.Duration
	PlayerName      string
	ExcludedTargets map[string]struct{}

	localTouchedTargets map[string]struct{}
	combatTs            []time.Time
	identityEvents      []model.Event
	identityDirty       bool
	identityScores      map[string]IdentityScore
	recentDamageEvents  []model.Event

	active map[string]*activeEncounter
	done   []*Encounter
}

type activeEncounter struct {
	enc    *Encounter
	lastTs time.Time
}

func NewEncounterSegmenter(idleTimeout time.Duration, playerName string) *EncounterSegmenter {
	if idleTimeout <= 0 {
		idleTimeout = 8 * time.Second
	}
	return &EncounterSegmenter{
		IdleTimeout:         idleTimeout,
		PlayerName:          playerName,
		localTouchedTargets: make(map[string]struct{}),
		active:              make(map[string]*activeEncounter),
	}
}

func (s *EncounterSegmenter) SetExcludedTargets(targets map[string]struct{}) {
	s.ExcludedTargets = targets
}

func (s *EncounterSegmenter) appendCombatTimestamp(ts time.Time) {
	if ts.IsZero() {
		return
	}
	if len(s.combatTs) == 0 {
		s.combatTs = append(s.combatTs, ts)
		return
	}
	last := s.combatTs[len(s.combatTs)-1]
	if !ts.Before(last) {
		s.combatTs = append(s.combatTs, ts)
		return
	}
	idx := sort.Search(len(s.combatTs), func(i int) bool {
		return !s.combatTs[i].Before(ts)
	})
	s.combatTs = append(s.combatTs, time.Time{})
	copy(s.combatTs[idx+1:], s.combatTs[idx:])
	s.combatTs[idx] = ts
}

func (s *EncounterSegmenter) hasCombatBetween(start, end time.Time) bool {
	if start.IsZero() || end.IsZero() {
		return false
	}
	if !end.After(start) {
		return false
	}
	if len(s.combatTs) == 0 {
		return false
	}

	start = start.Add(time.Nanosecond)
	end = end.Add(-time.Nanosecond)
	if !end.After(start) {
		return false
	}

	idx := sort.Search(len(s.combatTs), func(i int) bool {
		return !s.combatTs[i].Before(start)
	})
	if idx >= len(s.combatTs) {
		return false
	}
	return !s.combatTs[idx].After(end)
}

func (s *EncounterSegmenter) Process(ev model.Event) {
	// Identity and time-series tracking are additive and do not affect encounter segmentation.
	// Identity classifier consumes a sliding window of recent events.
	if ev.Kind == model.KindCastStart || isEncounterDamageEvent(ev) {
		s.observeIdentityEvent(ev)
	}
	// Players series consumes a bounded window of outgoing amount-bearing damage events.
	if isEncounterDamageEvent(ev) {
		s.recentDamageEvents = append(s.recentDamageEvents, ev)
		if len(s.recentDamageEvents) > 20000 {
			s.recentDamageEvents = s.recentDamageEvents[len(s.recentDamageEvents)-10000:]
		}
	}
	if isEncounterDamageEvent(ev) {
		s.appendCombatTimestamp(ev.Timestamp)
	}
	if isEncounterDamageEvent(ev) && isValidEncounterTarget(ev.Target) {
		if (s.PlayerName == "" || ev.Target != s.PlayerName) && s.PlayerName != "" {
			if ev.Actor == s.PlayerName || ev.Actor == "YOU" {
				s.localTouchedTargets[ev.Target] = struct{}{}
			}
		}
	}
	if !isEncounterDamageEvent(ev) {
		return
	}
	if !isValidEncounterTarget(ev.Target) {
		return
	}
	if s.ExcludedTargets != nil {
		if _, ok := s.ExcludedTargets[ev.Target]; ok {
			return
		}
	}
	if s.PlayerName != "" && ev.Target == s.PlayerName {
		return
	}

	target := ev.Target
	ae := s.active[target]
	if ae == nil {
		ae = &activeEncounter{enc: &Encounter{Target: target, Start: ev.Timestamp, ByActor: make(map[string]*EncounterActorStats)}, lastTs: ev.Timestamp}
		s.active[target] = ae
	}

	if !ae.lastTs.IsZero() && !ev.Timestamp.IsZero() {
		if ev.Timestamp.Sub(ae.lastTs) > s.IdleTimeout {
			ae.enc.End = ae.lastTs
			s.done = append(s.done, ae.enc)
			ae = &activeEncounter{enc: &Encounter{Target: target, Start: ev.Timestamp, ByActor: make(map[string]*EncounterActorStats)}, lastTs: ev.Timestamp}
			s.active[target] = ae
		}
	}

	ae.lastTs = ev.Timestamp
	ae.enc.End = ev.Timestamp

	st := ae.enc.ByActor[ev.Actor]
	if st == nil {
		st = &EncounterActorStats{Actor: ev.Actor, Breakdown: make(map[model.DamageClass]*DamageBreakdownStats)}
		ae.enc.ByActor[ev.Actor] = st
	}
	if st.Breakdown == nil {
		st.Breakdown = make(map[model.DamageClass]*DamageBreakdownStats)
	}
	if st.FirstDamage.IsZero() || ev.Timestamp.Before(st.FirstDamage) {
		st.FirstDamage = ev.Timestamp
	}
	if st.LastDamage.IsZero() || ev.Timestamp.After(st.LastDamage) {
		st.LastDamage = ev.Timestamp
	}
	switch ev.Kind {
	case model.KindMeleeDamage:
		st.Melee += ev.Amount
	case model.KindNonMeleeDamage:
		st.NonMelee += ev.Amount
	}

	if ev.DamageClass != model.DamageClassUnknown {
		agg := st.Breakdown[ev.DamageClass]
		if agg == nil {
			agg = &DamageBreakdownStats{Class: ev.DamageClass, Name: damageClassName(ev.DamageClass)}
			st.Breakdown[ev.DamageClass] = agg
		}
		if agg.Hits == 0 {
			agg.MinHit = ev.Amount
			agg.MaxHit = ev.Amount
		} else {
			if ev.Amount < agg.MinHit {
				agg.MinHit = ev.Amount
			}
			if ev.Amount > agg.MaxHit {
				agg.MaxHit = ev.Amount
			}
		}
		agg.Hits += 1
		agg.TotalDamage += ev.Amount
		if ev.Crit {
			agg.CritHits += 1
			agg.CritDamage += ev.Amount
		}
	}
	st.Hits += 1
	if ev.Amount > st.MaxHit {
		st.MaxHit = ev.Amount
	}
	if ev.Crit {
		st.CritHits += 1
		st.CritDmgSum += ev.Amount
	}
	st.Total += ev.Amount
	ae.enc.Total += ev.Amount
}

func (s *EncounterSegmenter) Finalize() []*Encounter {
	for _, ae := range s.active {
		if ae.enc != nil {
			if ae.enc.End.IsZero() {
				ae.enc.End = ae.lastTs
			}
			s.done = append(s.done, ae.enc)
		}
	}
	s.active = make(map[string]*activeEncounter)

	sort.Slice(s.done, func(i, j int) bool {
		if s.done[i].Start.Equal(s.done[j].Start) {
			return s.done[i].Target < s.done[j].Target
		}
		return s.done[i].Start.Before(s.done[j].Start)
	})
	return s.done
}

func (s *EncounterSegmenter) Snapshot() []*Encounter {
	out := make([]*Encounter, 0, len(s.done)+len(s.active))
	out = append(out, s.done...)
	for _, ae := range s.active {
		if ae.enc != nil {
			out = append(out, ae.enc)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].Target < out[j].Target
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

func isEncounterDamageEvent(ev model.Event) bool {
	if !ev.AmountKnown {
		return false
	}
	switch ev.Kind {
	case model.KindMeleeDamage, model.KindNonMeleeDamage:
		return true
	case model.KindIncomingDamage:
		return false
	default:
		return false
	}
}

func isValidEncounterTarget(target string) bool {
	if target == "" {
		return false
	}

	lt := strings.ToLower(target)
	if lt == "you" {
		return false
	}
	if strings.HasPrefix(lt, "on ") {
		return false
	}
	if lt == "by non-melee" {
		return false
	}
	if strings.HasPrefix(lt, "by ") {
		return false
	}
	if strings.Contains(lt, "been healed") {
		return false
	}
	return true
}
