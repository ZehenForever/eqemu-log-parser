package engine

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

type DamageBreakdownStats struct {
	Class       model.DamageClass
	Name        string
	Hits        int64
	CritHits    int64
	TotalDamage int64
	MinHit      int64
	MaxHit      int64
	CritDamage  int64
}

type DamageBreakdownRowView struct {
	Name      string  `json:"name"`
	PctPlayer float64 `json:"pctPlayer"`
	Damage    int64   `json:"damage"`
	DPS       float64 `json:"dpsEncounter"`
	SDPS      float64 `json:"sdps"`
	Sec       int64   `json:"sec"`
	Hits      int64   `json:"hits"`
	MaxHit    int64   `json:"maxHit"`
	MinHit    int64   `json:"minHit"`
	AvgHit    float64 `json:"avgHit"`
	CritPct   float64 `json:"critPct"`
	AvgCrit   float64 `json:"avgCrit"`
}

type DamageBreakdownView struct {
	EncounterID string                   `json:"encounterId"`
	Target      string                   `json:"target"`
	Actor       string                   `json:"actor"`
	Rows        []DamageBreakdownRowView `json:"rows"`
}

func parseEncounterKey(encounterKey string) (target string, start time.Time, ok bool) {
	parts := strings.Split(encounterKey, "|")
	if len(parts) != 2 {
		return "", time.Time{}, false
	}
	target = parts[0]
	ms, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", time.Time{}, false
	}
	start = time.UnixMilli(ms).In(time.UTC)
	return target, start, true
}

func (s *EncounterSegmenter) findEncounterByKey(target string, start time.Time) *Encounter {
	encs := s.Snapshot()
	var best *Encounter
	for _, enc := range encs {
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
	if best != nil {
		return best
	}

	coalesced := s.coalesceEncounters(encs, 0)
	for _, enc := range coalesced {
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
	return best
}

func (s *EncounterSegmenter) GetDamageBreakdownByKey(encounterKey string, actor string) (DamageBreakdownView, bool) {
	target, start, ok := parseEncounterKey(encounterKey)
	if !ok {
		return DamageBreakdownView{}, false
	}
	if actor == "" {
		return DamageBreakdownView{}, false
	}

	enc := s.findEncounterByKey(target, start)
	if enc == nil {
		return DamageBreakdownView{}, false
	}
	st := enc.ByActor[actor]
	if st == nil {
		return DamageBreakdownView{}, false
	}
	if st.Breakdown == nil {
		return DamageBreakdownView{EncounterID: encounterID(enc.Target, enc.Start, enc.End), Target: enc.Target, Actor: actor, Rows: nil}, true
	}

	encSec := durationSecondsInt(enc.Start, enc.End)
	activeSec := durationSecondsInt(st.FirstDamage, st.LastDamage)
	actorTotal := st.Total

	rows := make([]DamageBreakdownRowView, 0, len(st.Breakdown))
	for _, c := range []model.DamageClass{model.DamageClassPierce, model.DamageClassSlash, model.DamageClassCrush, model.DamageClassBash, model.DamageClassKick, model.DamageClassDirect} {
		agg := st.Breakdown[c]
		if agg == nil || agg.Hits <= 0 {
			continue
		}

		pctPlayer := 0.0
		if actorTotal > 0 {
			pctPlayer = (float64(agg.TotalDamage) / float64(actorTotal)) * 100
		}
		dps := 0.0
		if encSec > 0 {
			dps = float64(agg.TotalDamage) / float64(encSec)
		}
		sdps := 0.0
		if activeSec > 0 {
			sdps = float64(agg.TotalDamage) / float64(activeSec)
		}
		avgHit := float64(0)
		if agg.Hits > 0 {
			avgHit = float64(agg.TotalDamage) / float64(agg.Hits)
		}
		critPct := 0.0
		if agg.Hits > 0 {
			critPct = (float64(agg.CritHits) / float64(agg.Hits)) * 100
		}
		avgCrit := 0.0
		if agg.CritHits > 0 {
			avgCrit = float64(agg.CritDamage) / float64(agg.CritHits)
		}

		rows = append(rows, DamageBreakdownRowView{
			Name:      agg.Name,
			PctPlayer: pctPlayer,
			Damage:    agg.TotalDamage,
			DPS:       dps,
			SDPS:      sdps,
			Sec:       activeSec,
			Hits:      agg.Hits,
			MaxHit:    agg.MaxHit,
			MinHit:    agg.MinHit,
			AvgHit:    avgHit,
			CritPct:   critPct,
			AvgCrit:   avgCrit,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Damage == rows[j].Damage {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].Damage > rows[j].Damage
	})

	return DamageBreakdownView{EncounterID: encounterID(enc.Target, enc.Start, enc.End), Target: enc.Target, Actor: actor, Rows: rows}, true
}

func damageClassName(c model.DamageClass) string {
	switch c {
	case model.DamageClassPierce:
		return "Pierces"
	case model.DamageClassSlash:
		return "Slashes"
	case model.DamageClassCrush:
		return "Crushes"
	case model.DamageClassBash:
		return "Bashes"
	case model.DamageClassKick:
		return "Kicks"
	case model.DamageClassDirect:
		return "Direct Damage"
	default:
		return "Unknown"
	}
}

func (s *EncounterSegmenter) GetDamageBreakdown(encounterId string, actor string) (DamageBreakdownView, bool) {
	target, start, end, ok := parseEncounterID(encounterId)
	if !ok {
		return DamageBreakdownView{}, false
	}
	if actor == "" {
		return DamageBreakdownView{}, false
	}

	enc := s.findEncounterExact(target, start, end)
	if enc == nil {
		return DamageBreakdownView{}, false
	}
	st := enc.ByActor[actor]
	if st == nil {
		return DamageBreakdownView{}, false
	}
	if st.Breakdown == nil {
		return DamageBreakdownView{EncounterID: encounterId, Target: enc.Target, Actor: actor, Rows: nil}, true
	}

	encSec := durationSecondsInt(enc.Start, enc.End)
	activeSec := durationSecondsInt(st.FirstDamage, st.LastDamage)
	actorTotal := st.Total

	rows := make([]DamageBreakdownRowView, 0, len(st.Breakdown))
	for _, c := range []model.DamageClass{model.DamageClassPierce, model.DamageClassSlash, model.DamageClassCrush, model.DamageClassBash, model.DamageClassKick, model.DamageClassDirect} {
		agg := st.Breakdown[c]
		if agg == nil || agg.Hits <= 0 {
			continue
		}

		pctPlayer := 0.0
		if actorTotal > 0 {
			pctPlayer = (float64(agg.TotalDamage) / float64(actorTotal)) * 100
		}
		dps := 0.0
		if encSec > 0 {
			dps = float64(agg.TotalDamage) / float64(encSec)
		}
		sdps := 0.0
		if activeSec > 0 {
			sdps = float64(agg.TotalDamage) / float64(activeSec)
		}
		avgHit := float64(0)
		if agg.Hits > 0 {
			avgHit = float64(agg.TotalDamage) / float64(agg.Hits)
		}
		critPct := 0.0
		if agg.Hits > 0 {
			critPct = (float64(agg.CritHits) / float64(agg.Hits)) * 100
		}
		avgCrit := 0.0
		if agg.CritHits > 0 {
			avgCrit = float64(agg.CritDamage) / float64(agg.CritHits)
		}

		rows = append(rows, DamageBreakdownRowView{
			Name:      agg.Name,
			PctPlayer: pctPlayer,
			Damage:    agg.TotalDamage,
			DPS:       dps,
			SDPS:      sdps,
			Sec:       activeSec,
			Hits:      agg.Hits,
			MaxHit:    agg.MaxHit,
			MinHit:    agg.MinHit,
			AvgHit:    avgHit,
			CritPct:   critPct,
			AvgCrit:   avgCrit,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Damage == rows[j].Damage {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].Damage > rows[j].Damage
	})

	return DamageBreakdownView{EncounterID: encounterId, Target: enc.Target, Actor: actor, Rows: rows}, true
}

func (s *EncounterSegmenter) findEncounterExact(target string, start, end time.Time) *Encounter {
	encs := s.Snapshot()
	for _, enc := range encs {
		if enc == nil {
			continue
		}
		if enc.Target != target {
			continue
		}
		if !enc.Start.Equal(start) {
			continue
		}
		if !enc.End.Equal(end) {
			continue
		}
		return enc
	}

	// If the caller provided a coalesced encounter ID, it won't exist as a base segment.
	// Search the coalesced view as well so breakdown works with the UI encounter list.
	coalesced := s.coalesceEncounters(encs, 0)
	for _, enc := range coalesced {
		if enc == nil {
			continue
		}
		if enc.Target != target {
			continue
		}
		if !enc.Start.Equal(start) {
			continue
		}
		if !enc.End.Equal(end) {
			continue
		}
		return enc
	}
	return nil
}

func parseEncounterID(encounterId string) (target string, start time.Time, end time.Time, ok bool) {
	parts := strings.Split(encounterId, "|")
	if len(parts) != 3 {
		return "", time.Time{}, time.Time{}, false
	}
	target = parts[0]
	start, err := time.Parse(time.RFC3339, parts[1])
	if err != nil {
		return "", time.Time{}, time.Time{}, false
	}
	end, err = time.Parse(time.RFC3339, parts[2])
	if err != nil {
		return "", time.Time{}, time.Time{}, false
	}
	return target, start, end, true
}
