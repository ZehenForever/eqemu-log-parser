package engine

import (
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

func TestPlayersSeries_BucketAlignmentAndSums(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "Sigdis")

	// Mark Sigdis and Genaenyu as likely PCs; leave an NPC excluded.
	seg.identityScores = map[string]IdentityScore{
		"Sigdis":   {Name: "Sigdis", Score: DefaultPCThreshold, Class: IdentityLikelyPC},
		"Genaenyu": {Name: "Genaenyu", Score: DefaultPCThreshold, Class: IdentityLikelyPC},
		"a rat":    {Name: "a rat", Score: 0, Class: IdentityLikelyNPC},
	}
	seg.identityDirty = false

	// Events fall into 5s buckets by unix truncation.
	base := time.Unix(100, 0)
	seg.Process(model.Event{Timestamp: base.Add(1 * time.Second), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "Goblin", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: base.Add(4 * time.Second), Kind: model.KindNonMeleeDamage, Actor: "Genaenyu", Target: "Goblin", Amount: 5, AmountKnown: true})
	// Same bucket (100)
	seg.Process(model.Event{Timestamp: base.Add(2 * time.Second), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "Goblin", Amount: 3, AmountKnown: true})
	// Next bucket (105)
	seg.Process(model.Event{Timestamp: base.Add(6 * time.Second), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "Goblin", Amount: 7, AmountKnown: true})
	// Non-PC should be ignored for series.
	seg.Process(model.Event{Timestamp: base.Add(7 * time.Second), Kind: model.KindMeleeDamage, Actor: "a rat", Target: "Sigdis", Amount: 999, AmountKnown: true})

	series := seg.BuildPlayersSeries(time.Unix(200, 0), 5, 100, "all")
	if series.BucketSec != 5 {
		t.Fatalf("bucketSec=%d want=5", series.BucketSec)
	}
	if len(series.Buckets) != 2 {
		t.Fatalf("buckets=%d want=2", len(series.Buckets))
	}

	// Newest first => bucket 105 then 100
	b0 := series.Buckets[0]
	b1 := series.Buckets[1]
	if b0.TotalDamage != 7 {
		t.Fatalf("b0 total=%d want=7", b0.TotalDamage)
	}
	if b0.DamageByActor["Sigdis"] != 7 {
		t.Fatalf("b0 Sigdis=%d want=7", b0.DamageByActor["Sigdis"])
	}
	if _, ok := b0.DamageByActor["a rat"]; ok {
		t.Fatalf("b0 should not include non-PC actor")
	}

	if b1.TotalDamage != 18 {
		t.Fatalf("b1 total=%d want=18", b1.TotalDamage)
	}
	if b1.DamageByActor["Sigdis"] != 13 {
		t.Fatalf("b1 Sigdis=%d want=13", b1.DamageByActor["Sigdis"])
	}
	if b1.DamageByActor["Genaenyu"] != 5 {
		t.Fatalf("b1 Genaenyu=%d want=5", b1.DamageByActor["Genaenyu"])
	}
}

func TestPlayersSeries_EvictionMaxBuckets(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "Sigdis")
	seg.identityScores = map[string]IdentityScore{
		"Sigdis": {Name: "Sigdis", Score: DefaultPCThreshold, Class: IdentityLikelyPC},
	}
	seg.identityDirty = false

	base := time.Unix(100, 0)
	for i := 0; i < 6; i++ {
		seg.Process(model.Event{Timestamp: base.Add(time.Duration(i*5) * time.Second), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "Goblin", Amount: 1, AmountKnown: true})
	}

	series := seg.BuildPlayersSeries(time.Unix(200, 0), 5, 3, "all")
	if len(series.Buckets) != 3 {
		t.Fatalf("buckets=%d want=3", len(series.Buckets))
	}
	// Newest is i=5 (125), oldest retained should be i=3 (115)
	newest := series.Buckets[0]
	oldest := series.Buckets[len(series.Buckets)-1]
	if newest.BucketStart == oldest.BucketStart {
		t.Fatalf("expected different newest/oldest bucket starts")
	}
	if newest.TotalDamage != 1 || oldest.TotalDamage != 1 {
		t.Fatalf("unexpected totals newest=%d oldest=%d", newest.TotalDamage, oldest.TotalDamage)
	}
}

func TestPlayersSeries_ModeMeFiltersActors(t *testing.T) {
	seg := NewEncounterSegmenter(8*time.Second, "Sigdis")
	seg.identityScores = map[string]IdentityScore{
		"Sigdis":   {Name: "Sigdis", Score: DefaultPCThreshold, Class: IdentityLikelyPC},
		"Genaenyu": {Name: "Genaenyu", Score: DefaultPCThreshold, Class: IdentityLikelyPC},
	}
	seg.identityDirty = false

	base := time.Unix(100, 0)
	seg.Process(model.Event{Timestamp: base.Add(1 * time.Second), Kind: model.KindMeleeDamage, Actor: "Sigdis", Target: "Goblin", Amount: 10, AmountKnown: true})
	seg.Process(model.Event{Timestamp: base.Add(2 * time.Second), Kind: model.KindMeleeDamage, Actor: "Genaenyu", Target: "Goblin", Amount: 10, AmountKnown: true})

	series := seg.BuildPlayersSeries(time.Unix(200, 0), 5, 100, "me")
	if len(series.Actors) != 1 || series.Actors[0] != "Sigdis" {
		t.Fatalf("actors=%v want=[Sigdis]", series.Actors)
	}
	if len(series.Buckets) != 1 {
		t.Fatalf("buckets=%d want=1", len(series.Buckets))
	}
	b := series.Buckets[0]
	if _, ok := b.DamageByActor["Genaenyu"]; ok {
		t.Fatalf("expected Genaenyu to be filtered out in me mode")
	}
	if b.DamageByActor["Sigdis"] != 10 {
		t.Fatalf("Sigdis=%d want=10", b.DamageByActor["Sigdis"])
	}
}
