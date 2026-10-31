//go:build !js

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
)

// scoutData is what scout fetches about a player from dominion.games. It's kept
// in the games folder as scout-<player>.json, so a report can be made again
// without the browser.
type scoutData struct {
	Player    string        `json:"player"`
	ID        int           `json:"id"`
	Me        string        `json:"me"`
	Fetched   string        `json:"fetched"`
	Summary   *scoutSummary `json:"summary"`
	MySummary *scoutSummary `json:"my_summary"`
	// the player's record against you
	HeadToHead *struct {
		Wins     int     `json:"wins"`
		Losses   int     `json:"losses"`
		Ties     int     `json:"ties"`
		Expected float64 `json:"expected"` // the player's expected score against you
	} `json:"headToHead"`
	Games []scoutGame `json:"games"` // newest first
}

// scoutSummary is a profile's summary on dominion.games, for two-player games.
type scoutSummary struct {
	Wins             int     `json:"wins"`
	Losses           int     `json:"losses"`
	Ties             int     `json:"ties"`
	CurrentRank      int     `json:"currentRank"`
	CurrentLevel     float64 `json:"currentLevel"`
	BestWinStreak    int     `json:"bestWinStreak"`
	BestLossStreak   int     `json:"bestLossStreak"`
	CurrentSkill     float64 `json:"currentSkill"`     // μ
	CurrentDeviation float64 `json:"currentDeviation"` // φ
}

// scoutGame is a game as the player's profile lists it.
type scoutGame struct {
	ID       string             `json:"id"`
	Time     string             `json:"time"`
	Rated    bool               `json:"rated"`
	Players  []string           `json:"players"`
	Levels   map[string]float64 `json:"levels"`
	Scores   map[string]int     `json:"scores"`
	Expected float64            `json:"expected"` // the player's expected score, from both ratings
}

func scoutFile(dir, player string) string { return filepath.Join(dir, "scout-"+slug(player)+".json") }

// scoutList is how many of a player's latest two-player games scout asks the
// server for: the most it lists.
const scoutList = 100

// scoutFetch saves what dominion.games in Brave lists about a player: their
// latest two-player games against people, their profile's summary, yours and
// your head-to-head record. Then it loads their games, newest first, until n
// are saved in dir: some games don't load (the server can't, or a player was
// AFK), and older ones take their place.
func scoutFetch(player, dir string, n, minTurns int, budget time.Duration) error {
	t, done, err := captureTab("scout")
	if err != nil {
		return err
	}
	defer done()
	name, _ := json.Marshal(player)
	out, err := t.page(fmt.Sprintf("return await window.scryCapture.scout(%s, %d);", name, scoutList), 2*time.Minute)
	if err != nil {
		return err
	}
	var d scoutData
	if err := json.Unmarshal(out, &d); err != nil {
		return err
	}
	if d.Player != d.Me { // your own summary, from your profile
		me, _ := json.Marshal(d.Me)
		if out, err = t.page(fmt.Sprintf("return await window.scryCapture.scout(%s, 0);", me), 2*time.Minute); err != nil {
			return err
		}
		var mine scoutData
		if err := json.Unmarshal(out, &mine); err != nil {
			return err
		}
		d.MySummary = mine.Summary
	}
	d.Fetched = time.Now().Format(time.RFC3339)
	b, _ := json.MarshalIndent(d, "", " ")
	if err := os.WriteFile(scoutFile(dir, d.Player), b, 0o644); err != nil {
		return err
	}
	tried := map[string]bool{}
	for start := time.Now(); ; {
		have := 0
		for _, g := range d.Games {
			if isSaved(dir, g.ID) {
				have++
			}
		}
		var todo []string
		for _, g := range d.Games {
			if len(todo) < n-have && !isSaved(dir, g.ID) && !tried[g.ID] {
				todo = append(todo, g.ID)
				tried[g.ID] = true
			}
		}
		fmt.Fprintf(os.Stderr, "%s: %d latest games against people listed, %d saved, loading %d more\n", d.Player, len(d.Games), have, len(todo))
		left := budget - time.Since(start)
		if len(todo) == 0 || left <= 0 {
			return nil
		}
		if err := captureGames(t, todo, dir, minTurns, left); err != nil {
			return err
		}
	}
}

// isSaved reports whether the game is saved in dir.
func isSaved(dir, id string) bool {
	_, err := os.Stat(filepath.Join(dir, id+".log"))
	return err == nil
}

// scoutedPlayer is a player scouted before, for the Scout page.
type scoutedPlayer struct {
	Player  string `json:"player"`
	Fetched string `json:"fetched"`
	Listed  int    `json:"listed"` // their games against people the server listed
	Saved   int    `json:"saved"`  // those saved
}

// scouted lists the players fetched into dir, the latest fetch first.
func scouted(dir string) []scoutedPlayer {
	files, _ := filepath.Glob(filepath.Join(dir, "scout-*.json"))
	out := []scoutedPlayer{}
	for _, f := range files {
		var d scoutData
		if b, err := os.ReadFile(f); err != nil || json.Unmarshal(b, &d) != nil {
			continue
		}
		p := scoutedPlayer{Player: d.Player, Fetched: d.Fetched, Listed: len(d.Games)}
		for _, g := range d.Games {
			if isSaved(dir, g.ID) {
				p.Saved++
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fetched > out[j].Fetched })
	return out
}

// scoutRun is the Scout page's run of `scry scout`. There's one at a time, as
// it drives the one dominion.games tab.
type scoutRun struct {
	Player  string   `json:"player"`
	Started string   `json:"started"`
	Running bool     `json:"running"`
	Log     []string `json:"log"`
	Error   string   `json:"error,omitempty"`
}

var (
	scoutMu  sync.Mutex
	scoutNow = scoutRun{Log: []string{}}
)

// startScout runs `scry scout` in a process of its own for the Scout page and
// keeps its output. The report goes next to the fetch, as scout-<player>.md.
func startScout(player, dir string, n int) error {
	scoutMu.Lock()
	defer scoutMu.Unlock()
	if scoutNow.Running {
		return fmt.Errorf("already scouting %s", scoutNow.Player)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	md := strings.TrimSuffix(scoutFile(dir, player), ".json") + ".md"
	// scry's own binary, without a shell; the name can't be read as a flag after "--"
	cmd := exec.Command(exe, "scout", "-dir", dir, "-n", strconv.Itoa(n), "-o", md, "--", player) // nosemgrep: go.lang.security.audit.dangerous-exec-command
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = w, w
	err = cmd.Start()
	w.Close()
	if err != nil {
		r.Close()
		return err
	}
	scoutNow = scoutRun{Player: player, Started: time.Now().Format(time.RFC3339), Running: true, Log: []string{}}
	go func() {
		defer r.Close()
		for sc := bufio.NewScanner(r); sc.Scan(); {
			scoutMu.Lock()
			scoutNow.Log = append(scoutNow.Log, sc.Text())
			scoutMu.Unlock()
		}
		err := cmd.Wait()
		scoutMu.Lock()
		defer scoutMu.Unlock()
		scoutNow.Running = false
		if err != nil {
			scoutNow.Error = err.Error()
			if k := len(scoutNow.Log); k > 0 && strings.HasPrefix(scoutNow.Log[k-1], "scry: ") {
				scoutNow.Error = strings.TrimPrefix(scoutNow.Log[k-1], "scry: ")
			}
		}
	}()
	return nil
}

// scoutStatus is the Scout page's latest run.
func scoutStatus() scoutRun {
	scoutMu.Lock()
	defer scoutMu.Unlock()
	s := scoutNow
	s.Log = slices.Clone(s.Log)
	return s
}

// scoutReport is what scout writes as JSON.
type scoutReport struct {
	Data       *scoutData          `json:"dominion_games,omitempty"`
	Them       *advisor.Side       `json:"them"`
	Me         *advisor.Side       `json:"me"`
	HeadToHead []*advisor.Features `json:"head_to_head"` // your saved games against them, from your side
}

// scout compares a player's latest saved games with your games of the last
// siteDays days, with what scoutFetch saved about them when there is a file.
func scout(player, me, dir string, n int, asJSON bool) (string, error) {
	client, err := loadClient()
	if err != nil {
		return "", err
	}
	account, profiles, features, err := advisor.Corpus(client, dir)
	if err != nil {
		return "", err
	}
	var data *scoutData
	if b, err := os.ReadFile(scoutFile(dir, player)); err == nil {
		data = &scoutData{}
		if err := json.Unmarshal(b, data); err != nil {
			return "", fmt.Errorf("%s: %w", scoutFile(dir, player), err)
		}
		player = data.Player // as dominion.games spells it
		if me == "" {
			me = data.Me
		}
	}
	if me == "" {
		me = account
	}
	// theirs: their n latest saved games among those the server listed (or among all, without a fetch)
	theirs := advisor.Of(player, features)
	if data != nil {
		ids := map[string]bool{}
		for _, g := range data.Games {
			ids[g.ID] = true
		}
		theirs = slices.DeleteFunc(theirs, func(f *advisor.Features) bool { return !ids[f.ID] })
	}
	if len(theirs) > n {
		theirs = theirs[len(theirs)-n:] // saved in game-number order
	}
	if len(theirs) == 0 {
		return "", fmt.Errorf("no saved games of %q; run scout without -offline to load them", player)
	}
	roles := func(card string) []string {
		var types []string
		if c := client.Cards[card]; c != nil {
			types = c.Types
		}
		return advisor.CardRoles(card, profiles, types)
	}
	r := &scoutReport{Data: data, Them: advisor.Scout(player, theirs, roles), Me: advisor.Scout(me, lastDays(advisor.Of(me, features), siteDays), roles)}
	for _, f := range advisor.Of(me, features) {
		if f.Opponent == player {
			r.HeadToHead = append(r.HeadToHead, f)
		}
	}
	if asJSON {
		b, err := json.MarshalIndent(r, "", " ")
		return string(b) + "\n", err
	}
	return r.markdown(), nil
}

func pct(x float64) string { return fmt.Sprintf("%.0f%%", 100*x) }

// rateText is "54% (40-67%), 7 of 13".
func rateText(r advisor.Rate) string {
	if r.Games == 0 {
		return "-"
	}
	return fmt.Sprintf("%s (%.0f-%.0f%%), %s of %d", pct(r.Rate), 100*max(0, r.Low), 100*min(1, r.High), strings.TrimSuffix(fmt.Sprintf("%.1f", r.Wins), ".0"), r.Games)
}

func turnText(x float64) string {
	if x == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", x)
}

func (r *scoutReport) markdown() string {
	var b strings.Builder
	them, me := r.Them, r.Me
	p := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	row := func(cells ...string) { p("| %s |", strings.Join(cells, " | ")) }
	head := func(cells ...string) {
		row(cells...)
		row(slices.Repeat([]string{"---"}, len(cells))...)
	}
	byName := func(rs []advisor.Row) map[string]advisor.Rate {
		m := map[string]advisor.Rate{}
		for _, r := range rs {
			m[r.Name] = r.Rate
		}
		return m
	}
	p("# %s, scouted for %s", them.Name, me.Name)
	p("")
	p("%s: %d games against people, %s to %s. %s: %d games of the last %d days, %s to %s.", them.Name, them.Games, them.First, them.Last, me.Name, me.Games, siteDays, me.First, me.Last)
	if d := r.Data; d != nil {
		p("Fetched from dominion.games on %s.", strings.Replace(d.Fetched, "T", " ", 1))
	}
	p("Ranges are 80%% intervals: with %d games most differences under 20 points are chance.", them.Games)

	if d := r.Data; d != nil && d.Summary != nil {
		p("\n## dominion.games profile (two-player)\n")
		s, m := d.Summary, d.MySummary
		if m == nil {
			m = &scoutSummary{}
		}
		head("", them.Name, me.Name)
		row("Level", fmt.Sprintf("%.2f", s.CurrentLevel), fmt.Sprintf("%.2f", m.CurrentLevel))
		row("μ / φ", fmt.Sprintf("%.2f / %.3f", s.CurrentSkill, s.CurrentDeviation), fmt.Sprintf("%.2f / %.3f", m.CurrentSkill, m.CurrentDeviation))
		rank := func(n int) string {
			if n == 0 {
				return "unranked" // the leaderboard leaves out players with a high φ
			}
			return fmt.Sprintf("#%d", n)
		}
		row("Rank", rank(s.CurrentRank), rank(m.CurrentRank))
		row("Wins-losses-ties", fmt.Sprintf("%d-%d-%d", s.Wins, s.Losses, s.Ties), fmt.Sprintf("%d-%d-%d", m.Wins, m.Losses, m.Ties))
		row("Longest win / loss streak", fmt.Sprintf("%d / %d", s.BestWinStreak, s.BestLossStreak), fmt.Sprintf("%d / %d", m.BestWinStreak, m.BestLossStreak))
		if s.CurrentDeviation >= 0.3 { // about twice an active player's
			p("\nTheir φ is high, as after months without rated games. The level is 50 + 7.5 × (μ − 2φ), so it understates them: μ, the skill estimate, is %.2f against your %.2f.", s.CurrentSkill, m.CurrentSkill)
		}
		var won, expected float64
		for _, g := range d.Games {
			a, b := g.Scores[d.Player], 0
			for name, v := range g.Scores {
				if name != d.Player {
					b = v
				}
			}
			switch {
			case a > b:
				won++
			case a == b:
				won += 0.5
			}
			expected += g.Expected
		}
		p("\nIn the %d latest two-player games against people the server lists, %s scored %.1f against %.1f their ratings predicted.", len(d.Games), d.Player, won, expected)
		if h := d.HeadToHead; h != nil {
			p("Head-to-head on dominion.games: %s is %d-%d-%d against %s; their ratings give %s an expected score of %s.", them.Name, h.Wins, h.Losses, h.Ties, me.Name, them.Name, pct(h.Expected))
		}
	}

	p("\n## Results\n")
	head("", them.Name, me.Name)
	row("Win rate", rateText(them.Record), rateText(me.Record))
	if them.Leveled+me.Leveled > 0 {
		row("Wins against the levels' prediction", fmt.Sprintf("%+.1f over %d", them.Won-them.Expected, them.Leveled), fmt.Sprintf("%+.1f over %d", me.Won-me.Expected, me.Leveled))
	}
	row("Level in the latest game", fmt.Sprintf("%.2f", them.Level), fmt.Sprintf("%.2f", me.Level))
	row("Opponents' average level", fmt.Sprintf("%.2f", them.OppLevel), fmt.Sprintf("%.2f", me.OppLevel))
	row("Against a higher-rated player", rateText(them.Stronger.With), rateText(me.Stronger.With))
	row("Against a lower-rated player", rateText(them.Stronger.Without), rateText(me.Stronger.Without))
	row("As first player", rateText(them.Seat.With), rateText(me.Seat.With))
	row("As second player", rateText(them.Seat.Without), rateText(me.Seat.Without))
	row("Average VP margin", fmt.Sprintf("%+.1f", them.Margin), fmt.Sprintf("%+.1f", me.Margin))
	row("Turns per game", turnText(them.Turns), turnText(me.Turns))
	row("First Gold, turn", turnText(them.FirstGold), turnText(me.FirstGold))
	row("First Province, turn", turnText(them.FirstProvince), turnText(me.FirstProvince))
	row("Games without a Province", fmt.Sprint(them.NoProvince), fmt.Sprint(me.NoProvince))
	for _, e := range []string{"provinces", "piles", "resign"} {
		row("Ended on "+e, rateText(them.Endings[e]), rateText(me.Endings[e]))
	}
	tl, ml := byName(them.Length), byName(me.Length)
	for _, l := range []string{"up to 14 turns", "15-19 turns", "20 turns or more"} {
		row("Games of "+l, rateText(tl[l]), rateText(ml[l]))
	}

	p("\n## Openings\n")
	head("Coppers on turns 1/2", them.Name, me.Name)
	for _, s := range []string{"5/2", "4/3", "3/4", "2/5"} {
		if them.Splits[s].Games+me.Splits[s].Games > 0 {
			row(s, rateText(them.Splits[s]), rateText(me.Splits[s]))
		}
	}
	p("\nWhat they opened with (turns 1-2):\n")
	ts, mshape := byName(them.Shapes), byName(me.Shapes)
	head("Opening", them.Name, me.Name)
	for _, s := range []string{"two kingdom cards", "a kingdom card and Silver", "a kingdom card, no Silver", "Silver and Silver", "one Silver, no kingdom card", "no kingdom card, no Silver"} {
		if ts[s].Games+mshape[s].Games > 0 {
			row(s, rateText(ts[s]), rateText(mshape[s]))
		}
	}
	for i, s := range them.OpenedAs {
		if s.With.Games+me.OpenedAs[i].With.Games > 0 {
			row(s.Name, rateText(s.With), rateText(me.OpenedAs[i].With))
		}
	}
	var again []string
	for _, o := range them.Openings {
		if o.Rate.Games >= 2 {
			again = append(again, fmt.Sprintf("%s (%s)", o.Name, rateText(o.Rate)))
		}
	}
	if len(again) > 0 {
		p("\nOpenings %s repeated: %s.", them.Name, strings.Join(again, "; "))
	} else {
		p("\n%s opened differently in every game.", them.Name)
	}
	p("\nKingdom cards %s opened with, against yours:\n", them.Name)
	mo := byName(me.Opened)
	head("Card", them.Name, me.Name)
	for _, o := range them.Opened[:min(12, len(them.Opened))] {
		row(o.Name, rateText(o.Rate), rateText(mo[o.Name]))
	}

	splits := func(title, intro string, a, c []advisor.Split) {
		p("\n## %s\n\n%s\n", title, intro)
		head("", them.Name+": share, win rate", them.Name+" otherwise", me.Name+": share, win rate", me.Name+" otherwise")
		for i, s := range a {
			share := func(s advisor.Split) string {
				n := s.With.Games + s.Without.Games
				if n == 0 {
					return "-"
				}
				return fmt.Sprintf("%s, %s", pct(float64(s.With.Games)/float64(n)), rateText(s.With))
			}
			row(s.Name, share(s), rateText(s.Without), share(c[i]), rateText(c[i].Without))
		}
	}
	splits("Styles", "What they built, by the roles of the kingdom cards they took (roles come from how the cards played in all saved games).", them.Styles, me.Styles)
	splits("Kingdoms", "What the kingdom offered, whether they used it or not.", them.Kingdoms, me.Kingdoms)

	p("\n## Cards\n\nKingdom cards in at least 2 of %s's games, the ones they took most first:\n", them.Name)
	head("Card", "Kingdoms", "Took it", "Win rate when taken", "Win rate when skipped")
	for _, c := range them.Took[:min(20, len(them.Took))] {
		row(c.Card, fmt.Sprint(c.Present), fmt.Sprint(c.Took.Games), rateText(c.Took), rateText(c.Skipped))
	}
	if len(them.Skipped) > 0 {
		var names []string
		for _, c := range them.Skipped[:min(15, len(them.Skipped))] {
			names = append(names, fmt.Sprintf("%s (%d of %d)", c.Card, c.Skipped.Games, c.Present))
		}
		p("\nUsually left alone: %s.", strings.Join(names, ", "))
	}

	if notes := r.notes(); len(notes) > 0 {
		p("\n## Where %s differs most\n\nSplits with at least 4 games on each side and 20 points or more between them, largest first:\n", them.Name)
		for _, n := range notes {
			p("- %s", n)
		}
	}

	p("\n## Your saved games against %s\n", them.Name)
	if len(r.HeadToHead) == 0 {
		p("None.")
	} else {
		head("Date", "Result", "Seat", "Kingdom")
		for _, f := range r.HeadToHead {
			res := map[float64]string{1: "won", 0.5: "tied", 0: "lost"}[f.Result]
			score := fmt.Sprintf("%d-%d", f.MyVP, f.OppVP)
			if len(f.Final) == 2 {
				score = fmt.Sprintf("%d-%d", f.Final[0], f.Final[1])
			}
			seat := "second"
			if f.First {
				seat = "first"
			}
			row(f.Date, res+" "+score, seat, strings.Join(f.Kingdom, ", "))
		}
	}
	return b.String()
}

// notes lists the opponent's splits that look like more than chance.
func (r *scoutReport) notes() []string {
	type note struct {
		text string
		diff float64
	}
	var out []note
	add := func(what string, s advisor.Split) {
		if s.With.Games < 4 || s.Without.Games < 4 || math.Abs(s.Diff) < 0.2 {
			return
		}
		out = append(out, note{fmt.Sprintf("%s: %s with, %s without (%+.0f points)", what, rateText(s.With), rateText(s.Without), 100*s.Diff), math.Abs(s.Diff)})
	}
	them := r.Them
	add("as first player", them.Seat)
	add("against a higher-rated player", them.Stronger)
	for _, s := range them.OpenedAs {
		add(s.Name, s)
	}
	for _, s := range them.Styles {
		add(s.Name+" ("+s.Desc+")", s)
	}
	for _, s := range them.Kingdoms {
		add(s.Name, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].diff > out[j].diff })
	texts := make([]string, len(out))
	for i, n := range out {
		texts[i] = n.text
	}
	return texts
}
