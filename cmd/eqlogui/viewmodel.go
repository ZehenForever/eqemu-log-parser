package main

import (
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/engine"
)

type ActorStatsViewUI struct {
	Actor     string  `json:"actor"`
	Melee     int64   `json:"melee"`
	NonMelee  int64   `json:"nonMelee"`
	Total     int64   `json:"total"`
	DPSEnc    float64 `json:"dpsEncounter"`
	SDPS      float64 `json:"sdps"`
	ActiveSec int64   `json:"activeSec"`
	PctTotal  float64 `json:"pctTotal"`
	Hits      int64   `json:"hits"`
	MaxHit    int64   `json:"maxHit"`
	AvgHit    float64 `json:"avgHit"`
	CritPct   float64 `json:"critPct"`
	AvgCrit   float64 `json:"avgCrit"`
	Crits     int64   `json:"crits"`
}

func DamageBreakdownViewToUI(v engine.DamageBreakdownView) DamageBreakdownViewUI {
	out := DamageBreakdownViewUI{
		EncounterID: v.EncounterID,
		Target:      v.Target,
		Actor:       v.Actor,
		Rows:        make([]DamageBreakdownRowViewUI, 0, len(v.Rows)),
	}
	for _, r := range v.Rows {
		out.Rows = append(out.Rows, DamageBreakdownRowViewUI{
			Name:      r.Name,
			PctPlayer: r.PctPlayer,
			Damage:    r.Damage,
			DPS:       r.DPS,
			SDPS:      r.SDPS,
			Sec:       r.Sec,
			Hits:      r.Hits,
			MaxHit:    r.MaxHit,
			MinHit:    r.MinHit,
			AvgHit:    r.AvgHit,
			CritPct:   r.CritPct,
			AvgCrit:   r.AvgCrit,
		})
	}
	return out
}

type EncounterViewUI struct {
	EncounterKey string             `json:"encounterKey"`
	EncounterID  string             `json:"encounterId"`
	Target       string             `json:"target"`
	Start        string             `json:"start"`
	End          string             `json:"end"`
	EncounterSec int64              `json:"encounterSec"`
	TotalDamage  int64              `json:"totalDamage"`
	DPSEncounter float64            `json:"dpsEncounter"`
	Actors       []ActorStatsViewUI `json:"actors"`
}

type SnapshotUI struct {
	Now            string            `json:"now"`
	FilePath       string            `json:"filePath"`
	Tailing        bool              `json:"tailing"`
	LastHours      float64           `json:"lastHours"`
	EncounterCount int               `json:"encounterCount"`
	Encounters     []EncounterViewUI `json:"encounters"`
}

type PlayerBucketUI struct {
	BucketStart   string           `json:"bucketStart"`
	BucketSec     int64            `json:"bucketSec"`
	DamageByActor map[string]int64 `json:"damageByActor"`
	TotalDamage   int64            `json:"totalDamage"`
}

type PlayersSeriesUI struct {
	Now        string           `json:"now"`
	BucketSec  int64            `json:"bucketSec"`
	MaxBuckets int              `json:"maxBuckets"`
	Actors     []string         `json:"actors"`
	Buckets    []PlayerBucketUI `json:"buckets"`
}

type RoomSummaryUI struct {
	RoomID          string `json:"roomId"`
	LastSeen        string `json:"lastSeen"`
	PublisherCount  int    `json:"publisherCount"`
	SubscriberCount int    `json:"subscriberCount"`
	BucketSec       int    `json:"bucketSec"`
}

type RoomListUI struct {
	Rooms []RoomSummaryUI `json:"rooms"`
}

type PublishingStatusUI struct {
	Enabled    bool   `json:"enabled"`
	LastError  string `json:"lastError"`
	SentEvents int64  `json:"sentEvents"`
}

type SubscribeStatusUI struct {
	Enabled       bool   `json:"enabled"`
	Connected     bool   `json:"connected"`
	LastError     string `json:"lastError"`
	RoomID        string `json:"roomId"`
	ReconnectInMs int64  `json:"reconnectInMs"`
}

type ConfigDefaultsUI struct {
	HubURL      string `json:"hubUrl"`
	RoomID      string `json:"roomId"`
	Token       string `json:"token"`
	ConfigPath  string `json:"configPath"`
	ConfigError string `json:"configError"`
}

type DamageBreakdownRowViewUI struct {
	Name      string  `json:"name"`
	PctPlayer float64 `json:"pctPlayer"`
	Damage    int64   `json:"damage"`
	DPS       float64 `json:"dpsEncounter"`
	SDPS      float64 `json:"sdps"`
	Sec       int64   `json:"sec"`
	Hits      int64   `json:"hits"`
	MaxHit    int64   `json:"maxHit"`
	MinHit    int64   `json:"minHit"`
	AvgHit    float64 `json:"avgHit"`
	CritPct   float64 `json:"critPct"`
	AvgCrit   float64 `json:"avgCrit"`
}

type DamageBreakdownViewUI struct {
	EncounterID string                     `json:"encounterId"`
	Target      string                     `json:"target"`
	Actor       string                     `json:"actor"`
	Rows        []DamageBreakdownRowViewUI `json:"rows"`
}

func EncounterViewToUI(e engine.EncounterView) EncounterViewUI {
	enc := EncounterViewUI{
		EncounterKey: e.EncounterKey,
		EncounterID:  e.EncounterID,
		Target:       e.Target,
		Start:        e.Start.Format(time.RFC3339),
		End:          e.End.Format(time.RFC3339),
		EncounterSec: e.EncounterSec,
		TotalDamage:  e.TotalDamage,
		DPSEncounter: e.DPSEncounter,
		Actors:       make([]ActorStatsViewUI, 0, len(e.Actors)),
	}
	for _, a := range e.Actors {
		enc.Actors = append(enc.Actors, ActorStatsViewUI{
			Actor:     a.Actor,
			Melee:     a.Melee,
			NonMelee:  a.NonMelee,
			Total:     a.Total,
			DPSEnc:    a.DPS,
			SDPS:      a.SDPS,
			ActiveSec: a.ActiveSec,
			PctTotal:  a.PctTotal,
			Hits:      a.Hits,
			MaxHit:    a.MaxHit,
			AvgHit:    a.AvgHit,
			CritPct:   a.CritPct,
			AvgCrit:   a.AvgCrit,
			Crits:     a.Crits,
		})
	}
	return enc
}

func SnapshotToUISummary(s engine.Snapshot) SnapshotUI {
	out := SnapshotUI{
		Now:            s.Now.Format(time.RFC3339),
		FilePath:       s.FilePath,
		Tailing:        s.Tailing,
		EncounterCount: s.EncounterCount,
		Encounters:     make([]EncounterViewUI, 0, len(s.Encounters)),
	}
	for _, e := range s.Encounters {
		out.Encounters = append(out.Encounters, EncounterViewUI{
			EncounterKey: e.EncounterKey,
			EncounterID:  e.EncounterID,
			Target:       e.Target,
			Start:        e.Start.Format(time.RFC3339),
			End:          e.End.Format(time.RFC3339),
			EncounterSec: e.EncounterSec,
			TotalDamage:  e.TotalDamage,
			DPSEncounter: e.DPSEncounter,
			Actors:       nil,
		})
	}
	return out
}

func snapshotToUI(s engine.Snapshot) SnapshotUI {
	out := SnapshotUI{
		Now:            s.Now.Format(time.RFC3339),
		FilePath:       s.FilePath,
		Tailing:        s.Tailing,
		EncounterCount: s.EncounterCount,
		Encounters:     make([]EncounterViewUI, 0, len(s.Encounters)),
	}
	for _, e := range s.Encounters {
		enc := EncounterViewUI{
			EncounterKey: e.EncounterKey,
			EncounterID:  e.EncounterID,
			Target:       e.Target,
			Start:        e.Start.Format(time.RFC3339),
			End:          e.End.Format(time.RFC3339),
			EncounterSec: e.EncounterSec,
			TotalDamage:  e.TotalDamage,
			DPSEncounter: e.DPSEncounter,
			Actors:       make([]ActorStatsViewUI, 0, len(e.Actors)),
		}
		for _, a := range e.Actors {
			enc.Actors = append(enc.Actors, ActorStatsViewUI{
				Actor:     a.Actor,
				Melee:     a.Melee,
				NonMelee:  a.NonMelee,
				Total:     a.Total,
				DPSEnc:    a.DPS,
				SDPS:      a.SDPS,
				ActiveSec: a.ActiveSec,
				PctTotal:  a.PctTotal,
				Hits:      a.Hits,
				MaxHit:    a.MaxHit,
				AvgHit:    a.AvgHit,
				CritPct:   a.CritPct,
				AvgCrit:   a.AvgCrit,
				Crits:     a.Crits,
			})
		}
		out.Encounters = append(out.Encounters, enc)
	}
	return out
}

func SnapshotToUI(s engine.Snapshot) SnapshotUI {
	return snapshotToUI(s)
}
