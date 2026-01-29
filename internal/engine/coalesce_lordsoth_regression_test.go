package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
	"github.com/ZehenForever/eqemu-log-parser/internal/parse"
)

type encounterSummary struct {
	target string
	sec    int64
	start  time.Time
	end    time.Time
	total  int64
}

func buildSegmenterFromLogFile(t *testing.T, relPath string) *EncounterSegmenter {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to resolve test file path")
	}
	baseDir := filepath.Dir(thisFile)
	// internal/engine -> repo root -> testdata
	repoRoot := filepath.Clean(filepath.Join(baseDir, "..", ".."))
	p := filepath.Clean(filepath.Join(repoRoot, relPath))
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("failed to open fixture %q: %v", p, err)
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
		t.Fatalf("parse error for %q: %v", p, err)
	}

	// Ensure the segmenter has a stable done set for snapshot building.
	seg.Finalize()
	return seg
}

func summarizeEncounters(encs []EncounterView) []encounterSummary {
	out := make([]encounterSummary, 0, len(encs))
	for _, e := range encs {
		out = append(out, encounterSummary{target: e.Target, sec: e.EncounterSec, start: e.Start, end: e.End, total: e.TotalDamage})
	}
	// Sort by target then start time for readable debug output.
	sort.Slice(out, func(i, j int) bool {
		if out[i].target == out[j].target {
			if out[i].start.Equal(out[j].start) {
				return out[i].end.Before(out[j].end)
			}
			return out[i].start.Before(out[j].start)
		}
		return out[i].target < out[j].target
	})
	return out
}

func formatEncounterSummaries(encs []EncounterView) string {
	summ := summarizeEncounters(encs)
	var b strings.Builder
	for _, s := range summ {
		b.WriteString(fmt.Sprintf("%s\tsec=%d\ttotal=%d\tstart=%s\tend=%s\n", s.target, s.sec, s.total, s.start.Format(time.RFC3339), s.end.Format(time.RFC3339)))
	}
	return b.String()
}

func countTargets(encs []EncounterView, target string) int {
	n := 0
	for _, e := range encs {
		if e.Target == target {
			n++
		}
	}
	return n
}

func findFirstTarget(encs []EncounterView, target string) (EncounterView, bool) {
	for _, e := range encs {
		if e.Target == target {
			return e, true
		}
	}
	return EncounterView{}, false
}

func containsActor(enc EncounterView, actor string) bool {
	for _, a := range enc.Actors {
		if a.Actor == actor {
			return true
		}
	}
	return false
}

func maxTargetSec(encs []EncounterView, target string) int64 {
	var max int64
	for _, e := range encs {
		if e.Target != target {
			continue
		}
		if e.EncounterSec > max {
			max = e.EncounterSec
		}
	}
	return max
}

func TestCoalesce_LordSoth_RegressionFixture(t *testing.T) {
	seg := buildSegmenterFromLogFile(t, filepath.Join("testdata", "eqlog_lordsoth_coalesce.txt"))

	snapOn := seg.BuildSnapshot(time.Now(), "", false, SnapshotOptions{IncludePCTargets: true, LimitEncounters: 0, CoalesceTargets: true})
	snapOff := seg.BuildSnapshot(time.Now(), "", false, SnapshotOptions{IncludePCTargets: true, LimitEncounters: 0, CoalesceTargets: false})

	encOn := snapOn.Encounters
	encOff := snapOff.Encounters

	// Coalescing enabled assertions.
	if got := countTargets(encOn, "Lord Soth"); got != 1 {
		t.Fatalf("coalesce ON: Lord Soth encounters=%d want=1\n--- encounters ---\n%s", got, formatEncounterSummaries(encOn))
	}
	lordOn, ok := findFirstTarget(encOn, "Lord Soth")
	if !ok {
		t.Fatalf("coalesce ON: expected to find Lord Soth\n--- encounters ---\n%s", formatEncounterSummaries(encOn))
	}
	if lordOn.EncounterSec < 600 || lordOn.EncounterSec > 1100 {
		t.Fatalf("coalesce ON: Lord Soth EncounterSec=%d want in [600,1100]\n--- encounters ---\n%s", lordOn.EncounterSec, formatEncounterSummaries(encOn))
	}
	if lordOn.TotalDamage <= 0 {
		t.Fatalf("coalesce ON: Lord Soth TotalDamage=%d want > 0\n--- encounters ---\n%s", lordOn.TotalDamage, formatEncounterSummaries(encOn))
	}
	if !containsActor(lordOn, "Sigdis") {
		t.Fatalf("coalesce ON: expected actor Sigdis in Lord Soth\n--- encounters ---\n%s", formatEncounterSummaries(encOn))
	}
	if !containsActor(lordOn, "Genaenyu") {
		t.Fatalf("coalesce ON: expected actor Genaenyu in Lord Soth\n--- encounters ---\n%s", formatEncounterSummaries(encOn))
	}
	if got := countTargets(encOn, "A Crocodile"); got < 1 {
		t.Fatalf("coalesce ON: expected at least one A Crocodile encounter\n--- encounters ---\n%s", formatEncounterSummaries(encOn))
	}
	if len(encOn) <= 1 {
		t.Fatalf("coalesce ON: expected total encounter count > 1 (guard against over-merge)\n--- encounters ---\n%s", formatEncounterSummaries(encOn))
	}

	// Coalescing disabled assertions.
	if got := countTargets(encOff, "Lord Soth"); got < 2 {
		t.Fatalf("coalesce OFF: Lord Soth encounters=%d want >= 2\n--- encounters ---\n%s", got, formatEncounterSummaries(encOff))
	}
	maxOff := maxTargetSec(encOff, "Lord Soth")
	if maxOff >= lordOn.EncounterSec {
		t.Fatalf("coalesce OFF: max Lord Soth segment sec=%d want < coalesced sec=%d\n--- encounters ---\n%s", maxOff, lordOn.EncounterSec, formatEncounterSummaries(encOff))
	}
}
