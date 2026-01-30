package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
	"github.com/ZehenForever/eqemu-log-parser/internal/parse"
)

func TestRoomPlayersActors_FilterLordHydreriousFixture(t *testing.T) {
	p := filepath.Join("..", "..", "testdata", "encounter_LordHydrerious.txt")
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	// This fixture isn't an eqlog_*.txt file; hardcode a stable local name.
	localPlayer := "LOCAL"
	ctx := &model.ParseContext{LocalActorName: localPlayer}

	r := newRoom("test", "x")
	serverRecv := int64(20_000)

	scanner := bufio.NewScanner(f)
	// accommodate longer EQ log lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	published := 0
	nonPcParsedActors := make(map[string]int)
	nonPcExampleLines := make(map[string]string)

	for scanner.Scan() {
		line := scanner.Text()
		ev, ok := parse.ParseLine(ctx, line, time.Local)
		if !ok {
			continue
		}
		if !ev.AmountKnown {
			continue
		}
		if ev.Kind != model.KindMeleeDamage && ev.Kind != model.KindNonMeleeDamage {
			continue
		}

		actor := ev.Actor
		if actor == "YOU" {
			actor = localPlayer
		}

		kind := ""
		switch ev.Kind {
		case model.KindMeleeDamage:
			kind = "melee"
		case model.KindNonMeleeDamage:
			kind = "nonmelee"
		default:
			continue
		}

		// Track cases where parsing produced a non-PC-looking actor; hub must drop them.
		if !isPCLikeActorName(actor) {
			nonPcParsedActors[actor]++
			if _, exists := nonPcExampleLines[actor]; !exists {
				nonPcExampleLines[actor] = line
			}
		}

		r.IngestBatch(serverRecv, PublishBatchRequest{
			PublisherID:  "p1",
			SentAtUnixMs: serverRecv,
			Events: []DamageEvent{
				{
					TsUnixMs: ev.Timestamp.UnixMilli(),
					Actor:    actor,
					Target:   ev.Target,
					Kind:     kind,
					Verb:     ev.Verb,
					Amount:   ev.Amount,
					Crit:     ev.Crit,
				},
			},
		})
		published++
		serverRecv++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	if published <= 0 {
		t.Fatalf("no publishable damage events parsed from fixture")
	}

	snap := r.Snapshot()
	if len(r.order) == 0 {
		t.Fatalf("no buckets created from fixture events")
	}

	actors := snap.Actors
	if len(actors) == 0 {
		t.Fatalf("no actors emitted in snapshot")
	}

	// Invariants:
	// - Must not contain known bad strings.
	// - Must not contain spaces.
	// - Must not contain entries ending with " was" (case-insensitive).
	// - Must not contain YOU.
	badSuffixWas := regexp.MustCompile(`(?i)\swas$`)
	pcLike := regexp.MustCompile(`^[A-Z][A-Za-z'\-]{2,20}$`)

	seenPcLike := false
	for _, a := range actors {
		if a == "YOU" {
			t.Fatalf("unexpected actor in snapshot: %q", a)
		}
		if a == "Lord Hydrerious" || a == "Lord Hydrerious was" {
			t.Fatalf("unexpected NPC actor in snapshot: %q", a)
		}
		if strings.Contains(a, " ") {
			t.Fatalf("unexpected multi-word actor in snapshot: %q", a)
		}
		if badSuffixWas.MatchString(a) {
			t.Fatalf("unexpected actor ending with ' was' in snapshot: %q", a)
		}
		if pcLike.MatchString(a) {
			seenPcLike = true
		}
	}
	if !seenPcLike {
		t.Fatalf("expected at least one PC-like actor in snapshot actors; got=%v", actors)
	}

	// Optional debug output on failure would go above; keep quiet on pass.
	_ = nonPcParsedActors
	_ = nonPcExampleLines
}
