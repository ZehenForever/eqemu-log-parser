package engine

import (
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

func TestTimeFilterLastHours_Allow(t *testing.T) {
	now := time.Unix(10_000, 0)
	f := NewTimeFilterLastHours(1, now)
	if f.Cutoff == nil {
		t.Fatalf("expected cutoff")
	}

	cutoff := *f.Cutoff
	if f.Allow(cutoff.Add(-1 * time.Nanosecond)) {
		t.Fatalf("expected ts before cutoff to be rejected")
	}
	if !f.Allow(cutoff) {
		t.Fatalf("expected ts at cutoff to be allowed")
	}
	if !f.Allow(cutoff.Add(1 * time.Nanosecond)) {
		t.Fatalf("expected ts after cutoff to be allowed")
	}

	no := NewTimeFilterLastHours(0, now)
	if !no.Allow(now.Add(-1000 * time.Hour)) {
		t.Fatalf("expected no-cutoff filter to allow")
	}
}

func TestTimeFilter_Ingestion_SkipsOldEvents(t *testing.T) {
	now := time.Unix(10_000, 0)
	f := NewTimeFilterLastHours(1, now)

	seg := NewEncounterSegmenter(8*time.Second, "")

	oldTs := now.Add(-2 * time.Hour)
	newTs := now.Add(-30 * time.Minute)

	events := []model.Event{
		{Timestamp: oldTs, Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 100, AmountKnown: true},
		{Timestamp: newTs, Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 10, AmountKnown: true},
		{Timestamp: newTs.Add(1 * time.Second), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 20, AmountKnown: true},
	}

	for _, ev := range events {
		if !f.Allow(ev.Timestamp) {
			continue
		}
		seg.Process(ev)
	}

	encs := seg.Finalize()
	if len(encs) != 1 {
		t.Fatalf("encounters=%d want=1", len(encs))
	}
	if encs[0].Total != 30 {
		t.Fatalf("total=%d want=30", encs[0].Total)
	}
}
