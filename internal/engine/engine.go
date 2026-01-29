package engine

import (
	"sort"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

type ActorStats struct {
	Actor        string
	Melee        int64
	NonMelee     int64
	Total        int64
	FirstDamage  time.Time
	LastDamage   time.Time
	TargetDamage map[string]int64
}

func (s *ActorStats) DurationSeconds() float64 {
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

func (s *ActorStats) DPS() float64 {
	d := s.DurationSeconds()
	if d <= 0 {
		return 0
	}
	return float64(s.Total) / d
}

type TargetStats struct {
	Target string
	Total  int64
}

type Engine struct {
	ByActor     map[string]*ActorStats
	TotalTarget map[string]int64
}

func New() *Engine {
	return &Engine{
		ByActor:     make(map[string]*ActorStats),
		TotalTarget: make(map[string]int64),
	}
}

func (e *Engine) Process(ev model.Event) model.Event {
	switch ev.Kind {
	case model.KindMeleeDamage, model.KindNonMeleeDamage:
		if ev.AmountKnown {
			e.addDamage(ev)
		}
		return ev
	default:
		return ev
	}
}

func (e *Engine) addDamage(ev model.Event) {
	st := e.ByActor[ev.Actor]
	if st == nil {
		st = &ActorStats{Actor: ev.Actor, TargetDamage: make(map[string]int64)}
		e.ByActor[ev.Actor] = st
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
	st.Total += ev.Amount

	if ev.Target != "" {
		st.TargetDamage[ev.Target] += ev.Amount
		e.TotalTarget[ev.Target] += ev.Amount
	}
}

func (e *Engine) ActorsSortedByTotal() []*ActorStats {
	out := make([]*ActorStats, 0, len(e.ByActor))
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

func (e *Engine) TopTargets(n int) []TargetStats {
	out := make([]TargetStats, 0, len(e.TotalTarget))
	for t, v := range e.TotalTarget {
		out = append(out, TargetStats{Target: t, Total: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total == out[j].Total {
			return out[i].Target < out[j].Target
		}
		return out[i].Total > out[j].Total
	})
	if n <= 0 || len(out) <= n {
		return out
	}
	return out[:n]
}
