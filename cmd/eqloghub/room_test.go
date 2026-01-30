package main

import "testing"

func TestIsPCLikeActorName(t *testing.T) {
	if isPCLikeActorName("Sigdis") != true {
		t.Fatalf("Sigdis should be pc-like")
	}
	if isPCLikeActorName("Emberval") != true {
		t.Fatalf("Emberval should be pc-like")
	}
	if isPCLikeActorName("Lord Hydrerious") != false {
		t.Fatalf("multi-word NPC should not be pc-like")
	}
	if isPCLikeActorName("Lord Hydrerious was") != false {
		t.Fatalf("phrase fragment should not be pc-like")
	}
	if isPCLikeActorName("a training dummy") != false {
		t.Fatalf("lowercase multi-word NPC should not be pc-like")
	}
}

func TestRoom_IgnoresNonPcActorEvents(t *testing.T) {
	r := newRoom("r1", "t1")
	serverRecv := int64(20_000)

	r.IngestBatch(serverRecv, PublishBatchRequest{
		PublisherID:  "p1",
		SentAtUnixMs: serverRecv,
		Events: []DamageEvent{
			{TsUnixMs: 10_100, Actor: "Lord Hydrerious", Target: "Sigdis", Kind: "melee", Verb: "hits", Amount: 50},
			{TsUnixMs: 10_200, Actor: "Lord Hydrerious was", Target: "Sigdis", Kind: "melee", Verb: "hits", Amount: 60},
			{TsUnixMs: 10_300, Actor: "Sigdis", Target: "a rat", Kind: "melee", Verb: "slashes", Amount: 10},
		},
	})

	if len(r.order) != 1 {
		t.Fatalf("bucket count=%d want=1", len(r.order))
	}
	bs := r.order[0]
	agg := r.buckets[bs]
	if agg == nil {
		t.Fatalf("missing bucket")
	}
	if agg.totalDamage != 10 {
		t.Fatalf("totalDamage=%d want=10", agg.totalDamage)
	}
	if agg.damageByActor["Sigdis"] != 10 {
		t.Fatalf("Sigdis=%d want=10", agg.damageByActor["Sigdis"])
	}
	if _, ok := agg.damageByActor["Lord Hydrerious"]; ok {
		t.Fatalf("unexpected NPC actor in bucket")
	}
	if _, ok := agg.damageByActor["Lord Hydrerious was"]; ok {
		t.Fatalf("unexpected NPC fragment actor in bucket")
	}

	snap := r.Snapshot()
	for _, a := range snap.Actors {
		if a == "Lord Hydrerious" || a == "Lord Hydrerious was" {
			t.Fatalf("unexpected NPC actor in snapshot actors list: %q", a)
		}
	}
}

func TestRoom_DedupeAcrossPublishers(t *testing.T) {
	r := newRoom("r1", "t1")
	serverRecv := int64(20_000)

	ev := DamageEvent{
		TsUnixMs: 10_500,
		Actor:    "Sigdis",
		Target:   "a rat",
		Kind:     "melee",
		Verb:     "slashes",
		Amount:   100,
		Crit:     false,
	}

	r.IngestBatch(serverRecv, PublishBatchRequest{PublisherID: "p1", SentAtUnixMs: serverRecv, Events: []DamageEvent{ev}})
	r.IngestBatch(serverRecv+5, PublishBatchRequest{PublisherID: "p2", SentAtUnixMs: serverRecv + 5, Events: []DamageEvent{ev}})

	if len(r.order) != 1 {
		t.Fatalf("bucket count=%d want=1", len(r.order))
	}
	bs := r.order[0]
	agg := r.buckets[bs]
	if agg == nil {
		t.Fatalf("missing bucket")
	}
	if agg.totalDamage != 100 {
		t.Fatalf("totalDamage=%d want=100", agg.totalDamage)
	}
	if agg.damageByActor["Sigdis"] != 100 {
		t.Fatalf("Sigdis=%d want=100", agg.damageByActor["Sigdis"])
	}
}

func TestRoom_BucketingAccumulatesWithin5s(t *testing.T) {
	r := newRoom("r1", "t1")
	serverRecv := int64(50_000)

	r.IngestBatch(serverRecv, PublishBatchRequest{
		PublisherID:  "p1",
		SentAtUnixMs: serverRecv,
		Events: []DamageEvent{
			{TsUnixMs: 10_100, Actor: "Sigdis", Target: "a rat", Kind: "melee", Verb: "slashes", Amount: 10},
			{TsUnixMs: 10_400, Actor: "Sigdis", Target: "a rat", Kind: "melee", Verb: "slashes", Amount: 20},
		},
	})

	if len(r.order) != 1 {
		t.Fatalf("bucket count=%d want=1", len(r.order))
	}
	bs := r.order[0]
	if bs != 10_000 {
		t.Fatalf("bucketStart=%d want=10000", bs)
	}
	agg := r.buckets[bs]
	if agg.totalDamage != 30 {
		t.Fatalf("totalDamage=%d want=30", agg.totalDamage)
	}
}

func TestRoom_OffsetAlignsPublishersToSameBucket(t *testing.T) {
	r := newRoom("r1", "t1")
	serverRecv := int64(20_000)

	// Publisher p1: offset=0, event at 12s.
	r.IngestBatch(serverRecv, PublishBatchRequest{
		PublisherID:  "p1",
		SentAtUnixMs: serverRecv,
		Events: []DamageEvent{
			{TsUnixMs: 12_000, Actor: "Sigdis", Target: "a rat", Kind: "melee", Verb: "slashes", Amount: 10},
		},
	})

	// Publisher p2: sentAt is 1s behind, so offset~=+1000ms.
	// Event at 11s should adjust to 12s.
	r.IngestBatch(serverRecv, PublishBatchRequest{
		PublisherID:  "p2",
		SentAtUnixMs: serverRecv - 1000,
		Events: []DamageEvent{
			{TsUnixMs: 11_000, Actor: "Genaenyu", Target: "a rat", Kind: "melee", Verb: "pierces", Amount: 20},
		},
	})

	if len(r.order) != 1 {
		t.Fatalf("bucket count=%d want=1", len(r.order))
	}
	bs := r.order[0]
	if bs != 10_000 {
		t.Fatalf("bucketStart=%d want=10000", bs)
	}
	agg := r.buckets[bs]
	if agg.totalDamage != 30 {
		t.Fatalf("totalDamage=%d want=30", agg.totalDamage)
	}
	if agg.damageByActor["Sigdis"] != 10 {
		t.Fatalf("Sigdis=%d want=10", agg.damageByActor["Sigdis"])
	}
	if agg.damageByActor["Genaenyu"] != 20 {
		t.Fatalf("Genaenyu=%d want=20", agg.damageByActor["Genaenyu"])
	}
}
