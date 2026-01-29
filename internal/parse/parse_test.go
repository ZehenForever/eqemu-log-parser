package parse

import (
	"testing"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

func TestParseLine_YouMelee(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] You pierce a training dummy for 7239 points of damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindMeleeDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.DamageClass != model.DamageClassPierce {
		t.Fatalf("damageClass=%v", ev.DamageClass)
	}
	if ev.Actor != "YOU" || ev.Target != "a training dummy" {
		t.Fatalf("actor/target=%q/%q", ev.Actor, ev.Target)
	}
	if ev.Amount != 7239 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
	if ev.Timestamp.IsZero() {
		t.Fatalf("timestamp zero")
	}
}

func TestParseLine_NonMelee(t *testing.T) {
	line := "[Fri Jan 23 07:46:03 2026] Emberval hit a training dummy for 1920 points of non-melee damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindNonMeleeDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.DamageClass != model.DamageClassDirect {
		t.Fatalf("damageClass=%v", ev.DamageClass)
	}
	if ev.Actor != "Emberval" || ev.Target != "a training dummy" {
		t.Fatalf("actor/target=%q/%q", ev.Actor, ev.Target)
	}
	if ev.Amount != 1920 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
}

func TestParseLine_ActorMelee_MultiWordActor(t *testing.T) {
	line := "[Fri Jan 23 07:53:49 2026] Sigdis crushes DPS Machine for 359 points of damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindMeleeDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.DamageClass != model.DamageClassCrush {
		t.Fatalf("damageClass=%v", ev.DamageClass)
	}
	if ev.Actor != "Sigdis" {
		t.Fatalf("actor=%q", ev.Actor)
	}
	if ev.Target != "DPS Machine" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.Amount != 359 {
		t.Fatalf("amount=%d", ev.Amount)
	}
}

func TestParseLine_YouMelee_SlashDamageClass(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] You slash a training dummy for 123 points of damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindMeleeDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.DamageClass != model.DamageClassSlash {
		t.Fatalf("damageClass=%v", ev.DamageClass)
	}
}

func TestParseLine_OtherMelee_MultiWordMobActor(t *testing.T) {
	line := "[Fri Jan 23 07:53:49 2026] DPS Machine hits Sigdis for 202 points of damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindMeleeDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Actor != "DPS Machine" {
		t.Fatalf("actor=%q", ev.Actor)
	}
	if ev.Verb != "hits" {
		t.Fatalf("verb=%q", ev.Verb)
	}
	if ev.Target != "Sigdis" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.Amount != 202 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
}

func TestParseLine_Miss(t *testing.T) {
	line := "[Fri Jan 23 07:46:03 2026] You try to pierce a training dummy, but miss!"
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindMiss {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Actor != "YOU" || ev.Target != "a training dummy" {
		t.Fatalf("actor/target=%q/%q", ev.Actor, ev.Target)
	}
}

func TestParseLine_Avoid(t *testing.T) {
	line := "[Fri Jan 23 07:53:49 2026] DPS Machine tries to hit Sigdis, but Sigdis dodges!"
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindAvoid {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Actor != "DPS Machine" || ev.Target != "Sigdis" {
		t.Fatalf("actor/target=%q/%q", ev.Actor, ev.Target)
	}
}

func TestParseLine_CritMeta(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] Emberval scores a critical hit! (7138)"
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindCritMeta {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Actor != "Emberval" {
		t.Fatalf("actor=%q", ev.Actor)
	}
	if ev.MetaInt != 7138 {
		t.Fatalf("metaint=%d", ev.MetaInt)
	}
}

func TestParseLine_CritMeta_YouBlast(t *testing.T) {
	line := "[Fri Jan 23 07:46:07 2026] You deliver a critical blast! (3452)"
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindCritMeta {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Actor != "YOU" {
		t.Fatalf("actor=%q", ev.Actor)
	}
	if ev.MetaInt != 3452 {
		t.Fatalf("metaint=%d", ev.MetaInt)
	}
}

func TestParseLine_Thorns(t *testing.T) {
	line := "[Fri Jan 23 07:53:49 2026] DPS Machine was pierced by thorns."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindThornsMarker {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Target != "DPS Machine" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.AmountKnown {
		t.Fatalf("expected unknown amount")
	}
}

func TestParseLine_CastStart(t *testing.T) {
	line := "[Fri Jan 23 07:47:01 2026] You begin casting Bite of the Shissar Poison VII."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindCastStart {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Actor != "YOU" || ev.SpellOrSkill != "Bite of the Shissar Poison VII" {
		t.Fatalf("actor/spell=%q/%q", ev.Actor, ev.SpellOrSkill)
	}
}

func TestParseLine_Affliction(t *testing.T) {
	line := "[Fri Jan 23 07:47:09 2026] A training dummy is afflicted by poison."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindAffliction {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Target != "A training dummy" || ev.SpellOrSkill != "poison" {
		t.Fatalf("target/spell=%q/%q", ev.Target, ev.SpellOrSkill)
	}
}

func TestParseLine_AutoAttackToggle(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] Auto attack is on."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindZoneOrSystem {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.SpellOrSkill != "auto_attack" || ev.Verb != "on" {
		t.Fatalf("spell/verb=%q/%q", ev.SpellOrSkill, ev.Verb)
	}
}

func TestParseLine_Heal_You(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] You have been healed for 1234 points."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindHeal {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Target != "YOU" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.Amount != 1234 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
}

func TestParseLine_Heal_Target(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] Sigdis has been healed for 4321 points."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindHeal {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Target != "Sigdis" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.Amount != 4321 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
}

func TestParseLine_IncomingDamage_ByNonMelee(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] You have taken 55 points of damage by non-melee."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindIncomingDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Target != "YOU" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.Amount != 55 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
}

func TestParseLine_IncomingDamage_NonMelee(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] You have taken 66 points of non-melee damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindIncomingDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Target != "YOU" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.Amount != 66 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
}

func TestParseLine_IncomingDamage_OnTarget_HitsOnYOU(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] DPS Machine hits on YOU for 231 points of damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindIncomingDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Actor != "DPS Machine" {
		t.Fatalf("actor=%q", ev.Actor)
	}
	if ev.Target != "YOU" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.Verb != "hits" {
		t.Fatalf("verb=%q", ev.Verb)
	}
	if ev.Amount != 231 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
}

func TestParseLine_IncomingDamage_OnTarget_HitOnPC(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] DPS Machine hit on Sigdis for 389 points of damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindIncomingDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if ev.Actor != "DPS Machine" {
		t.Fatalf("actor=%q", ev.Actor)
	}
	if ev.Target != "Sigdis" {
		t.Fatalf("target=%q", ev.Target)
	}
	if ev.Verb != "hit" {
		t.Fatalf("verb=%q", ev.Verb)
	}
	if ev.Amount != 389 || !ev.AmountKnown {
		t.Fatalf("amount=%d known=%v", ev.Amount, ev.AmountKnown)
	}
}

func TestParseLine_CritAssociation_Context(t *testing.T) {
	ctx := &model.ParseContext{LocalActorName: "Emberval"}
	meta := "[Fri Jan 23 07:46:01 2026] Emberval scores a critical hit! (7138)"
	_, ok := ParseLine(ctx, meta, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	dmg := "[Fri Jan 23 07:46:01 2026] You pierce a training dummy for 7239 points of damage."
	ev, ok := ParseLine(ctx, dmg, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Kind != model.KindMeleeDamage {
		t.Fatalf("kind=%v", ev.Kind)
	}
	if !ev.Crit {
		t.Fatalf("expected crit")
	}
	if ev.MetaInt != 7138 {
		t.Fatalf("metaint=%d", ev.MetaInt)
	}
}

func TestParseLine_TimestampParses(t *testing.T) {
	line := "[Fri Jan 23 07:46:01 2026] You pierce a training dummy for 7239 points of damage."
	ev, ok := ParseLine(nil, line, time.Local)
	if !ok {
		t.Fatalf("expected ok")
	}
	if ev.Timestamp.Year() != 2026 {
		t.Fatalf("year=%d", ev.Timestamp.Year())
	}
	// Ensure it's in local time location (or at least non-nil).
	if ev.Timestamp.Location() == nil {
		t.Fatalf("nil location")
	}
	_ = time.Local
}
