package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

func TestParseFile_FullTestdata(t *testing.T) {
	p := filepath.Join("..", "..", "testdata", "eqlog_Emberval_Imperium_EQ.txt")
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("testdata file not present: %v", err)
	}
	defer f.Close()

	playerName, _ := PlayerNameFromLogPath(p)
	ctx := &model.ParseContext{LocalActorName: playerName}

	it := ParseFile(f, ctx, time.Local)
	var melee, nonMelee, thorns, critMeta, castStart, affliction, miss, avoid int
	seenDPS := false
	seenDPSMachine := false
	for it.Next() {
		ev := it.Event()
		if ev.Raw == "" {
			t.Fatalf("empty raw")
		}
		if ev.Timestamp.IsZero() {
			t.Fatalf("zero timestamp for line: %q", ev.Raw)
		}
		if (ev.Kind == model.KindMeleeDamage || ev.Kind == model.KindNonMeleeDamage) && ev.Target == "Machine" {
			t.Fatalf("unexpected truncated target %q for line: %q", ev.Target, ev.Raw)
		}
		if ev.Kind == model.KindMeleeDamage || ev.Kind == model.KindNonMeleeDamage {
			if strings.HasPrefix(ev.Target, "hits ") || strings.HasPrefix(ev.Target, "kicks ") || strings.HasPrefix(ev.Target, "bashes ") {
				t.Fatalf("unexpected verb-prefixed target %q for line: %q", ev.Target, ev.Raw)
			}
		}
		if ev.Actor == "DPS" {
			seenDPS = true
		}
		if ev.Actor == "DPS Machine" {
			seenDPSMachine = true
		}
		switch ev.Kind {
		case model.KindMeleeDamage:
			melee++
		case model.KindNonMeleeDamage:
			nonMelee++
		case model.KindThornsMarker:
			thorns++
		case model.KindCritMeta:
			critMeta++
		case model.KindCastStart:
			castStart++
		case model.KindAffliction:
			affliction++
		case model.KindMiss:
			miss++
		case model.KindAvoid:
			avoid++
		}
	}
	if err := it.Err(); err != nil {
		t.Fatalf("iter error: %v", err)
	}

	const minMelee = 10
	const minNonMelee = 1
	if melee < minMelee {
		t.Fatalf("melee=%d (<%d)", melee, minMelee)
	}
	if nonMelee < minNonMelee {
		t.Fatalf("nonMelee=%d (<%d)", nonMelee, minNonMelee)
	}
	if thorns < 1 {
		t.Fatalf("thorns=%d (<1)", thorns)
	}
	if critMeta < 1 {
		t.Fatalf("critMeta=%d (<1)", critMeta)
	}
	if castStart < 1 {
		t.Fatalf("castStart=%d (<1)", castStart)
	}
	if affliction < 1 {
		t.Fatalf("affliction=%d (<1)", affliction)
	}
	if miss < 1 {
		t.Fatalf("miss=%d (<1)", miss)
	}
	if avoid < 1 {
		t.Fatalf("avoid=%d (<1)", avoid)
	}
	if seenDPSMachine && seenDPS {
		t.Fatalf("unexpected truncated actor %q when %q exists in file", "DPS", "DPS Machine")
	}
}
