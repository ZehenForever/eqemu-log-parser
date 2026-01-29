package main

type BucketSnapshotEntry struct {
	BucketStartUnixMs int64            `json:"bucketStartUnixMs"`
	DamageByActor     map[string]int64 `json:"damageByActor"`
	TotalDamage       int64            `json:"totalDamage"`
}

type BucketUpdateMessage struct {
	Type              string           `json:"type"`
	BucketSec         int64            `json:"bucketSec"`
	BucketStartUnixMs int64            `json:"bucketStartUnixMs"`
	DamageByActor     map[string]int64 `json:"damageByActor"`
	TotalDamage       int64            `json:"totalDamage"`
}

type BucketSnapshotMessage struct {
	Type      string                `json:"type"`
	BucketSec int64                 `json:"bucketSec"`
	Buckets   []BucketSnapshotEntry `json:"buckets"`
	Actors    []string              `json:"actors"`
}

type DamageEvent struct {
	TsUnixMs int64  `json:"tsUnixMs"`
	Actor    string `json:"actor"`
	Target   string `json:"target"`
	Kind     string `json:"kind"` // "melee" | "nonmelee"
	Verb     string `json:"verb"`
	Amount   int64  `json:"amount"`
	Crit     bool   `json:"crit"`
}

type PublishBatchRequest struct {
	PublisherID  string        `json:"publisherId"`
	SentAtUnixMs int64         `json:"sentAtUnixMs"`
	Events       []DamageEvent `json:"events"`
}

type OkResponse struct {
	Ok bool `json:"ok"`
}

type RoomSummary struct {
	RoomID          string `json:"roomId"`
	LastSeenUnixMs  int64  `json:"lastSeenUnixMs"`
	PublisherCount  int    `json:"publisherCount"`
	SubscriberCount int    `json:"subscriberCount"`
	BucketSec       int    `json:"bucketSec"`
}

type RoomsListResponse struct {
	Rooms []RoomSummary `json:"rooms"`
}
