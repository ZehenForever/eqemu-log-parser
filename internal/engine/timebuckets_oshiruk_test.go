package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
	"github.com/ZehenForever/eqemu-log-parser/internal/parse"
)

func TestPlayersSeries_OshirukIsNotAnActor(t *testing.T) {
	p := filepath.Join("..", "..", "testdata", "encounter_Oshiruk.txt")
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("failed to open fixture %q: %v", p, err)
	}
	defer f.Close()

	// This fixture isn't an eqlog_*.txt file; use a stable local player name.
	playerName := "LOCAL"
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
		t.Fatalf("parse error for %q: %v", p, err)
	}

	series := seg.BuildPlayersSeries(time.Now(), 5, 50, "all")
	if len(series.Buckets) == 0 {
		t.Fatalf("expected buckets")
	}

	seenDamage := false
	for _, b := range series.Buckets {
		if b.TotalDamage > 0 {
			seenDamage = true
			break
		}
	}
	if !seenDamage {
		t.Fatalf("expected some non-zero bucket totalDamage")
	}

	if len(series.Actors) == 0 {
		t.Fatalf("expected some actors")
	}

	seenPC := false
	for _, a := range series.Actors {
		if a == "Oshiruk" {
			t.Fatalf("unexpected NPC boss in players actors list: %q", a)
		}
		if a == "YOU" {
			t.Fatalf("unexpected YOU in players actors list")
		}
		if containsSpace(a) {
			t.Fatalf("unexpected multi-word actor in players actors list: %q", a)
		}
		if IsPCActor(a, seg.identityScores) {
			seenPC = true
		}
	}
	if !seenPC {
		t.Fatalf("expected at least one likely PC actor in players series; got=%v", series.Actors)
	}
}

func containsSpace(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' {
			return true
		}
	}
	return false
}
