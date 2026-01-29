package engine

import (
	"regexp"
	"strings"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

type IdentityClass uint8

const (
	IdentityUnknown IdentityClass = iota
	IdentityLikelyPC
	IdentityLikelyNPC
)

func (c IdentityClass) String() string {
	switch c {
	case IdentityLikelyPC:
		return "LikelyPC"
	case IdentityLikelyNPC:
		return "LikelyNPC"
	default:
		return "Unknown"
	}
}

type IdentityScore struct {
	Name    string
	Score   int
	Class   IdentityClass
	Reasons []string
}

const DefaultPCThreshold = 6

var rePCMorph = regexp.MustCompile(`^[A-Z][a-zA-Z'\-]{2,15}$`)

type identityAccum struct {
	seenActor         bool
	seenTarget        bool
	actorAmountDamage int
	actorNonMelee     bool
	actorCastStart    bool
}

func ClassifyNames(events []model.Event) map[string]IdentityScore {
	acc := make(map[string]*identityAccum)
	ensure := func(name string) *identityAccum {
		if name == "" {
			return nil
		}
		a := acc[name]
		if a == nil {
			a = &identityAccum{}
			acc[name] = a
		}
		return a
	}

	for _, ev := range events {
		switch ev.Kind {
		case model.KindCastStart:
			if a := ensure(ev.Actor); a != nil {
				a.seenActor = true
				a.actorCastStart = true
			}
		case model.KindMeleeDamage, model.KindNonMeleeDamage:
			if !ev.AmountKnown {
				break
			}
			if a := ensure(ev.Actor); a != nil {
				a.seenActor = true
				a.actorAmountDamage++
				if ev.Kind == model.KindNonMeleeDamage {
					a.actorNonMelee = true
				}
			}
			if a := ensure(ev.Target); a != nil {
				a.seenTarget = true
			}
		}
	}

	out := make(map[string]IdentityScore, len(acc))
	for name, a := range acc {
		sc := IdentityScore{Name: name}

		if !strings.Contains(name, " ") {
			sc.Score += 3
			sc.Reasons = append(sc.Reasons, "single_token")
		}
		b := name[0]
		if b >= 'A' && b <= 'Z' {
			sc.Score += 2
			sc.Reasons = append(sc.Reasons, "initial_cap")
		}
		if rePCMorph.MatchString(name) {
			sc.Score += 1
			sc.Reasons = append(sc.Reasons, "pc_regex")
		}

		if a.actorAmountDamage >= 3 {
			sc.Score += 1
			sc.Reasons = append(sc.Reasons, "actor_damage>=3")
		}
		if a.actorNonMelee {
			sc.Score += 1
			sc.Reasons = append(sc.Reasons, "actor_nonmelee")
		}
		if a.actorCastStart {
			sc.Score += 1
			sc.Reasons = append(sc.Reasons, "actor_caststart")
		}

		article := false
		if strings.HasPrefix(name, "a ") || strings.HasPrefix(name, "an ") || strings.HasPrefix(name, "the ") {
			sc.Score -= 4
			article = true
			sc.Reasons = append(sc.Reasons, "article_prefix")
		}
		hasSpace := strings.Contains(name, " ")
		if hasSpace {
			sc.Score -= 2
			sc.Reasons = append(sc.Reasons, "has_spaces")
			if name[0] >= 'a' && name[0] <= 'z' {
				sc.Score -= 3
				sc.Reasons = append(sc.Reasons, "spaces_lowercase_start")
			}
		}
		if strings.Contains(strings.ToLower(name), "training dummy") {
			sc.Score -= 2
			sc.Reasons = append(sc.Reasons, "training_dummy")
		}
		if a.seenTarget && !a.seenActor {
			sc.Score -= 2
			sc.Reasons = append(sc.Reasons, "target_only")
		}

		if sc.Score >= DefaultPCThreshold {
			sc.Class = IdentityLikelyPC
		} else if article {
			sc.Class = IdentityLikelyNPC
		} else {
			sc.Class = IdentityUnknown
		}

		out[name] = sc
	}

	return out
}

func ApplyIdentityOverrides(scores map[string]IdentityScore, pcThreshold int, forcePC, forceNPC map[string]struct{}) {
	if pcThreshold <= 0 {
		pcThreshold = DefaultPCThreshold
	}

	for name, sc := range scores {
		article := false
		for _, r := range sc.Reasons {
			switch r {
			case "article_prefix":
				article = true
			}
		}

		if sc.Score >= pcThreshold {
			sc.Class = IdentityLikelyPC
		} else if article {
			sc.Class = IdentityLikelyNPC
		} else {
			sc.Class = IdentityUnknown
		}

		if forceNPC != nil {
			if _, ok := forceNPC[name]; ok {
				sc.Class = IdentityLikelyNPC
				sc.Reasons = append(sc.Reasons, "force-npc")
				scores[name] = sc
				continue
			}
		}
		if forcePC != nil {
			if _, ok := forcePC[name]; ok {
				sc.Class = IdentityLikelyPC
				sc.Reasons = append(sc.Reasons, "force-pc")
			}
		}

		scores[name] = sc
	}
}
