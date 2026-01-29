package engine

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

func TestSnapshot_EncountersSortedNewestFirst(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	// Encounter A ends earlier
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "a rat", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "a rat", Amount: 10, AmountKnown: true})

	// Encounter B ends later
	seg.Process(model.Event{Timestamp: time.Unix(110, 0), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "a bat", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(111, 0), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "a bat", Amount: 10, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0), "", false, SnapshotOptions{IncludePCTargets: true})
	if len(snap.Encounters) != 2 {
		t.Fatalf("encounters=%d want=2", len(snap.Encounters))
	}
	if snap.Encounters[0].Target != "a bat" {
		t.Fatalf("first target=%q want=a bat", snap.Encounters[0].Target)
	}
	if snap.Encounters[1].Target != "a rat" {
		t.Fatalf("second target=%q want=a rat", snap.Encounters[1].Target)
	}
}

func TestSnapshot_ActorsSortedByTotalDesc(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	// Same encounter target, two actors with different totals
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 100, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindMeleeDamage, Actor: "Bob", Target: "a rat", Amount: 300, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0), "", false, SnapshotOptions{IncludePCTargets: true})
	if len(snap.Encounters) != 1 {
		t.Fatalf("encounters=%d want=1", len(snap.Encounters))
	}
	actors := snap.Encounters[0].Actors
	if len(actors) != 2 {
		t.Fatalf("actors=%d want=2", len(actors))
	}
	if actors[0].Actor != "Bob" || actors[0].Total != 300 {
		t.Fatalf("first actor=%q total=%d want Bob 300", actors[0].Actor, actors[0].Total)
	}
	if actors[1].Actor != "Alice" || actors[1].Total != 100 {
		t.Fatalf("second actor=%q total=%d want Alice 100", actors[1].Actor, actors[1].Total)
	}
}

func TestSnapshot_EncounterID_IsDeterministic(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")
	start := time.Unix(100, 0).In(time.UTC)
	end := time.Unix(101, 0).In(time.UTC)
	seg.Process(model.Event{Timestamp: start, Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Undead Troll Wizard", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: end, Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Undead Troll Wizard", Amount: 10, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0).In(time.UTC), "", false, SnapshotOptions{IncludePCTargets: true})
	if len(snap.Encounters) != 1 {
		t.Fatalf("encounters=%d want=1", len(snap.Encounters))
	}
	e := snap.Encounters[0]
	if e.EncounterID == "" {
		t.Fatalf("empty encounterId")
	}
	want := "Undead Troll Wizard|" + start.Format(time.RFC3339) + "|" + end.Format(time.RFC3339)
	if e.EncounterID != want {
		t.Fatalf("encounterId=%q want=%q", e.EncounterID, want)
	}
}

func TestSnapshot_FiltersPCTargetsByDefault(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	// Make Sigdis appear as an actor to ensure it is classified LikelyPC.
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "a rat", Amount: 10, AmountKnown: true})

	// Encounter targeting a PC-like name.
	seg.Process(model.Event{Timestamp: time.Unix(110, 0), Kind: model.KindMeleeDamage, Actor: "a goblin", Target: "Sigdis", Amount: 10, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0), "", false, SnapshotOptions{})
	if len(snap.Encounters) != 1 {
		t.Fatalf("encounters=%d want=1", len(snap.Encounters))
	}
	if snap.Encounters[0].Target != "a rat" {
		t.Fatalf("target=%q want=a rat", snap.Encounters[0].Target)
	}

	// Also ensure it marshals cleanly.
	if _, err := json.Marshal(snap); err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
}

func TestSnapshot_IncludesPCTarget_WhenTouchedByLocalPlayer(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "Emberval")

	// Create an encounter keyed on a PC-like single-token name.
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Emberval", Target: "Khisanth", Amount: 10, AmountKnown: true})

	// Ensure Khisanth is classified LikelyPC by making it appear as an actor elsewhere.
	seg.Process(model.Event{Timestamp: time.Unix(110, 0), Kind: model.KindMeleeDamage, Actor: "Khisanth", Target: "a rat", Amount: 10, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0), "", false, SnapshotOptions{IncludePCTargets: false})
	found := false
	for _, e := range snap.Encounters {
		if e.Target == "Khisanth" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected encounter %q to be included when local player touched target", "Khisanth")
	}
}

func TestSnapshot_ExcludesPCTarget_WhenNotTouchedByLocalPlayer(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "Emberval")

	// Make Sigdis appear as an actor to ensure it is classified LikelyPC.
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "a rat", Amount: 10, AmountKnown: true})

	// Encounter targeting a PC-like name, but not touched by Emberval.
	seg.Process(model.Event{Timestamp: time.Unix(110, 0), Kind: model.KindMeleeDamage, Actor: "DPS Machine", Target: "Sigdis", Amount: 10, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0), "", false, SnapshotOptions{IncludePCTargets: false})
	for _, e := range snap.Encounters {
		if e.Target == "Sigdis" {
			t.Fatalf("expected encounter %q to remain excluded when local player never touched target", "Sigdis")
		}
	}
}

func TestSnapshot_THJDerivedStats(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	// Single encounter, two actors.
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 100, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 200, AmountKnown: true, Crit: true})
	seg.Process(model.Event{Timestamp: time.Unix(102, 0), Kind: model.KindNonMeleeDamage, Actor: "Bob", Target: "a rat", Amount: 100, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0), "", false, SnapshotOptions{IncludePCTargets: true})
	if len(snap.Encounters) != 1 {
		t.Fatalf("encounters=%d want=1", len(snap.Encounters))
	}
	actors := snap.Encounters[0].Actors
	if len(actors) != 2 {
		t.Fatalf("actors=%d want=2", len(actors))
	}

	// Sorted by total desc: Alice then Bob.
	a := actors[0]
	b := actors[1]
	if a.Actor != "Alice" || b.Actor != "Bob" {
		t.Fatalf("actors=%q,%q", a.Actor, b.Actor)
	}

	if a.Hits != 2 {
		t.Fatalf("alice hits=%d want=2", a.Hits)
	}
	if a.MaxHit != 200 {
		t.Fatalf("alice maxHit=%d want=200", a.MaxHit)
	}
	if math.Abs(a.AvgHit-150.0) > 0.0001 {
		t.Fatalf("alice avgHit=%v want=150", a.AvgHit)
	}
	if a.Crits != 1 {
		t.Fatalf("alice crits=%d want=1", a.Crits)
	}
	if math.Abs(a.CritPct-50.0) > 0.0001 {
		t.Fatalf("alice critPct=%v want=50", a.CritPct)
	}
	if math.Abs(a.AvgCrit-200.0) > 0.0001 {
		t.Fatalf("alice avgCrit=%v want=200", a.AvgCrit)
	}

	if b.Hits != 1 {
		t.Fatalf("bob hits=%d want=1", b.Hits)
	}
	if b.MaxHit != 100 {
		t.Fatalf("bob maxHit=%d want=100", b.MaxHit)
	}
	if math.Abs(b.AvgHit-100.0) > 0.0001 {
		t.Fatalf("bob avgHit=%v want=100", b.AvgHit)
	}
	if b.Crits != 0 {
		t.Fatalf("bob crits=%d want=0", b.Crits)
	}
	if b.CritPct != 0 {
		t.Fatalf("bob critPct=%v want=0", b.CritPct)
	}
	if b.AvgCrit != 0 {
		t.Fatalf("bob avgCrit=%v want=0", b.AvgCrit)
	}

	// PctTotal should sum to ~100.
	pctSum := a.PctTotal + b.PctTotal
	if math.Abs(pctSum-100.0) > 0.0001 {
		t.Fatalf("pctSum=%v want=100", pctSum)
	}
}
