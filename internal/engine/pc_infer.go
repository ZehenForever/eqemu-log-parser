package engine

import (
	"strings"

	"github.com/ZehenForever/eqemu-log-parser/internal/model"
)

func InferPCNames(events []model.Event) map[string]struct{} {
	out := make(map[string]struct{})
	for _, ev := range events {
		switch ev.Kind {
		case model.KindMeleeDamage, model.KindNonMeleeDamage:
			// ok
		default:
			continue
		}
		actor := ev.Actor
		if actor == "" || actor == "YOU" {
			continue
		}
		if strings.Contains(actor, " ") {
			continue
		}
		b := actor[0]
		if b < 'A' || b > 'Z' {
			continue
		}
		out[actor] = struct{}{}
	}
	return out
}
