//go:build !js

package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/yourconscience/scrying-pool/scry/internal/bots"
	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
	"github.com/yourconscience/scrying-pool/scry/internal/engine"
	_ "github.com/yourconscience/scrying-pool/scry/internal/engine/cards"
	"github.com/yourconscience/scrying-pool/scry/internal/replay"
)

// seat names in simulated logs: distinct initials, so Cartographer tells them apart
var simNames = [2]string{"Alice", "Bob"}

// simCmd runs "scry sim play" and "scry sim fuzz".
func simCmd(args []string) error {
	if len(args) > 0 && args[0] == "check" {
		return simCheck(args[1:])
	}
	if len(args) == 0 || args[0] != "play" && args[0] != "fuzz" {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	sub := args[0]
	fs := flag.NewFlagSet("sim "+sub, flag.ExitOnError)
	kingdom := fs.String("kingdom", "", "comma-separated kingdom cards; empty: 10 random playable cards each game")
	a := fs.String("a", "bm", "one bot: random, bm or bm+<card>")
	b := fs.String("b", "bm", "the other bot; the two swap seats every game")
	games := 100
	if sub == "fuzz" {
		games = 1000
	}
	n := fs.Int("n", games, "games to play")
	seed := fs.Uint64("seed", 1, "seed of the first game; game i uses seed+i")
	logFile := fs.String("log", "", "write the first game's log to this file")
	check := fs.Bool("check", false, "read each game's log back with Cartographer and compare every turn's decks and zones, and the final VP")
	show := fs.Int("show", 5, "with -check, how many logs with problems to print")
	fs.Parse(args[1:])

	t, err := engine.NewTable(cardsJSON)
	if err != nil {
		return err
	}
	var client *cartographer.Client
	if *check || *logFile != "" {
		if client, err = loadClient(); err != nil {
			return fmt.Errorf("-check and -log render logs with the client tables: %w", err)
		}
	}
	var fixed []engine.CardID
	if *kingdom != "" {
		for _, s := range strings.Split(*kingdom, ",") {
			d, ok := t.Card(strings.TrimSpace(s))
			if !ok {
				return fmt.Errorf("no card %q", s)
			}
			fixed = append(fixed, d.ID)
		}
	}
	specs := [2]string{*a, *b}
	if sub == "fuzz" {
		specs = [2]string{"random", "random"}
	}

	var wins [2]int
	ties, capped, turns, problems, checked := 0, 0, 0, 0, 0
	start := time.Now()
	for i := range *n {
		s := *seed + uint64(i)
		setup := engine.Setup{Kingdom: fixed, Names: simNames, Seed: s}
		if fixed == nil {
			setup = randomKingdom(t, client, s)
		}
		k := setup.Kingdom
		seats := [2]int{0, 1} // which spec plays in each seat
		if i%2 == 1 {
			seats = [2]int{1, 0}
		}
		var agents [2]engine.Agent
		for p := range agents {
			if agents[p], err = bots.New(specs[seats[p]], t, s*2+uint64(p)); err != nil {
				return err
			}
		}
		setup.Names, setup.Agents, setup.Seed = simNames, agents, s
		g, err := engine.New(t, setup)
		if err != nil {
			return err
		}
		var snaps []turnSnap
		if *check || *logFile != "" && i == 0 {
			g.Log = engine.NewLog(t)
		}
		g.AfterTurn = func(g *engine.Game) {
			if sub == "fuzz" {
				if err := g.Check(); err != nil {
					panic(err)
				}
			}
			if *check {
				snaps = append(snaps, snapTurn(g))
			}
		}
		res, perr := playSafely(g)
		if perr != nil {
			return fmt.Errorf("game %d (seed %d, kingdom %s): %v", i, s, kingdomNames(t, k), perr)
		}
		turns += res.Turns[0] + res.Turns[1]
		switch {
		case res.Capped:
			capped++
		case res.Winner < 0:
			ties++
		default:
			wins[seats[res.Winner]]++
		}
		if g.Log == nil {
			continue
		}
		text, err := g.Log.Render(client, simNames)
		if err != nil {
			return fmt.Errorf("game %d (seed %d): %w", i, s, err)
		}
		if *logFile != "" && i == 0 {
			if err := os.WriteFile(*logFile, []byte(text), 0o644); err != nil {
				return err
			}
		}
		if *check {
			checked++
			if ps := checkSimLog(client, text, snaps, res); len(ps) > 0 {
				problems++
				if problems <= *show {
					fmt.Printf("game %d (seed %d, kingdom %s):\n  %s\n", i, s, kingdomNames(t, k), strings.Join(ps[:min(len(ps), 40)], "\n  "))
				}
			}
		}
	}
	el := time.Since(start)
	fmt.Printf("%d games in %s (%.0f games/s), %.1f turns per game\n", *n, el.Round(time.Millisecond), float64(*n)/el.Seconds(), float64(turns)/float64(max(*n, 1)))
	if sub == "play" {
		fmt.Printf("%s wins %d, %s wins %d, ties %d, unfinished %d\n", specs[0], wins[0], specs[1], wins[1], ties, capped)
	} else {
		fmt.Printf("invariants held; unfinished %d\n", capped)
	}
	if *check {
		fmt.Printf("checked %d logs: %d with problems\n", checked, problems)
		if problems > 0 {
			return fmt.Errorf("%d of %d logs don't read back", problems, checked)
		}
	}
	return nil
}

// playSafely turns a panic in the engine, a card or a bot into an error.
func playSafely(g *engine.Game) (res engine.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return g.Play(), nil
}

// randomKingdom picks 10 playable kingdom piles, up to two landscapes (one
// Way at most), and sometimes Platinum and Colony or Shelters. With a client,
// only cards it knows, so the log can be rendered.
func randomKingdom(t *engine.Table, client *cartographer.Client, seed uint64) engine.Setup {
	known := map[string]bool{}
	if client != nil {
		for _, c := range client.Cards {
			known[normCard(c.Name)] = true
		}
	}
	var piles, lands []engine.CardID
	for _, c := range t.Playable() {
		d := t.Defs[c]
		if client != nil && !known[normCard(d.Name)] && !strings.Contains(d.Name, "/") {
			continue
		}
		if d.Landscape() {
			lands = append(lands, c)
		} else {
			piles = append(piles, c)
		}
	}
	r := rand.New(rand.NewPCG(seed, 2))
	r.Shuffle(len(piles), func(i, j int) { piles[i], piles[j] = piles[j], piles[i] })
	r.Shuffle(len(lands), func(i, j int) { lands[i], lands[j] = lands[j], lands[i] })
	shelters := true
	for _, n := range []string{"Hovel", "Necropolis", "Overgrown Estate"} {
		shelters = shelters && t.HasRules(t.MustCard(n))
	}
	s := engine.Setup{Kingdom: piles[:min(10, len(piles))], Colony: r.IntN(5) == 0, Shelters: shelters && r.IntN(5) == 0}
	sort.Slice(s.Kingdom, func(i, j int) bool { return t.Defs[s.Kingdom[i]].Name < t.Defs[s.Kingdom[j]].Name })
	ways := 0
	for _, c := range lands {
		if len(s.Landscapes) == r.IntN(3) {
			break
		}
		if t.Defs[c].Is(engine.TWay) {
			if ways++; ways > 1 {
				continue
			}
		}
		s.Landscapes = append(s.Landscapes, c)
	}
	return s
}

func normCard(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func kingdomNames(t *engine.Table, k []engine.CardID) string {
	var out []string
	for _, c := range k {
		out = append(out, t.Defs[c].Name)
	}
	return strings.Join(out, ",")
}

// turnSnap is every player's cards after a turn, by zone and in all.
type turnSnap struct {
	zones map[string]map[string]map[string]int // player, zone, card
	decks map[string]map[string]int
}

func snapTurn(g *engine.Game) turnSnap {
	s := turnSnap{zones: map[string]map[string]map[string]int{}, decks: map[string]map[string]int{}}
	for p := range g.P {
		name := g.P[p].Name
		s.zones[name] = g.Zones(p)
		s.decks[name] = map[string]int{}
		for _, x := range g.Owned(p) {
			s.decks[name][g.Def(x).Name]++
		}
	}
	return s
}

// checkSimLog reads a simulated log with Cartographer and lists where it
// disagrees with the engine.
func checkSimLog(c *cartographer.Client, text string, snaps []turnSnap, res engine.Result) []string {
	g, err := c.Parse(text, nil)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	for _, u := range g.Unknown {
		out = append(out, fmt.Sprintf("line %d not read: %q", u.Line, u.Text))
	}
	for _, m := range g.ZoneMisses() {
		out = append(out, fmt.Sprintf("line %d: %s's %s (%s, cause %s) wasn't in %s, found in %q", m.Line, m.Player, m.Card, m.Template, m.Cause, m.Want, m.Found))
	}
	turns := g.Turns()
	if len(turns) != len(snaps) {
		out = append(out, fmt.Sprintf("Cartographer reads %d turns, the engine played %d", len(turns), len(snaps)))
	}
	for i := range min(len(turns), len(snaps)) {
		tu, s := turns[i], snaps[i]
		for p, want := range s.decks {
			if have := map[string]int(tu.Decks[p]); !reflect.DeepEqual(have, want) {
				out = append(out, fmt.Sprintf("turn %d %s: %s's deck %v, engine %v", tu.Number, tu.Player, p, have, want))
			}
			have := map[string]map[string]int{}
			for z, cs := range tu.Zones[p] {
				if cs.Unknown > 0 {
					out = append(out, fmt.Sprintf("turn %d %s: %s's %s has %d unnamed cards", tu.Number, tu.Player, p, z, cs.Unknown))
				}
				if len(cs.Known) > 0 {
					have[z] = map[string]int(cs.Known)
				}
			}
			want := s.zones[p]
			if i == len(snaps)-1 {
				// the last turn has no Clean-up, and its log can't tell a played card from a
				// copy in hand when a card moves one of its own name (Cellar discards a Cellar)
				have, want = handWithPlay(have), handWithPlay(want)
			}
			if !reflect.DeepEqual(have, want) {
				out = append(out, fmt.Sprintf("turn %d %s: %s's zones %v, engine %v", tu.Number, tu.Player, p, have, want))
			}
		}
	}
	vp := c.Score(g, cartographer.ScoreInput{})
	for p, name := range simNames {
		if got := vp[name].Total(); got != res.VP[p] {
			out = append(out, fmt.Sprintf("%s's VP: Cartographer %d, engine %d", name, got, res.VP[p]))
		}
	}
	return out
}

// handWithPlay counts the hand and the cards in play as one zone.
func handWithPlay(zones map[string]map[string]int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for z, cs := range zones {
		if z == "in play" {
			z = "hand"
		}
		if out[z] == nil {
			out[z] = map[string]int{}
		}
		for c, n := range cs {
			out[z][c] += n
		}
	}
	return out
}

// simCheck replays saved games on the engine and reports how far each one matches.
func simCheck(args []string) error {
	fs := flag.NewFlagSet("sim check", flag.ExitOnError)
	verbose := fs.Bool("v", false, "one line per game")
	fs.Parse(args)
	t, err := engine.NewTable(cardsJSON)
	if err != nil {
		return err
	}
	client, err := loadClient()
	if err != nil {
		return err
	}
	paths := fs.Args()
	if len(paths) == 0 {
		dir, err := gamesDir("")
		if err != nil {
			return err
		}
		paths = []string{dir}
	}
	var logs []string
	for _, a := range paths {
		if st, err := os.Stat(a); err == nil && st.IsDir() {
			found, _ := filepath.Glob(filepath.Join(a, "*.log"))
			for _, f := range found {
				if !strings.HasSuffix(f, ".bot.log") {
					logs = append(logs, f)
				}
			}
		} else {
			logs = append(logs, a)
		}
	}
	turns, matched, done := 0, 0, 0
	missing := map[string]int{}
	stops := map[string]int{}
	for _, f := range logs {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		score, _ := os.ReadFile(strings.TrimSuffix(f, ".log") + ".score.txt")
		r := replay.Check(client, t, string(b), string(score))
		turns += r.Turns
		matched += r.Matched
		switch {
		case r.Done:
			done++
		case r.Missing != "":
			missing[r.Missing]++
		case r.Key != "":
			stops[r.Stop+" | real "+r.Key]++
		default:
			stops[r.Stop]++
		}
		if *verbose && !r.Done {
			name := filepath.Base(f)
			if r.Missing != "" {
				fmt.Printf("%s: %d/%d turns, then %s\n", name, r.Matched, r.Turns, r.Stop)
			} else {
				fmt.Printf("%s: %d/%d turns, %s: %s\n", name, r.Matched, r.Turns, r.Turn, r.Stop)
				if r.Real != "" || r.Engine != "" {
					fmt.Printf("  real:   %s\n  engine: %s\n", r.Real, r.Engine)
				}
			}
		}
	}
	fmt.Printf("%d games: %d replayed to the end; %d of %d turns (%.1f%%) match the real logs\n", len(logs), done, matched, turns, 100*float64(matched)/float64(max(turns, 1)))
	print := func(title string, m map[string]int, n int) {
		type kv struct {
			k string
			v int
		}
		var xs []kv
		for k, v := range m {
			xs = append(xs, kv{k, v})
		}
		sort.Slice(xs, func(i, j int) bool { return xs[i].v > xs[j].v || xs[i].v == xs[j].v && xs[i].k < xs[j].k })
		if len(xs) == 0 {
			return
		}
		fmt.Println(title)
		for _, x := range xs[:min(n, len(xs))] {
			fmt.Printf("  %3d  %s\n", x.v, x.k)
		}
	}
	print("stopped at a card without rules:", missing, 15)
	print("stopped where the engine differs:", stops, 30)
	return nil
}
