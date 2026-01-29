package engine

import (
	"sort"
	"strconv"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

const defaultCoalesceMergeGap = 90 * time.Second

type ActorStatsView struct {
	Actor     string  `json:"actor"`
	Melee     int64   `json:"melee"`
	NonMelee  int64   `json:"nonMelee"`
	Total     int64   `json:"total"`
	DPS       float64 `json:"dpsEncounter"`
	SDPS      float64 `json:"sdps"`
	ActiveSec int64   `json:"activeSec"`
	PctTotal  float64 `json:"pctTotal"`
	Hits      int64   `json:"hits"`
	MaxHit    int64   `json:"maxHit"`
	AvgHit    float64 `json:"avgHit"`
	CritPct   float64 `json:"critPct"`
	AvgCrit   float64 `json:"avgCrit"`
	Crits     int64   `json:"crits"`
}

type EncounterView struct {
	EncounterKey string           `json:"encounterKey"`
	EncounterID  string           `json:"encounterId"`
	Target       string           `json:"target"`
	Start        time.Time        `json:"start"`
	End          time.Time        `json:"end"`
	EncounterSec int64            `json:"encounterSec"`
	TotalDamage  int64            `json:"totalDamage"`
	DPSEncounter float64          `json:"dpsEncounter"`
	Actors       []ActorStatsView `json:"actors"`
}

func encounterKey(target string, start time.Time) string {
	return target + "|" + strconv.FormatInt(start.UnixMilli(), 10)
}

type Snapshot struct {
	Now            time.Time       `json:"now"`
	FilePath       string          `json:"filePath"`
	Tailing        bool            `json:"tailing"`
	EncounterCount int             `json:"encounterCount"`
	Encounters     []EncounterView `json:"encounters"`
}

type SnapshotOptions struct {
	IncludePCTargets bool
	LimitEncounters  int
	CoalesceTargets  bool
	CoalesceMergeGap time.Duration
}

func (s *EncounterSegmenter) coalesceEncounters(encs []*Encounter, mergeGap time.Duration) []*Encounter {
	if len(encs) == 0 {
		return encs
	}
	if mergeGap <= 0 {
		mergeGap = defaultCoalesceMergeGap
	}

	byTarget := make(map[string][]*Encounter)
	for _, enc := range encs {
		if enc == nil {
			continue
		}
		byTarget[enc.Target] = append(byTarget[enc.Target], enc)
	}

	out := make([]*Encounter, 0, len(encs))
	for _, group := range byTarget {
		sort.Slice(group, func(i, j int) bool {
			if group[i].Start.Equal(group[j].Start) {
				return group[i].End.Before(group[j].End)
			}
			return group[i].Start.Before(group[j].Start)
		})

		var cur *Encounter
		for _, e := range group {
			if e == nil {
				continue
			}
			if cur == nil {
				cur = copyEncounter(e)
				continue
			}

			gap := e.Start.Sub(cur.End)
			if gap > 0 && gap <= mergeGap && s.hasCombatBetween(cur.End, e.Start) {
				cur = mergeEncounters(cur, e)
				continue
			}

			out = append(out, cur)
			cur = copyEncounter(e)
		}
		if cur != nil {
			out = append(out, cur)
		}
	}

	return out
}

func copyEncounter(e *Encounter) *Encounter {
	if e == nil {
		return nil
	}
	out := &Encounter{
		Target:  e.Target,
		Start:   e.Start,
		End:     e.End,
		Total:   e.Total,
		ByActor: make(map[string]*EncounterActorStats, len(e.ByActor)),
	}
	for k, v := range e.ByActor {
		out.ByActor[k] = copyActorStats(v)
	}
	return out
}

func copyActorStats(s *EncounterActorStats) *EncounterActorStats {
	if s == nil {
		return nil
	}
	out := &EncounterActorStats{
		Actor:       s.Actor,
		Melee:       s.Melee,
		NonMelee:    s.NonMelee,
		Total:       s.Total,
		Hits:        s.Hits,
		CritHits:    s.CritHits,
		MaxHit:      s.MaxHit,
		CritDmgSum:  s.CritDmgSum,
		FirstDamage: s.FirstDamage,
		LastDamage:  s.LastDamage,
	}
	if s.Breakdown != nil {
		out.Breakdown = make(map[model.DamageClass]*DamageBreakdownStats, len(s.Breakdown))
		for c, agg := range s.Breakdown {
			if agg == nil {
				continue
			}
			copyAgg := *agg
			out.Breakdown[c] = &copyAgg
		}
	}
	return out
}

func mergeEncounters(a *Encounter, b *Encounter) *Encounter {
	if a == nil {
		return copyEncounter(b)
	}
	if b == nil {
		return copyEncounter(a)
	}

	out := copyEncounter(a)
	out.End = b.End
	out.Total += b.Total

	if out.ByActor == nil {
		out.ByActor = make(map[string]*EncounterActorStats)
	}
	for actor, st := range b.ByActor {
		if st == nil {
			continue
		}
		existing := out.ByActor[actor]
		if existing == nil {
			out.ByActor[actor] = copyActorStats(st)
			continue
		}

		existing.Melee += st.Melee
		existing.NonMelee += st.NonMelee
		existing.Total += st.Total
		existing.Hits += st.Hits
		existing.CritHits += st.CritHits
		existing.CritDmgSum += st.CritDmgSum
		if st.MaxHit > existing.MaxHit {
			existing.MaxHit = st.MaxHit
		}
		if existing.FirstDamage.IsZero() || (!st.FirstDamage.IsZero() && st.FirstDamage.Before(existing.FirstDamage)) {
			existing.FirstDamage = st.FirstDamage
		}
		if existing.LastDamage.IsZero() || (!st.LastDamage.IsZero() && st.LastDamage.After(existing.LastDamage)) {
			existing.LastDamage = st.LastDamage
		}

		if st.Breakdown != nil {
			if existing.Breakdown == nil {
				existing.Breakdown = make(map[model.DamageClass]*DamageBreakdownStats)
			}
			for c, agg := range st.Breakdown {
				if agg == nil {
					continue
				}
				ex := existing.Breakdown[c]
				if ex == nil {
					copyAgg := *agg
					existing.Breakdown[c] = &copyAgg
					continue
				}
				ex.Hits += agg.Hits
				ex.CritHits += agg.CritHits
				ex.TotalDamage += agg.TotalDamage
				ex.CritDamage += agg.CritDamage
				if ex.MinHit == 0 || (agg.MinHit > 0 && agg.MinHit < ex.MinHit) {
					ex.MinHit = agg.MinHit
				}
				if agg.MaxHit > ex.MaxHit {
					ex.MaxHit = agg.MaxHit
				}
			}
		}
	}

	return out
}

func sortEncountersMostRecentFirst(encs []*Encounter) {
	// Most recent first by End time.
	sort.Slice(encs, func(i, j int) bool {
		ei := encs[i].End
		ej := encs[j].End
		if ei.Equal(ej) {
			if encs[i].Start.Equal(encs[j].Start) {
				return encs[i].Target < encs[j].Target
			}
			return encs[i].Start.After(encs[j].Start)
		}
		return ei.After(ej)
	})
}

func filterEncountersForSnapshot(encs []*Encounter, includePCTargets bool, localTouchedTargets map[string]struct{}) []*Encounter {
	scores := classifyNamesFromEncounters(encs)

	filtered := make([]*Encounter, 0, len(encs))
	if includePCTargets {
		filtered = append(filtered, encs...)
		return filtered
	}

	for _, e := range encs {
		if sc, ok := scores[e.Target]; ok {
			if sc.Class == IdentityLikelyPC {
				if localTouchedTargets != nil {
					if _, ok := localTouchedTargets[e.Target]; ok {
						filtered = append(filtered, e)
						continue
					}
				}
				continue
			}
		}
		filtered = append(filtered, e)
	}

	return filtered
}

func (s *EncounterSegmenter) BuildSnapshot(now time.Time, filePath string, tailing bool, opts SnapshotOptions) Snapshot {
	encs := s.Snapshot()
	if len(encs) == 0 {
		return Snapshot{Now: now, FilePath: filePath, Tailing: tailing, EncounterCount: 0, Encounters: nil}
	}

	sortEncountersMostRecentFirst(encs)
	filtered := filterEncountersForSnapshot(encs, opts.IncludePCTargets, s.localTouchedTargets)
	if opts.CoalesceTargets {
		filtered = s.coalesceEncounters(filtered, opts.CoalesceMergeGap)
		sortEncountersMostRecentFirst(filtered)
	}

	if opts.LimitEncounters > 0 && len(filtered) > opts.LimitEncounters {
		filtered = filtered[:opts.LimitEncounters]
	}

	out := Snapshot{
		Now:            now,
		FilePath:       filePath,
		Tailing:        tailing,
		EncounterCount: len(filtered),
		Encounters:     make([]EncounterView, 0, len(filtered)),
	}

	for _, enc := range filtered {
		encSec := durationSecondsInt(enc.Start, enc.End)
		dpsEnc := 0.0
		if encSec > 0 {
			dpsEnc = float64(enc.Total) / float64(encSec)
		}
		view := EncounterView{
			EncounterKey: encounterKey(enc.Target, enc.Start),
			EncounterID:  encounterID(enc.Target, enc.Start, enc.End),
			Target:       enc.Target,
			Start:        enc.Start,
			End:          enc.End,
			EncounterSec: encSec,
			TotalDamage:  enc.Total,
			DPSEncounter: dpsEnc,
			Actors:       make([]ActorStatsView, 0, len(enc.ByActor)),
		}

		actors := enc.ActorsSortedByTotal()
		for _, st := range actors {
			activeSec := durationSecondsInt(st.FirstDamage, st.LastDamage)
			dps := 0.0
			if encSec > 0 {
				dps = float64(st.Total) / float64(encSec)
			}
			sdps := 0.0
			if activeSec > 0 {
				sdps = float64(st.Total) / float64(activeSec)
			}

			pctTotal := 0.0
			if enc.Total > 0 {
				pctTotal = (float64(st.Total) / float64(enc.Total)) * 100
			}
			avgHit := 0.0
			if st.Hits > 0 {
				avgHit = float64(st.Total) / float64(st.Hits)
			}
			critPct := 0.0
			if st.Hits > 0 {
				critPct = (float64(st.CritHits) / float64(st.Hits)) * 100
			}
			avgCrit := 0.0
			if st.CritHits > 0 {
				avgCrit = float64(st.CritDmgSum) / float64(st.CritHits)
			}

			view.Actors = append(view.Actors, ActorStatsView{
				Actor:     st.Actor,
				Melee:     st.Melee,
				NonMelee:  st.NonMelee,
				Total:     st.Total,
				DPS:       dps,
				SDPS:      sdps,
				ActiveSec: activeSec,
				PctTotal:  pctTotal,
				Hits:      st.Hits,
				MaxHit:    st.MaxHit,
				AvgHit:    avgHit,
				CritPct:   critPct,
				AvgCrit:   avgCrit,
				Crits:     st.CritHits,
			})
		}

		out.Encounters = append(out.Encounters, view)
	}

	return out
}

func (s *EncounterSegmenter) BuildSnapshotSummary(now time.Time, filePath string, tailing bool, opts SnapshotOptions) Snapshot {
	encs := s.Snapshot()
	if len(encs) == 0 {
		return Snapshot{Now: now, FilePath: filePath, Tailing: tailing, EncounterCount: 0, Encounters: nil}
	}

	sortEncountersMostRecentFirst(encs)
	filtered := filterEncountersForSnapshot(encs, opts.IncludePCTargets, s.localTouchedTargets)
	if opts.CoalesceTargets {
		filtered = s.coalesceEncounters(filtered, opts.CoalesceMergeGap)
		sortEncountersMostRecentFirst(filtered)
	}

	if opts.LimitEncounters > 0 && len(filtered) > opts.LimitEncounters {
		filtered = filtered[:opts.LimitEncounters]
	}

	out := Snapshot{
		Now:            now,
		FilePath:       filePath,
		Tailing:        tailing,
		EncounterCount: len(filtered),
		Encounters:     make([]EncounterView, 0, len(filtered)),
	}

	for _, enc := range filtered {
		encSec := durationSecondsInt(enc.Start, enc.End)
		dpsEnc := 0.0
		if encSec > 0 {
			dpsEnc = float64(enc.Total) / float64(encSec)
		}
		out.Encounters = append(out.Encounters, EncounterView{
			EncounterKey: encounterKey(enc.Target, enc.Start),
			EncounterID:  encounterID(enc.Target, enc.Start, enc.End),
			Target:       enc.Target,
			Start:        enc.Start,
			End:          enc.End,
			EncounterSec: encSec,
			TotalDamage:  enc.Total,
			DPSEncounter: dpsEnc,
			Actors:       nil,
		})
	}

	return out
}

func (s *EncounterSegmenter) BuildEncounterView(now time.Time, filePath string, tailing bool, opts SnapshotOptions, target string) (EncounterView, bool) {
	encs := s.Snapshot()
	if len(encs) == 0 {
		return EncounterView{}, false
	}

	sortEncountersMostRecentFirst(encs)
	filtered := filterEncountersForSnapshot(encs, opts.IncludePCTargets, s.localTouchedTargets)
	if opts.CoalesceTargets {
		filtered = s.coalesceEncounters(filtered, opts.CoalesceMergeGap)
		sortEncountersMostRecentFirst(filtered)
	}

	if opts.LimitEncounters > 0 && len(filtered) > opts.LimitEncounters {
		filtered = filtered[:opts.LimitEncounters]
	}

	for _, enc := range filtered {
		if enc.Target != target {
			continue
		}

		encSec := durationSecondsInt(enc.Start, enc.End)
		dpsEnc := 0.0
		if encSec > 0 {
			dpsEnc = float64(enc.Total) / float64(encSec)
		}
		view := EncounterView{
			EncounterKey: encounterKey(enc.Target, enc.Start),
			EncounterID:  encounterID(enc.Target, enc.Start, enc.End),
			Target:       enc.Target,
			Start:        enc.Start,
			End:          enc.End,
			EncounterSec: encSec,
			TotalDamage:  enc.Total,
			DPSEncounter: dpsEnc,
			Actors:       make([]ActorStatsView, 0, len(enc.ByActor)),
		}

		actors := enc.ActorsSortedByTotal()
		for _, st := range actors {
			activeSec := durationSecondsInt(st.FirstDamage, st.LastDamage)
			dps := 0.0
			if encSec > 0 {
				dps = float64(st.Total) / float64(encSec)
			}
			sdps := 0.0
			if activeSec > 0 {
				sdps = float64(st.Total) / float64(activeSec)
			}

			pctTotal := 0.0
			if enc.Total > 0 {
				pctTotal = (float64(st.Total) / float64(enc.Total)) * 100
			}
			avgHit := 0.0
			if st.Hits > 0 {
				avgHit = float64(st.Total) / float64(st.Hits)
			}
			critPct := 0.0
			if st.Hits > 0 {
				critPct = (float64(st.CritHits) / float64(st.Hits)) * 100
			}
			avgCrit := 0.0
			if st.CritHits > 0 {
				avgCrit = float64(st.CritDmgSum) / float64(st.CritHits)
			}

			view.Actors = append(view.Actors, ActorStatsView{
				Actor:     st.Actor,
				Melee:     st.Melee,
				NonMelee:  st.NonMelee,
				Total:     st.Total,
				DPS:       dps,
				SDPS:      sdps,
				ActiveSec: activeSec,
				PctTotal:  pctTotal,
				Hits:      st.Hits,
				MaxHit:    st.MaxHit,
				AvgHit:    avgHit,
				CritPct:   critPct,
				AvgCrit:   avgCrit,
				Crits:     st.CritHits,
			})
		}

		return view, true
	}

	return EncounterView{}, false
}

func (s *EncounterSegmenter) BuildEncounterViewByKey(now time.Time, filePath string, tailing bool, opts SnapshotOptions, target string, start time.Time) (EncounterView, bool) {
	encs := s.Snapshot()
	if len(encs) == 0 {
		return EncounterView{}, false
	}

	sortEncountersMostRecentFirst(encs)
	filtered := filterEncountersForSnapshot(encs, opts.IncludePCTargets, s.localTouchedTargets)
	if opts.CoalesceTargets {
		filtered = s.coalesceEncounters(filtered, opts.CoalesceMergeGap)
		sortEncountersMostRecentFirst(filtered)
	}

	if opts.LimitEncounters > 0 && len(filtered) > opts.LimitEncounters {
		filtered = filtered[:opts.LimitEncounters]
	}

	var best *Encounter
	for _, enc := range filtered {
		if enc == nil {
			continue
		}
		if enc.Target != target {
			continue
		}
		if !enc.Start.Equal(start) {
			continue
		}
		if best == nil || enc.End.After(best.End) {
			best = enc
		}
	}
	if best == nil {
		return EncounterView{}, false
	}

	encSec := durationSecondsInt(best.Start, best.End)
	dpsEnc := 0.0
	if encSec > 0 {
		dpsEnc = float64(best.Total) / float64(encSec)
	}
	view := EncounterView{
		EncounterKey: encounterKey(best.Target, best.Start),
		EncounterID:  encounterID(best.Target, best.Start, best.End),
		Target:       best.Target,
		Start:        best.Start,
		End:          best.End,
		EncounterSec: encSec,
		TotalDamage:  best.Total,
		DPSEncounter: dpsEnc,
		Actors:       make([]ActorStatsView, 0, len(best.ByActor)),
	}

	actors := best.ActorsSortedByTotal()
	for _, st := range actors {
		activeSec := durationSecondsInt(st.FirstDamage, st.LastDamage)
		dps := 0.0
		if encSec > 0 {
			dps = float64(st.Total) / float64(encSec)
		}
		sdps := 0.0
		if activeSec > 0 {
			sdps = float64(st.Total) / float64(activeSec)
		}

		pctTotal := 0.0
		if best.Total > 0 {
			pctTotal = (float64(st.Total) / float64(best.Total)) * 100
		}
		avgHit := 0.0
		if st.Hits > 0 {
			avgHit = float64(st.Total) / float64(st.Hits)
		}
		critPct := 0.0
		if st.Hits > 0 {
			critPct = (float64(st.CritHits) / float64(st.Hits)) * 100
		}
		avgCrit := 0.0
		if st.CritHits > 0 {
			avgCrit = float64(st.CritDmgSum) / float64(st.CritHits)
		}

		view.Actors = append(view.Actors, ActorStatsView{
			Actor:     st.Actor,
			Melee:     st.Melee,
			NonMelee:  st.NonMelee,
			Total:     st.Total,
			DPS:       dps,
			SDPS:      sdps,
			ActiveSec: activeSec,
			PctTotal:  pctTotal,
			Hits:      st.Hits,
			MaxHit:    st.MaxHit,
			AvgHit:    avgHit,
			CritPct:   critPct,
			AvgCrit:   avgCrit,
			Crits:     st.CritHits,
		})
	}

	return view, true
}

func (s *EncounterSegmenter) BuildEncounterViewExact(now time.Time, filePath string, tailing bool, opts SnapshotOptions, target string, start, end time.Time) (EncounterView, bool) {
	encs := s.Snapshot()
	if len(encs) == 0 {
		return EncounterView{}, false
	}

	sortEncountersMostRecentFirst(encs)
	filtered := filterEncountersForSnapshot(encs, opts.IncludePCTargets, s.localTouchedTargets)
	if opts.CoalesceTargets {
		filtered = s.coalesceEncounters(filtered, opts.CoalesceMergeGap)
		sortEncountersMostRecentFirst(filtered)
	}

	if opts.LimitEncounters > 0 && len(filtered) > opts.LimitEncounters {
		filtered = filtered[:opts.LimitEncounters]
	}

	for _, enc := range filtered {
		if enc.Target != target {
			continue
		}
		if !enc.Start.Equal(start) {
			continue
		}
		if !enc.End.Equal(end) {
			continue
		}

		encSec := durationSecondsInt(enc.Start, enc.End)
		dpsEnc := 0.0
		if encSec > 0 {
			dpsEnc = float64(enc.Total) / float64(encSec)
		}
		view := EncounterView{
			EncounterKey: encounterKey(enc.Target, enc.Start),
			EncounterID:  encounterID(enc.Target, enc.Start, enc.End),
			Target:       enc.Target,
			Start:        enc.Start,
			End:          enc.End,
			EncounterSec: encSec,
			TotalDamage:  enc.Total,
			DPSEncounter: dpsEnc,
			Actors:       make([]ActorStatsView, 0, len(enc.ByActor)),
		}

		actors := enc.ActorsSortedByTotal()
		for _, st := range actors {
			activeSec := durationSecondsInt(st.FirstDamage, st.LastDamage)
			dps := 0.0
			if encSec > 0 {
				dps = float64(st.Total) / float64(encSec)
			}
			sdps := 0.0
			if activeSec > 0 {
				sdps = float64(st.Total) / float64(activeSec)
			}

			pctTotal := 0.0
			if enc.Total > 0 {
				pctTotal = (float64(st.Total) / float64(enc.Total)) * 100
			}
			avgHit := 0.0
			if st.Hits > 0 {
				avgHit = float64(st.Total) / float64(st.Hits)
			}
			critPct := 0.0
			if st.Hits > 0 {
				critPct = (float64(st.CritHits) / float64(st.Hits)) * 100
			}
			avgCrit := 0.0
			if st.CritHits > 0 {
				avgCrit = float64(st.CritDmgSum) / float64(st.CritHits)
			}

			view.Actors = append(view.Actors, ActorStatsView{
				Actor:     st.Actor,
				Melee:     st.Melee,
				NonMelee:  st.NonMelee,
				Total:     st.Total,
				DPS:       dps,
				SDPS:      sdps,
				ActiveSec: activeSec,
				PctTotal:  pctTotal,
				Hits:      st.Hits,
				MaxHit:    st.MaxHit,
				AvgHit:    avgHit,
				CritPct:   critPct,
				AvgCrit:   avgCrit,
				Crits:     st.CritHits,
			})
		}

		return view, true
	}

	return EncounterView{}, false
}

func encounterID(target string, start, end time.Time) string {
	return target + "|" + start.Format(time.RFC3339) + "|" + end.Format(time.RFC3339)
}

func classifyNamesFromEncounters(encs []*Encounter) map[string]IdentityScore {
	// Identity scoring is based on parsed events. For UI polling, we approximate
	// by generating synthetic model.Event entries from encounter rollups.
	// This lets us reuse the existing ClassifyNames() logic and its defaults.
	synth := make([]model.Event, 0, 1024)
	for _, enc := range encs {
		if enc == nil {
			continue
		}
		for _, st := range enc.ByActor {
			if st == nil {
				continue
			}
			if st.Total <= 0 {
				continue
			}
			// Repeat a few times to satisfy actor_damage>=3 in the scoring.
			synth = append(synth,
				model.Event{Kind: model.KindMeleeDamage, Actor: st.Actor, Target: enc.Target, AmountKnown: true, Timestamp: enc.End},
				model.Event{Kind: model.KindMeleeDamage, Actor: st.Actor, Target: enc.Target, AmountKnown: true, Timestamp: enc.End},
				model.Event{Kind: model.KindMeleeDamage, Actor: st.Actor, Target: enc.Target, AmountKnown: true, Timestamp: enc.End},
			)
			if st.NonMelee > 0 {
				synth = append(synth, model.Event{Kind: model.KindNonMeleeDamage, Actor: st.Actor, Target: enc.Target, AmountKnown: true, Timestamp: enc.End})
			}
		}
	}
	return ClassifyNames(synth)
}

func durationSecondsInt(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	d := end.Sub(start)
	if d < 0 {
		return 0
	}
	sec := int64(d.Seconds()) + 1
	if sec < 1 {
		return 1
	}
	return sec
}
