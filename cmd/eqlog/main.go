package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ZehenForever/eqemu-log-parser/internal/engine"
	"github.com/ZehenForever/eqemu-log-parser/internal/model"
	"github.com/ZehenForever/eqemu-log-parser/internal/parse"
	"github.com/ZehenForever/eqemu-log-parser/internal/tail"
)

type multiStringFlag []string

func (m *multiStringFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiStringFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "parse":
		return runParse(args[1:])
	case "encounters":
		return runEncounters(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "eqlog parse --file <path>")
	fmt.Fprintln(os.Stderr, "eqlog encounters --file <path>")
}

func startAtEnd(follow bool, start string) (bool, error) {
	if start == "" {
		return follow, nil
	}
	switch strings.ToLower(start) {
	case "begin", "beginning", "start":
		return false, nil
	case "end":
		return true, nil
	default:
		return false, fmt.Errorf("invalid --start value %q (expected begin|end)", start)
	}
}

func runParse(args []string) int {
	fs := flag.NewFlagSet("parse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	filePath := fs.String("file", "", "path to EverQuest combat log")
	follow := fs.Bool("follow", false, "tail the file and process new lines as they are appended")
	start := fs.String("start", "", "when following, start at begin or end (default: end when --follow, begin otherwise)")
	lastHours := fs.Float64("last-hours", 0, "only ingest events from the last N hours (0 disables)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "--file is required")
		return 2
	}
	startEnd, err := startAtEnd(*follow, *start)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}

	now := time.Now()
	tf := engine.NewTimeFilterLastHours(*lastHours, now)

	if *follow {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		playerName, _ := parse.PlayerNameFromLogPath(*filePath)
		pctx := &model.ParseContext{LocalActorName: playerName}
		e := engine.New()

		if *lastHours > 0 && startEnd {
			f, err := os.Open(*filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to open file for preload: %v\n", err)
				return 1
			}
			it := parse.ParseFile(f, pctx, time.Local)
			for it.Next() {
				ev := it.Event()
				if !tf.Allow(ev.Timestamp) {
					continue
				}
				if playerName != "" {
					if ev.Actor == "YOU" {
						ev.Actor = playerName
					}
					if ev.Target == "YOU" {
						ev.Target = playerName
					}
				}
				e.Process(ev)
			}
			_ = f.Close()
			if err := it.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to read file for preload: %v\n", err)
				return 1
			}
		}

		tlr, err := tail.NewTailer(*filePath, tail.TailOptions{StartAtEnd: startEnd})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start tailer: %v\n", err)
			return 1
		}
		defer tlr.Stop()

		lineCh := make(chan string, 1024)
		errCh := make(chan error, 1)
		go func() {
			errCh <- tlr.Run(ctx, func(line string) {
				select {
				case lineCh <- line:
				default:
				}
			})
		}()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		dirty := *lastHours > 0 && startEnd
		for {
			select {
			case <-ctx.Done():
				return 0
			case err := <-errCh:
				if err != nil {
					fmt.Fprintf(os.Stderr, "tail error: %v\n", err)
					return 1
				}
				return 0
			case line := <-lineCh:
				ev, ok := parse.ParseLine(pctx, line, time.Local)
				if !ok {
					continue
				}
				if !tf.Allow(ev.Timestamp) {
					continue
				}
				if playerName != "" {
					if ev.Actor == "YOU" {
						ev.Actor = playerName
					}
					if ev.Target == "YOU" {
						ev.Target = playerName
					}
				}
				e.Process(ev)
				dirty = true
			case <-ticker.C:
				if !dirty {
					continue
				}
				printActorTable(e)
				fmt.Fprintln(os.Stdout)
				printTopTargets(e, 10)
				fmt.Fprintln(os.Stdout)
				dirty = false
			}
		}
	}

	f, err := os.Open(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open file: %v\n", err)
		return 1
	}
	defer f.Close()

	playerName, _ := parse.PlayerNameFromLogPath(*filePath)
	ctx := &model.ParseContext{LocalActorName: playerName}

	e := engine.New()
	it := parse.ParseFile(f, ctx, time.Local)
	for it.Next() {
		ev := it.Event()
		if !tf.Allow(ev.Timestamp) {
			continue
		}
		if playerName != "" {
			if ev.Actor == "YOU" {
				ev.Actor = playerName
			}
			if ev.Target == "YOU" {
				ev.Target = playerName
			}
		}
		e.Process(ev)
	}
	if err := it.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read file: %v\n", err)
		return 1
	}

	printActorTable(e)
	fmt.Fprintln(os.Stdout)
	printTopTargets(e, 10)
	return 0
}

func runEncounters(args []string) int {
	fs := flag.NewFlagSet("encounters", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	filePath := fs.String("file", "", "path to EverQuest combat log")
	idleTimeout := fs.Duration("idle-timeout", 8*time.Second, "idle timeout before encounter ends")
	includePCTargets := fs.Bool("include-pc-targets", false, "include encounters keyed by player-character targets")
	follow := fs.Bool("follow", false, "tail the file and process new lines as they are appended")
	start := fs.String("start", "", "when following, start at begin or end (default: end when --follow, begin otherwise)")
	lastHours := fs.Float64("last-hours", 0, "only ingest events from the last N hours (0 disables)")
	pcThreshold := fs.Int("pc-threshold", engine.DefaultPCThreshold, "score threshold for LikelyPC classification")
	debugIdentities := fs.Bool("debug-identities", false, "print identity classification summary")
	var forcePC multiStringFlag
	var forceNPC multiStringFlag
	fs.Var(&forcePC, "force-pc", "force a name to be treated as PC (repeatable)")
	fs.Var(&forceNPC, "force-npc", "force a name to be treated as NPC (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "--file is required")
		return 2
	}
	startEnd, err := startAtEnd(*follow, *start)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 2
	}

	now := time.Now()
	tf := engine.NewTimeFilterLastHours(*lastHours, now)

	if *follow {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		playerName, _ := parse.PlayerNameFromLogPath(*filePath)
		pctx := &model.ParseContext{LocalActorName: playerName}
		seg := engine.NewEncounterSegmenter(*idleTimeout, playerName)
		identityEvents := make([]model.Event, 0, 4096)

		if *lastHours > 0 && startEnd {
			f, err := os.Open(*filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to open file for preload: %v\n", err)
				return 1
			}
			it := parse.ParseFile(f, pctx, time.Local)
			for it.Next() {
				ev := it.Event()
				if !tf.Allow(ev.Timestamp) {
					continue
				}
				if playerName != "" {
					if ev.Actor == "YOU" {
						ev.Actor = playerName
					}
					if ev.Target == "YOU" {
						ev.Target = playerName
					}
				}
				seg.Process(ev)
				identityEvents = append(identityEvents, ev)
				if len(identityEvents) > 8192 {
					identityEvents = identityEvents[len(identityEvents)-4096:]
				}
			}
			_ = f.Close()
			if err := it.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to read file for preload: %v\n", err)
				return 1
			}
		}

		tlr, err := tail.NewTailer(*filePath, tail.TailOptions{StartAtEnd: startEnd})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start tailer: %v\n", err)
			return 1
		}
		defer tlr.Stop()

		lineCh := make(chan string, 1024)
		errCh := make(chan error, 1)
		go func() {
			errCh <- tlr.Run(ctx, func(line string) {
				select {
				case lineCh <- line:
				default:
				}
			})
		}()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		dirty := *lastHours > 0 && startEnd
		for {
			select {
			case <-ctx.Done():
				return 0
			case err := <-errCh:
				if err != nil {
					fmt.Fprintf(os.Stderr, "tail error: %v\n", err)
					return 1
				}
				return 0
			case line := <-lineCh:
				ev, ok := parse.ParseLine(pctx, line, time.Local)
				if !ok {
					continue
				}
				if !tf.Allow(ev.Timestamp) {
					continue
				}
				if playerName != "" {
					if ev.Actor == "YOU" {
						ev.Actor = playerName
					}
					if ev.Target == "YOU" {
						ev.Target = playerName
					}
				}
				seg.Process(ev)
				identityEvents = append(identityEvents, ev)
				if len(identityEvents) > 8192 {
					identityEvents = identityEvents[len(identityEvents)-4096:]
				}
				dirty = true
			case <-ticker.C:
				if !dirty {
					continue
				}

				scores := engine.ClassifyNames(identityEvents)
				forcePCSet := make(map[string]struct{}, len(forcePC))
				for _, n := range forcePC {
					forcePCSet[n] = struct{}{}
					if _, ok := scores[n]; !ok {
						scores[n] = engine.IdentityScore{Name: n}
					}
				}
				forceNPCSet := make(map[string]struct{}, len(forceNPC))
				for _, n := range forceNPC {
					forceNPCSet[n] = struct{}{}
					if _, ok := scores[n]; !ok {
						scores[n] = engine.IdentityScore{Name: n}
					}
				}
				engine.ApplyIdentityOverrides(scores, *pcThreshold, forcePCSet, forceNPCSet)

				if *debugIdentities {
					seen := make(map[string]struct{})
					for _, ev := range identityEvents {
						switch ev.Kind {
						case model.KindMeleeDamage, model.KindNonMeleeDamage:
							if ev.AmountKnown {
								if ev.Actor != "" {
									seen[ev.Actor] = struct{}{}
								}
								if ev.Target != "" {
									seen[ev.Target] = struct{}{}
								}
							}
						}
					}
					rows := make([]engine.IdentityScore, 0, len(seen))
					for name := range seen {
						if sc, ok := scores[name]; ok {
							rows = append(rows, sc)
						}
					}
					sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
					w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
					fmt.Fprintln(w, "Name\tScore\tClass\tReasons")
					for _, sc := range rows {
						fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", sc.Name, sc.Score, sc.Class.String(), strings.Join(sc.Reasons, ","))
					}
					_ = w.Flush()
					fmt.Fprintln(os.Stdout)
				}

				encs := seg.Snapshot()
				if !*includePCTargets {
					filt := encs[:0]
					for _, e := range encs {
						if sc, ok := scores[e.Target]; ok {
							if sc.Class == engine.IdentityLikelyPC {
								continue
							}
						}
						filt = append(filt, e)
					}
					encs = filt
				}
				if len(encs) > 0 {
					latest := encs[len(encs)-1]
					printEncounters([]*engine.Encounter{latest})
					fmt.Fprintln(os.Stdout)
				}
				dirty = false
			}
		}
	}

	f, err := os.Open(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open file: %v\n", err)
		return 1
	}
	defer f.Close()

	playerName, _ := parse.PlayerNameFromLogPath(*filePath)
	ctx := &model.ParseContext{LocalActorName: playerName}
	seg := engine.NewEncounterSegmenter(*idleTimeout, playerName)

	it := parse.ParseFile(f, ctx, time.Local)
	events := make([]model.Event, 0, 1024)
	for it.Next() {
		ev := it.Event()
		if !tf.Allow(ev.Timestamp) {
			continue
		}
		if playerName != "" {
			if ev.Actor == "YOU" {
				ev.Actor = playerName
			}
			if ev.Target == "YOU" {
				ev.Target = playerName
			}
		}
		events = append(events, ev)
	}
	if err := it.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read file: %v\n", err)
		return 1
	}

	scores := engine.ClassifyNames(events)
	forcePCSet := make(map[string]struct{}, len(forcePC))
	for _, n := range forcePC {
		forcePCSet[n] = struct{}{}
		if _, ok := scores[n]; !ok {
			scores[n] = engine.IdentityScore{Name: n}
		}
	}
	forceNPCSet := make(map[string]struct{}, len(forceNPC))
	for _, n := range forceNPC {
		forceNPCSet[n] = struct{}{}
		if _, ok := scores[n]; !ok {
			scores[n] = engine.IdentityScore{Name: n}
		}
	}
	engine.ApplyIdentityOverrides(scores, *pcThreshold, forcePCSet, forceNPCSet)

	if *debugIdentities {
		seen := make(map[string]struct{})
		for _, ev := range events {
			switch ev.Kind {
			case model.KindMeleeDamage, model.KindNonMeleeDamage:
				if ev.AmountKnown {
					if ev.Actor != "" {
						seen[ev.Actor] = struct{}{}
					}
					if ev.Target != "" {
						seen[ev.Target] = struct{}{}
					}
				}
			}
		}
		rows := make([]engine.IdentityScore, 0, len(seen))
		for name := range seen {
			if sc, ok := scores[name]; ok {
				rows = append(rows, sc)
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "Name\tScore\tClass\tReasons")
		for _, sc := range rows {
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", sc.Name, sc.Score, sc.Class.String(), strings.Join(sc.Reasons, ","))
		}
		_ = w.Flush()
		fmt.Fprintln(os.Stdout)
	}

	if !*includePCTargets {
		excluded := make(map[string]struct{})
		for name, sc := range scores {
			if sc.Class == engine.IdentityLikelyPC {
				excluded[name] = struct{}{}
			}
		}
		seg.SetExcludedTargets(excluded)
	}
	for _, ev := range events {
		seg.Process(ev)
	}

	encs := seg.Finalize()
	printEncounters(encs)
	return 0
}

func printEncounters(encs []*engine.Encounter) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "Target\tStart\tEnd\tDurationSeconds\tTotalDamage\tDPS(encounter)")
	for _, enc := range encs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%.1f\t%d\t%.1f\n",
			enc.Target,
			enc.Start.Format(time.RFC3339),
			enc.End.Format(time.RFC3339),
			enc.DurationSeconds(),
			enc.Total,
			enc.DPS(),
		)
	}
	_ = w.Flush()

	for _, enc := range encs {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "Encounter: %s\n", enc.Target)
		aw := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(aw, "Actor\tMelee\tNonMelee\tTotal\tDPS(enc)\tSDPS\tSec")
		actors := enc.ActorsSortedByTotal()
		limit := 8
		if len(actors) < limit {
			limit = len(actors)
		}
		encSec := enc.DurationSeconds()
		for i := 0; i < limit; i++ {
			st := actors[i]
			dpsEnc := 0.0
			if encSec > 0 {
				dpsEnc = float64(st.Total) / encSec
			}
			activeSec := st.ActiveSeconds()
			sdps := 0.0
			if activeSec > 0 {
				sdps = float64(st.Total) / activeSec
			}
			fmt.Fprintf(aw, "%s\t%d\t%d\t%d\t%.1f\t%.1f\t%.0f\n", st.Actor, st.Melee, st.NonMelee, st.Total, dpsEnc, sdps, activeSec)
		}
		_ = aw.Flush()
	}
}

func printActorTable(e *engine.Engine) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "Actor\tMelee\tNonMelee\tTotal\tDurationSeconds\tDPS(active)")

	rows := e.ActorsSortedByTotal()
	for _, st := range rows {
		dur := st.DurationSeconds()
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%.1f\t%.1f\n", st.Actor, st.Melee, st.NonMelee, st.Total, dur, st.DPS())
	}
	_ = w.Flush()
}

func printTopTargets(e *engine.Engine, n int) {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "Top Targets By Total Damage")
	fmt.Fprintln(w, "Target\tTotal")
	for _, ts := range e.TopTargets(n) {
		fmt.Fprintf(w, "%s\t%d\n", ts.Target, ts.Total)
	}
	_ = w.Flush()
}
