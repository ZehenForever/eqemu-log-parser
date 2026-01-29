package model

import "time"

type EventKind uint8

const (
	KindUnknown EventKind = iota
	KindMeleeDamage
	KindNonMeleeDamage
	KindMiss
	KindAvoid
	KindCritMeta
	KindCastStart
	KindAffliction
	KindHeal
	KindThornsMarker
	KindDeath
	KindZoneOrSystem
	KindIncomingDamage
)

type DamageClass uint8

const (
	DamageClassUnknown DamageClass = iota
	DamageClassPierce
	DamageClassSlash
	DamageClassCrush
	DamageClassBash
	DamageClassKick
	DamageClassDirect
)

type Event struct {
	Timestamp    time.Time
	Raw          string
	Kind         EventKind
	DamageClass  DamageClass
	Actor        string
	Target       string
	SpellOrSkill string
	Verb         string
	Amount       int64
	AmountKnown  bool
	Crit         bool
	MetaInt      int64
}

type ParseContext struct {
	LocalActorName string
	PendingCrit    *PendingCrit
}

type PendingCrit struct {
	Actor string
	Ts    time.Time
	Value int64
	TTL   int
}
