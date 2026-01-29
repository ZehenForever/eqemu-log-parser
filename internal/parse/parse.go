package parse

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

const tsLayout = "Mon Jan 02 15:04:05 2006"

var (
	reTimestamp = regexp.MustCompile(`^\[(?P<ts>[^\]]+)\]\s+(?P<msg>.*)$`)

	reCritMetaActor = regexp.MustCompile(`^(?P<actor>.+?)\s+scores\s+a\s+critical\s+hit!\s*\((?P<val>\d+)\)$`)
	reCritMetaYou   = regexp.MustCompile(`^You\s+deliver\s+a\s+critical\s+blast!\s*\((?P<val>\d+)\)$`)

	reCastStart  = regexp.MustCompile(`^You\s+begin\s+casting\s+(?P<spell>.+?)\.$`)
	reAffliction = regexp.MustCompile(`^(?P<target>.+?)\s+is\s+afflicted\s+by\s+(?P<spell>.+?)\.$`)

	reHealTarget       = regexp.MustCompile(`^(?P<target>.+?)\s+has\s+been\s+healed\s+for\s+(?P<amt>\d+)\s+points\.$`)
	reHealTargetDamage = regexp.MustCompile(`^(?P<target>.+?)\s+has\s+been\s+healed\s+for\s+(?P<amt>\d+)\s+points\s+of\s+damage\.$`)
	reHealYou          = regexp.MustCompile(`^You\s+have\s+been\s+healed\s+for\s+(?P<amt>\d+)\s+points\.$`)
	reHealYouDamage    = regexp.MustCompile(`^You\s+have\s+been\s+healed\s+for\s+(?P<amt>\d+)\s+points\s+of\s+damage\.$`)

	reIncomingByNonMelee = regexp.MustCompile(`^You\s+have\s+taken\s+(?P<amt>\d+)\s+points\s+of\s+damage\s+by\s+non-melee\.$`)
	reIncomingNonMelee   = regexp.MustCompile(`^You\s+have\s+taken\s+(?P<amt>\d+)\s+points\s+of\s+non-melee\s+damage\.$`)

	reIncomingOnMelee = regexp.MustCompile(`^(?P<actor>.+?)\s+(?P<verb>hits|hit|bashes|bash|kicks|kick|crushes|crush|slashes|slash|pierces|pierce|punches|punch|strikes|strike)\s+on\s+(?P<target>YOU|[A-Z][a-zA-Z'\-]{2,15})\s+for\s+(?P<amt>\d+)\s+points\s+of\s+damage\.$`)

	reThornsMarker = regexp.MustCompile(`^(?P<target>.+?)\s+was\s+pierced\s+by\s+thorns\.$`)

	reNonMelee = regexp.MustCompile(`^(?P<actor>.+?)\s+hit\s+(?P<target>.+?)\s+for\s+(?P<amt>\d+)\s+points\s+of\s+non-melee\s+damage\.$`)

	reYouMelee   = regexp.MustCompile(`^You\s+(?P<verb>\w+)\s+(?P<target>.+?)\s+for\s+(?P<amt>\d+)\s+points\s+of\s+damage\.$`)
	reOtherMelee = regexp.MustCompile(`^(?P<actor>.+?)\s+(?P<verb>hits|hit|kicks|kick|bashes|bash|crushes|crush|slashes|slash|pierces|pierce|punches|punch|claws|claw|bites|bite|mauls|maul|strikes|strike|backstabs|backstab|frenzies|frenzy|rends|rend)\s+(?P<target>.+?)\s+for\s+(?P<amt>\d+)\s+points\s+of\s+damage\.$`)

	reYouMiss     = regexp.MustCompile(`^You\s+try\s+to\s+(?P<verb>\w+)\s+(?P<target>.+?),\s+but\s+miss!$`)
	reTryHitAvoid = regexp.MustCompile(`^(?P<actor>.+?)\s+tries\s+to\s+hit\s+(?P<target>.+?),\s+but\s+(?P<defender>.+?)\s+(?P<avoid>dodges|blocks|parries|ripostes|misses)!$`)
	reTryVerbMiss = regexp.MustCompile(`^(?P<actor>.+?)\s+tries\s+to\s+(?P<verb>kick|bash|strike|slash|pierce|crush|hit)\s+(?P<target>.+?),\s+but\s+misses!$`)

	reAutoAttack = regexp.MustCompile(`^Auto\s+attack\s+is\s+(on|off)\.$`)
)

func ParseLine(ctx *model.ParseContext, line string, loc *time.Location) (model.Event, bool) {
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	if line == "" {
		return model.Event{}, false
	}
	if loc == nil {
		loc = time.Local
	}

	idx := reTimestamp.FindStringSubmatchIndex(line)
	if idx == nil {
		return model.Event{}, false
	}
	tsStr := reSub(line, idx, 1)
	msg := reSub(line, idx, 2)

	ts, err := time.ParseInLocation(tsLayout, tsStr, loc)
	if err != nil {
		return model.Event{}, false
	}

	if ctx != nil && ctx.PendingCrit != nil && ctx.PendingCrit.TTL <= 0 {
		ctx.PendingCrit = nil
	}

	ev := model.Event{Timestamp: ts, Raw: line, Kind: model.KindUnknown}

	if m := reCritMetaActor.FindStringSubmatchIndex(msg); m != nil {
		actor := reSub(msg, m, reCritMetaActor.SubexpIndex("actor"))
		valStr := reSub(msg, m, reCritMetaActor.SubexpIndex("val"))
		val, ok := parseInt64(valStr)
		if ok {
			ev.Kind = model.KindCritMeta
			if ctx != nil && ctx.LocalActorName != "" && actor == ctx.LocalActorName {
				actor = "YOU"
			}
			ev.Actor = actor
			ev.MetaInt = val
			if ctx != nil {
				ctx.PendingCrit = &model.PendingCrit{Actor: actor, Ts: ts, Value: val, TTL: 2}
			}
			return ev, true
		}
	}
	if m := reCritMetaYou.FindStringSubmatchIndex(msg); m != nil {
		valStr := reSub(msg, m, reCritMetaYou.SubexpIndex("val"))
		val, ok := parseInt64(valStr)
		if ok {
			ev.Kind = model.KindCritMeta
			ev.Actor = "YOU"
			ev.MetaInt = val
			if ctx != nil {
				ctx.PendingCrit = &model.PendingCrit{Actor: "YOU", Ts: ts, Value: val, TTL: 2}
			}
			return ev, true
		}
	}

	if m := reCastStart.FindStringSubmatchIndex(msg); m != nil {
		ev.Kind = model.KindCastStart
		ev.Actor = "YOU"
		ev.SpellOrSkill = reSub(msg, m, reCastStart.SubexpIndex("spell"))
		return ev, true
	}
	if m := reAffliction.FindStringSubmatchIndex(msg); m != nil {
		ev.Kind = model.KindAffliction
		ev.Target = reSub(msg, m, reAffliction.SubexpIndex("target"))
		ev.SpellOrSkill = reSub(msg, m, reAffliction.SubexpIndex("spell"))
		return ev, true
	}
	if m := reThornsMarker.FindStringSubmatchIndex(msg); m != nil {
		ev.Kind = model.KindThornsMarker
		ev.Target = reSub(msg, m, reThornsMarker.SubexpIndex("target"))
		ev.AmountKnown = false
		return ev, true
	}

	if m := reHealTarget.FindStringSubmatchIndex(msg); m != nil {
		target := reSub(msg, m, reHealTarget.SubexpIndex("target"))
		amtStr := reSub(msg, m, reHealTarget.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Kind = model.KindHeal
			ev.Target = target
			ev.Amount = amt
			ev.AmountKnown = true
			return ev, true
		}
	}
	if m := reHealTargetDamage.FindStringSubmatchIndex(msg); m != nil {
		target := reSub(msg, m, reHealTargetDamage.SubexpIndex("target"))
		amtStr := reSub(msg, m, reHealTargetDamage.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Kind = model.KindHeal
			ev.Target = target
			ev.Amount = amt
			ev.AmountKnown = true
			return ev, true
		}
	}
	if m := reHealYou.FindStringSubmatchIndex(msg); m != nil {
		amtStr := reSub(msg, m, reHealYou.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Kind = model.KindHeal
			ev.Target = "YOU"
			ev.Amount = amt
			ev.AmountKnown = true
			return ev, true
		}
	}
	if m := reHealYouDamage.FindStringSubmatchIndex(msg); m != nil {
		amtStr := reSub(msg, m, reHealYouDamage.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Kind = model.KindHeal
			ev.Target = "YOU"
			ev.Amount = amt
			ev.AmountKnown = true
			return ev, true
		}
	}

	if m := reIncomingByNonMelee.FindStringSubmatchIndex(msg); m != nil {
		amtStr := reSub(msg, m, reIncomingByNonMelee.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Kind = model.KindIncomingDamage
			ev.Target = "YOU"
			ev.Amount = amt
			ev.AmountKnown = true
			ev.Verb = "non-melee"
			return ev, true
		}
	}
	if m := reIncomingNonMelee.FindStringSubmatchIndex(msg); m != nil {
		amtStr := reSub(msg, m, reIncomingNonMelee.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Kind = model.KindIncomingDamage
			ev.Target = "YOU"
			ev.Amount = amt
			ev.AmountKnown = true
			ev.Verb = "non-melee"
			return ev, true
		}
	}
	if m := reIncomingOnMelee.FindStringSubmatchIndex(msg); m != nil {
		actor := reSub(msg, m, reIncomingOnMelee.SubexpIndex("actor"))
		verb := reSub(msg, m, reIncomingOnMelee.SubexpIndex("verb"))
		target := reSub(msg, m, reIncomingOnMelee.SubexpIndex("target"))
		amtStr := reSub(msg, m, reIncomingOnMelee.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Kind = model.KindIncomingDamage
			if ctx != nil && ctx.LocalActorName != "" && actor == ctx.LocalActorName {
				actor = "YOU"
			}
			ev.Actor = actor
			ev.Target = target
			ev.Verb = verb
			ev.Amount = amt
			ev.AmountKnown = true
			return ev, true
		}
	}

	if m := reNonMelee.FindStringSubmatchIndex(msg); m != nil {
		actor := reSub(msg, m, reNonMelee.SubexpIndex("actor"))
		target := reSub(msg, m, reNonMelee.SubexpIndex("target"))
		amtStr := reSub(msg, m, reNonMelee.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Kind = model.KindNonMeleeDamage
			if ctx != nil && ctx.LocalActorName != "" && actor == ctx.LocalActorName {
				actor = "YOU"
			}
			ev.Actor = actor
			ev.Target = target
			ev.Amount = amt
			ev.AmountKnown = true
			ev.DamageClass = model.DamageClassDirect
			handlePendingCrit(ctx, &ev)
			return ev, true
		}
	}
	if m := reYouMelee.FindStringSubmatchIndex(msg); m != nil {
		ev.Kind = model.KindMeleeDamage
		ev.Actor = "YOU"
		ev.Verb = reSub(msg, m, reYouMelee.SubexpIndex("verb"))
		ev.Target = reSub(msg, m, reYouMelee.SubexpIndex("target"))
		amtStr := reSub(msg, m, reYouMelee.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Amount = amt
			ev.AmountKnown = true
			applyDamageClass(&ev)
			handlePendingCrit(ctx, &ev)
			return ev, true
		}
	}
	if m := reOtherMelee.FindStringSubmatchIndex(msg); m != nil {
		ev.Kind = model.KindMeleeDamage
		actor := reSub(msg, m, reOtherMelee.SubexpIndex("actor"))
		if ctx != nil && ctx.LocalActorName != "" && actor == ctx.LocalActorName {
			actor = "YOU"
		}
		ev.Actor = actor
		ev.Verb = reSub(msg, m, reOtherMelee.SubexpIndex("verb"))
		ev.Target = reSub(msg, m, reOtherMelee.SubexpIndex("target"))
		amtStr := reSub(msg, m, reOtherMelee.SubexpIndex("amt"))
		amt, ok := parseInt64(amtStr)
		if ok {
			ev.Amount = amt
			ev.AmountKnown = true
			applyDamageClass(&ev)
			handlePendingCrit(ctx, &ev)
			return ev, true
		}
	}

	if m := reYouMiss.FindStringSubmatchIndex(msg); m != nil {
		ev.Kind = model.KindMiss
		ev.Actor = "YOU"
		ev.Verb = reSub(msg, m, reYouMiss.SubexpIndex("verb"))
		ev.Target = reSub(msg, m, reYouMiss.SubexpIndex("target"))
		handlePendingCrit(ctx, &ev)
		return ev, true
	}
	if m := reTryHitAvoid.FindStringSubmatchIndex(msg); m != nil {
		actor := reSub(msg, m, reTryHitAvoid.SubexpIndex("actor"))
		target := reSub(msg, m, reTryHitAvoid.SubexpIndex("target"))
		avoid := reSub(msg, m, reTryHitAvoid.SubexpIndex("avoid"))
		if ctx != nil && ctx.LocalActorName != "" && actor == ctx.LocalActorName {
			actor = "YOU"
		}
		ev.Actor = actor
		ev.Target = target
		ev.Verb = avoid
		if avoid == "misses" {
			ev.Kind = model.KindMiss
		} else {
			ev.Kind = model.KindAvoid
		}
		handlePendingCrit(ctx, &ev)
		return ev, true
	}
	if m := reTryVerbMiss.FindStringSubmatchIndex(msg); m != nil {
		ev.Kind = model.KindMiss
		actor := reSub(msg, m, reTryVerbMiss.SubexpIndex("actor"))
		if ctx != nil && ctx.LocalActorName != "" && actor == ctx.LocalActorName {
			actor = "YOU"
		}
		ev.Actor = actor
		ev.Verb = reSub(msg, m, reTryVerbMiss.SubexpIndex("verb"))
		ev.Target = reSub(msg, m, reTryVerbMiss.SubexpIndex("target"))
		handlePendingCrit(ctx, &ev)
		return ev, true
	}

	if m := reAutoAttack.FindStringSubmatchIndex(msg); m != nil {
		ev.Kind = model.KindZoneOrSystem
		ev.SpellOrSkill = "auto_attack"
		ev.Verb = reSub(msg, m, 1)
		handlePendingCrit(ctx, &ev)
		return ev, true
	}

	return ev, true
}

func applyDamageClass(ev *model.Event) {
	if ev == nil {
		return
	}
	if !ev.AmountKnown {
		return
	}
	if ev.Kind != model.KindMeleeDamage {
		return
	}

	switch strings.ToLower(ev.Verb) {
	case "pierce", "pierces":
		ev.DamageClass = model.DamageClassPierce
	case "slash", "slashes":
		ev.DamageClass = model.DamageClassSlash
	case "crush", "crushes":
		ev.DamageClass = model.DamageClassCrush
	case "bash", "bashes":
		ev.DamageClass = model.DamageClassBash
	case "kick", "kicks":
		ev.DamageClass = model.DamageClassKick
	}
}

func ParseFile(r io.Reader, ctx *model.ParseContext, loc *time.Location) *Iterator {
	return &Iterator{r: r, ctx: ctx, loc: loc}
}

type Iterator struct {
	r   io.Reader
	s   *bufio.Scanner
	err error
	ctx *model.ParseContext
	loc *time.Location

	cur model.Event
	ok  bool
}

func (it *Iterator) Next() bool {
	if it.s == nil {
		it.s = bufio.NewScanner(it.r)
		// allow long lines
		buf := make([]byte, 0, 128*1024)
		it.s.Buffer(buf, 4*1024*1024)
	}

	for it.s.Scan() {
		line := it.s.Text()
		e, ok := ParseLine(it.ctx, line, it.loc)
		if !ok {
			continue
		}
		it.cur = e
		it.ok = true
		return true
	}
	it.err = it.s.Err()
	it.ok = false
	return false
}

func (it *Iterator) Event() model.Event { return it.cur }
func (it *Iterator) Err() error         { return it.err }

func handlePendingCrit(ctx *model.ParseContext, ev *model.Event) {
	if ctx == nil || ctx.PendingCrit == nil {
		return
	}
	pc := ctx.PendingCrit
	if pc.TTL <= 0 {
		ctx.PendingCrit = nil
		return
	}
	dt := ev.Timestamp.Sub(pc.Ts)
	if dt < 0 || dt > 1*time.Second {
		ctx.PendingCrit = nil
		return
	}
	if ev.Actor != pc.Actor {
		return
	}
	if ev.Kind != model.KindMeleeDamage && ev.Kind != model.KindNonMeleeDamage {
		return
	}
	ev.Crit = true
	ev.MetaInt = pc.Value
	pc.TTL--
	if pc.TTL <= 0 {
		ctx.PendingCrit = nil
	}
}

func reSub(s string, idx []int, group int) string {
	if group <= 0 {
		return ""
	}
	start := idx[group*2]
	end := idx[group*2+1]
	if start < 0 || end < 0 {
		return ""
	}
	return s[start:end]
}

func parseInt64(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
