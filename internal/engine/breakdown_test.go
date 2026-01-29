package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
	"github.com/ZehenForever/eqemu-log-parser/internal/parse"
)

func TestDamageBreakdown_Aggregation(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "")

	start := time.Unix(100, 0).In(time.UTC)
	seg.Process(model.Event{Timestamp: start, Kind: model.KindMeleeDamage, DamageClass: model.DamageClassPierce, Verb: "pierce", Actor: "Alice", Target: "a rat", Amount: 100, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(101, 0).In(time.UTC), Kind: model.KindMeleeDamage, DamageClass: model.DamageClassPierce, Verb: "pierce", Actor: "Alice", Target: "a rat", Amount: 150, AmountKnown: true})
	seg.Process(model.Event{Timestamp: time.Unix(102, 0).In(time.UTC), Kind: model.KindMeleeDamage, DamageClass: model.DamageClassSlash, Verb: "slash", Actor: "Alice", Target: "a rat", Amount: 200, AmountKnown: true, Crit: true})
	end := time.Unix(103, 0).In(time.UTC)
	seg.Process(model.Event{Timestamp: end, Kind: model.KindNonMeleeDamage, DamageClass: model.DamageClassDirect, Actor: "Alice", Target: "a rat", Amount: 50, AmountKnown: true})

	encounterId := "a rat|" + start.Format(time.RFC3339) + "|" + end.Format(time.RFC3339)
	view, ok := seg.GetDamageBreakdown(encounterId, "Alice")
	if !ok {
		t.Fatalf("expected ok")
	}
	if view.EncounterID != encounterId {
		t.Fatalf("encounterId=%q want=%q", view.EncounterID, encounterId)
	}
	if view.Actor != "Alice" {
		t.Fatalf("actor=%q", view.Actor)
	}
	if view.Target != "a rat" {
		t.Fatalf("target=%q", view.Target)
	}
	if len(view.Rows) != 3 {
		t.Fatalf("rows=%d want=3", len(view.Rows))
	}

	// Sorted by damage desc.
	if view.Rows[0].Name != "Pierces" || view.Rows[0].Damage != 250 {
		t.Fatalf("row0=%+v", view.Rows[0])
	}
	if view.Rows[0].Hits != 2 || view.Rows[0].MinHit != 100 || view.Rows[0].MaxHit != 150 {
		t.Fatalf("row0 stats=%+v", view.Rows[0])
	}

	if view.Rows[1].Name != "Slashes" || view.Rows[1].Damage != 200 {
		t.Fatalf("row1=%+v", view.Rows[1])
	}
	if view.Rows[1].Hits != 1 || view.Rows[1].CritPct != 100 {
		t.Fatalf("row1 stats=%+v", view.Rows[1])
	}
	if view.Rows[1].AvgCrit != 200 {
		t.Fatalf("row1 avgCrit=%v want=200", view.Rows[1].AvgCrit)
	}

	if view.Rows[2].Name != "Direct Damage" || view.Rows[2].Damage != 50 {
		t.Fatalf("row2=%+v", view.Rows[2])
	}

	pctSum := view.Rows[0].PctPlayer + view.Rows[1].PctPlayer + view.Rows[2].PctPlayer
	if pctSum < 99.999 || pctSum > 100.001 {
		t.Fatalf("pctSum=%v want~100", pctSum)
	}
}

func TestDamageBreakdown_Regression_TestdataHasMeleeAndDirect(t *testing.T) {
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
		seg.Process(ev)
	}
	if err := it.Err(); err != nil {
		t.Fatalf("iter error: %v", err)
	}

	wantMelee := map[string]struct{}{
		"Pierces": {},
		"Slashes": {},
		"Crushes": {},
		"Bashes":  {},
		"Kicks":   {},
	}

	encs := seg.Snapshot()
	for _, enc := range encs {
		if enc == nil {
			continue
		}
		encounterId := enc.Target + "|" + enc.Start.Format(time.RFC3339) + "|" + enc.End.Format(time.RFC3339)
		for actor, st := range enc.ByActor {
			if st == nil || st.Breakdown == nil {
				continue
			}
			view, ok := seg.GetDamageBreakdown(encounterId, actor)
			if !ok || len(view.Rows) == 0 {
				continue
			}
			foundDirect := false
			foundMelee := false
			for _, r := range view.Rows {
				if r.Name == "Direct Damage" {
					foundDirect = true
				}
				if _, ok := wantMelee[r.Name]; ok {
					foundMelee = true
				}
			}
			if foundDirect && foundMelee {
				return
			}
		}
	}

	t.Fatalf("expected to find at least one encounter+actor in testdata with both melee-class and direct-damage breakdown rows")
}
