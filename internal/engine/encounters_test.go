package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
	"github.com/ZehenForever/eqemu-log-parser/internal/parse"
)

func segmentEncountersFromFile(t *testing.T, includePCTargets bool, pcThreshold int, forcePC, forceNPC map[string]struct{}) []*Encounter {
	t.Helper()

	p := filepath.Join("..", "..", "testdata", "eqlog_Emberval_Imperium_EQ.txt")
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("testdata file not present: %v", err)
	}
	defer f.Close()

	playerName, _ := parse.PlayerNameFromLogPath(p)
	ctx := &model.ParseContext{LocalActorName: playerName}
	seg := NewEncounterSegmenter(8*time.Second, playerName)

	it := parse.ParseFile(f, ctx, time.Local)
	events := make([]model.Event, 0, 1024)
	for it.Next() {
		ev := it.Event()
		if playerName != "" {
			if ev.Actor == "YOU" {
				ev.Actor = playerName
			}
			if ev.Target == "YOU" {
				ev.Target = playerName
			}
		}
		events = append(events, ev)
	}
	if err := it.Err(); err != nil {
		t.Fatalf("iter error: %v", err)
	}

	scores := ClassifyNames(events)
	for n := range forcePC {
		if _, ok := scores[n]; !ok {
			scores[n] = IdentityScore{Name: n}
		}
	}
	for n := range forceNPC {
		if _, ok := scores[n]; !ok {
			scores[n] = IdentityScore{Name: n}
		}
	}
	ApplyIdentityOverrides(scores, pcThreshold, forcePC, forceNPC)

	if !includePCTargets {
		excluded := make(map[string]struct{})
		for name, sc := range scores {
			if sc.Class == IdentityLikelyPC {
				excluded[name] = struct{}{}
			}
		}
		seg.SetExcludedTargets(excluded)
	}

	for _, ev := range events {
		seg.Process(ev)
	}
	return seg.Finalize()
}

func TestEncounterDurationSeconds_Inclusive_FirstEqualsLastIsOne(t *testing.T) {
	e := &Encounter{Start: time.Date(2026, 1, 23, 7, 46, 1, 0, time.Local), End: time.Date(2026, 1, 23, 7, 46, 1, 0, time.Local)}
	if got := e.DurationSeconds(); got != 1 {
		t.Fatalf("DurationSeconds=%v want=1", got)
	}
}

func TestEncounterCoalescing_MergesWithCombatBetween(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	// Segment 1: Lord Soth
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Lord Soth", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Lord Soth", Amount: 10, AmountKnown: true})

	// Gap with combat activity against adds.
	seg.Process(model.Event{Timestamp: time.Unix(120, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Fallen Knight of Soth", Amount: 5, AmountKnown: true})

	// Segment 2: Lord Soth resumes within merge gap window.
	seg.Process(model.Event{Timestamp: time.Unix(140, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Lord Soth", Amount: 20, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(141, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Lord Soth", Amount: 20, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0), "", false, SnapshotOptions{IncludePCTargets: true, CoalesceTargets: true, CoalesceMergeGap: 90 * time.Second})

	found := 0
	for _, e := range snap.Encounters {
		if e.Target == "Lord Soth" {
			found++
			if e.TotalDamage != 60 {
				t.Fatalf("total=%d want=60", e.TotalDamage)
			}
			if len(e.Actors) != 1 {
				t.Fatalf("actors=%d want=1", len(e.Actors))
			}
			if e.Actors[0].Actor != "Alice" {
				t.Fatalf("actor=%q", e.Actors[0].Actor)
			}
			// ActiveSec should span from first to last damage to the boss across both segments.
			// FirstDamage=100, LastDamage=141 => 42 inclusive seconds.
			if e.Actors[0].ActiveSec != 42 {
				t.Fatalf("activeSec=%d want=42", e.Actors[0].ActiveSec)
			}
		}
	}
	if found != 1 {
		t.Fatalf("Lord Soth encounters=%d want=1", found)
	}
}

func TestEncounterCoalescing_DoesNotMergeWithoutCombatBetween(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Lord Soth", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Lord Soth", Amount: 10, AmountKnown: true})

	// No combat in the gap.

	seg.Process(model.Event{Timestamp: time.Unix(140, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Lord Soth", Amount: 20, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(141, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "Lord Soth", Amount: 20, AmountKnown: true})

	snap := seg.BuildSnapshot(time.Unix(200, 0), "", false, SnapshotOptions{IncludePCTargets: true, CoalesceTargets: true, CoalesceMergeGap: 90 * time.Second})

	found := 0
	for _, e := range snap.Encounters {
		if e.Target == "Lord Soth" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("Lord Soth encounters=%d want=2", found)
	}
}

func TestEncounterDurationSeconds_Inclusive_OneSecondDiffIsTwo(t *testing.T) {
	start := time.Date(2026, 1, 23, 7, 46, 1, 0, time.Local)
	e := &Encounter{Start: start, End: start.Add(1 * time.Second)}
	if got := e.DurationSeconds(); got != 2 {
		t.Fatalf("DurationSeconds=%v want=2", got)
	}
}

func TestEncounterSegmentation_FullTestdata_HasAtLeastTwoEncounters(t *testing.T) {
	encs := segmentEncountersFromFile(t, false, DefaultPCThreshold, nil, nil)
	if len(encs) < 2 {
		t.Fatalf("encounters=%d (<2)", len(encs))
	}

	foundDummy := false
	foundMachine := false
	for _, enc := range encs {
		if enc.Target == "Machine" {
			t.Fatalf("unexpected encounter target %q", enc.Target)
		}
		if strings.HasPrefix(enc.Target, "hits ") || strings.HasPrefix(enc.Target, "kicks ") || strings.HasPrefix(enc.Target, "bashes ") {
			t.Fatalf("unexpected verb-prefixed encounter target %q", enc.Target)
		}
		if enc.Target == "Sigdis" {
			t.Fatalf("unexpected PC-target encounter %q", enc.Target)
		}
		if strings.Contains(enc.Target, "training dummy") {
			foundDummy = true
		}
		if strings.Contains(enc.Target, "DPS Machine") {
			foundMachine = true
		}
	}
	if !foundDummy {
		t.Fatalf("expected encounter for training dummy")
	}
	if !foundMachine {
		t.Fatalf("expected encounter for DPS Machine")
	}
}

func TestEncounterSegmentation_FullTestdata_IncludePCTargets_RestoresSigdis(t *testing.T) {
	encs := segmentEncountersFromFile(t, true, DefaultPCThreshold, nil, nil)
	foundSigdis := false
	for _, enc := range encs {
		if enc.Target == "Sigdis" {
			foundSigdis = true
			break
		}
	}
	if !foundSigdis {
		t.Fatalf("expected PC-target encounter %q when includePCTargets enabled", "Sigdis")
	}
}

func TestEncounterSegmentation_HealOnly_DoesNotCreateEncounters(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindHeal, Target: "YOU", Amount: 100, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindHeal, Target: "Sigdis", Amount: 200, AmountKnown: true})
	encs := seg.Finalize()
	if len(encs) != 0 {
		t.Fatalf("encounters=%d want=0", len(encs))
	}
}

func TestEncounterSegmentation_DamagePlusHeal_HealDoesNotAffectTotals(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	// Damage creates the encounter and totals.
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 20, AmountKnown: true})

	// Heal should be ignored for encounter creation/extension and totals.
	seg.Process(model.Event{Timestamp: time.Unix(102, 0), Kind: model.KindHeal, Target: "a rat", Amount: 9999, AmountKnown: true})

	encs := seg.Finalize()
	if len(encs) != 1 {
		t.Fatalf("encounters=%d want=1", len(encs))
	}
	if encs[0].Target != "a rat" {
		t.Fatalf("target=%q", encs[0].Target)
	}
	if encs[0].Total != 30 {
		t.Fatalf("total=%d want=30", encs[0].Total)
	}
}

func TestIsValidEncounterTarget_DefensiveRejectsFragments(t *testing.T) {
	invalid := []string{
		"",
		"YOU",
		"you",
		"on YOU",
		"On Sigdis",
		"by non-melee",
		"By non-melee",
		"by DPS Machine",
		"Sigdis has been healed for 10 points.",
	}
	for _, s := range invalid {
		if isValidEncounterTarget(s) {
			t.Fatalf("expected invalid target %q", s)
		}
	}

	valid := []string{
		"a rat",
		"DPS Machine",
		"Innoruuk",
	}
	for _, s := range valid {
		if !isValidEncounterTarget(s) {
			t.Fatalf("expected valid target %q", s)
		}
	}
}

func TestEncounterSegmentation_IncomingOnly_DoesNotCreateEncounters(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")
	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindIncomingDamage, Target: "YOU", Amount: 55, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindIncomingDamage, Target: "YOU", Amount: 66, AmountKnown: true})
	encs := seg.Finalize()
	if len(encs) != 0 {
		t.Fatalf("encounters=%d want=0", len(encs))
	}
}

func TestEncounterSegmentation_DamagePlusIncoming_IncomingDoesNotAffectTotals(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	seg.Process(model.Event{Timestamp: time.Unix(100, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0), Kind: model.KindMeleeDamage, Actor: "Alice", Target: "a rat", Amount: 20, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(102, 0), Kind: model.KindIncomingDamage, Target: "YOU", Amount: 9999, AmountKnown: true})

	encs := seg.Finalize()
	if len(encs) != 1 {
		t.Fatalf("encounters=%d want=1", len(encs))
	}
	if encs[0].Total != 30 {
		t.Fatalf("total=%d want=30", encs[0].Total)
	}
}
