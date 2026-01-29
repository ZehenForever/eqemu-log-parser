package engine

import (
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

func TestEngineAggregatesDamage(t *testing.T) {
	e := New()
	base := time.Date(2026, 1, 23, 7, 46, 1, 0, time.Local)

	e.Process(model.Event{Timestamp: base, Kind: model.KindMeleeDamage, Actor: "A", Target: "T", Amount: 10, AmountKnown: true})
	e.Process(model.Event{Timestamp: base.Add(1 * time.Second), Kind: model.KindNonMeleeDamage, Actor: "A", Target: "T", Amount: 5, AmountKnown: true})

	st := e.ByActor["A"]
	if st == nil {
		t.Fatalf("missing actor")
	}
	if st.Melee != 10 || st.NonMelee != 5 || st.Total != 15 {
		t.Fatalf("melee/nonmelee/total=%d/%d/%d", st.Melee, st.NonMelee, st.Total)
	}
}

func TestActorDurationSeconds_Inclusive_FirstEqualsLastIsOne(t *testing.T) {
	st := &ActorStats{FirstDamage: time.Date(2026, 1, 23, 7, 46, 1, 0, time.Local), LastDamage: time.Date(2026, 1, 23, 7, 46, 1, 0, time.Local)}
	if got := st.DurationSeconds(); got != 1 {
		t.Fatalf("DurationSeconds=%v want=1", got)
	}
}

func TestActorDurationSeconds_Inclusive_OneSecondDiffIsTwo(t *testing.T) {
	start := time.Date(2026, 1, 23, 7, 46, 1, 0, time.Local)
	st := &ActorStats{FirstDamage: start, LastDamage: start.Add(1 * time.Second)}
	if got := st.DurationSeconds(); got != 2 {
		t.Fatalf("DurationSeconds=%v want=2", got)
	}
}
