package engine

import (
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

func TestEncounterActorStats_SDPS_UsesActorActiveSeconds(t *testing.T) {
	seg := NewEncounterSegmenter(100*time.Second, "")
	base := time.Date(2026, 1, 23, 7, 0, 0, 0, time.Local)

	// Encounter seconds: 10 (t=0..9 inclusive)
	// Actor A active: 10 (t=0..9)
	// Actor B active: 7  (t=3..9)
	events := []model.Event{
		{Timestamp: base.Add(0 * time.Second), Kind: model.KindMeleeDamage, Actor: "A", Target: "Mob", Amount: 100, AmountKnown: true},
		{Timestamp: base.Add(3 * time.Second), Kind: model.KindMeleeDamage, Actor: "B", Target: "Mob", Amount: 100, AmountKnown: true},
		{Timestamp: base.Add(9 * time.Second), Kind: model.KindMeleeDamage, Actor: "A", Target: "Mob", Amount: 900, AmountKnown: true},
		{Timestamp: base.Add(9 * time.Second), Kind: model.KindMeleeDamage, Actor: "B", Target: "Mob", Amount: 600, AmountKnown: true},
	}
	for _, ev := range events {
		seg.Process(ev)
	}

	encs := seg.Finalize()
	if len(encs) != 1 {
		t.Fatalf("encounters=%d want=1", len(encs))
	}
	enc := encs[0]
	if got := enc.DurationSeconds(); got != 10 {
		t.Fatalf("EncounterSeconds=%.0f want=10", got)
	}

	stA := enc.ByActor["A"]
	if stA == nil {
		t.Fatalf("missing actor A")
	}
	if got := stA.ActiveSeconds(); got != 10 {
		t.Fatalf("A active=%.0f want=10", got)
	}

	stB := enc.ByActor["B"]
	if stB == nil {
		t.Fatalf("missing actor B")
	}
	if got := stB.ActiveSeconds(); got != 7 {
		t.Fatalf("B active=%.0f want=7", got)
	}

	dpsEncB := float64(stB.Total) / enc.DurationSeconds()
	sdpsB := float64(stB.Total) / stB.ActiveSeconds()
	if dpsEncB == sdpsB {
		t.Fatalf("expected DPS(enc)=%.3f and SDPS=%.3f to differ", dpsEncB, sdpsB)
	}
	if sdpsB <= dpsEncB {
		t.Fatalf("expected SDPS %.3f > DPS(enc) %.3f", sdpsB, dpsEncB)
	}
}

func TestEncounterSDPS_FullTestdata_DPSMachineSecondEncounter_EmbervalActiveSeconds(t *testing.T) {
	encs := segmentEncountersFromFile(t, false, DefaultPCThreshold, nil, nil)

	var target *Encounter
	for _, enc := range encs {
		if enc.Target == "DPS Machine" && enc.DurationSeconds() == 50 {
			target = enc
			break
		}
	}
	if target == nil {
		t.Fatalf("expected DPS Machine encounter with EncounterSeconds=50")
	}

	st := target.ByActor["Emberval"]
	if st == nil {
		t.Fatalf("expected Emberval in DPS Machine encounter")
	}
	if got := st.ActiveSeconds(); got != 47 {
		t.Fatalf("Emberval active=%.0f want=47", got)
	}

	dpsEnc := float64(st.Total) / target.DurationSeconds()
	sdps := float64(st.Total) / st.ActiveSeconds()
	if sdps <= dpsEnc {
		t.Fatalf("expected SDPS %.3f > DPS(enc) %.3f", sdps, dpsEnc)
	}
}
