package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
	"github.com/ZehenForever/eqemu-log-parser/internal/parse"
)

func TestClassifyNames_Innoruuk_TargetOnly_IsUnknown(t *testing.T) {
	events := []model.Event{{
		Timestamp:   time.Date(2026, 1, 23, 7, 0, 0, 0, time.Local),
		Kind:        model.KindMeleeDamage,
		Actor:       "Emberval",
		Target:      "Innoruuk",
		Amount:      10,
		AmountKnown: true,
	}}
	s := ClassifyNames(events)
	sc := s["Innoruuk"]
	if sc.Name != "Innoruuk" {
		t.Fatalf("missing Innoruuk")
	}
	if sc.Class != IdentityUnknown {
		t.Fatalf("class=%v score=%d reasons=%v", sc.Class, sc.Score, sc.Reasons)
	}
}

func TestClassifyNames_FullTestdata_SigdisIsLikelyPC(t *testing.T) {
	p := filepath.Join("..", "..", "testdata", "eqlog_Emberval_Imperium_EQ.txt")
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("testdata file not present: %v", err)
	}
	defer f.Close()

	playerName, _ := parse.PlayerNameFromLogPath(p)
	ctx := &model.ParseContext{LocalActorName: playerName}
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

	s := ClassifyNames(events)
	sc := s["Sigdis"]
	if sc.Name != "Sigdis" {
		t.Fatalf("missing Sigdis")
	}
	if sc.Class != IdentityLikelyPC {
		t.Fatalf("class=%v score=%d reasons=%v", sc.Class, sc.Score, sc.Reasons)
	}
}

func TestApplyIdentityOverrides_ForceNPC_Innoruuk(t *testing.T) {
	s := map[string]IdentityScore{
		"Innoruuk": {Name: "Innoruuk", Score: DefaultPCThreshold, Reasons: []string{"single_token", "initial_cap", "pc_regex"}},
	}
	forceNPC := map[string]struct{}{"Innoruuk": {}}
	ApplyIdentityOverrides(s, DefaultPCThreshold, nil, forceNPC)
	if s["Innoruuk"].Class != IdentityLikelyNPC {
		t.Fatalf("class=%v", s["Innoruuk"].Class)
	}
}

func TestApplyIdentityOverrides_ForceNPCWins(t *testing.T) {
	s := map[string]IdentityScore{
		"Sigdis": {Name: "Sigdis", Score: 99, Reasons: []string{"single_token"}},
	}
	forcePC := map[string]struct{}{"Sigdis": {}}
	forceNPC := map[string]struct{}{"Sigdis": {}}
	ApplyIdentityOverrides(s, DefaultPCThreshold, forcePC, forceNPC)
	if s["Sigdis"].Class != IdentityLikelyNPC {
		t.Fatalf("class=%v", s["Sigdis"].Class)
	}
}

func TestEncounterSegmentation_ForcePCWithHighThreshold_ExcludesSigdis(t *testing.T) {
	forcePC := map[string]struct{}{"Sigdis": {}}
	encs := segmentEncountersFromFile(t, false, 100, forcePC, nil)
	for _, enc := range encs {
		if enc.Target == "Sigdis" {
			t.Fatalf("expected Sigdis excluded")
		}
	}
}

func TestEncounterSegmentation_ForceNPC_AllowsSigdisEvenIfWouldBePC(t *testing.T) {
	forceNPC := map[string]struct{}{"Sigdis": {}}
	encs := segmentEncountersFromFile(t, false, DefaultPCThreshold, nil, forceNPC)
	foundSigdis := false
	for _, enc := range encs {
		if enc.Target == "Sigdis" {
			foundSigdis = true
			break
		}
	}
	if !foundSigdis {
		t.Fatalf("expected Sigdis included when forced NPC")
	}
}
