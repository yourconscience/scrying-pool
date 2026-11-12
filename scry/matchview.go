//go:build !js

package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

// The match view of a game: where each player's points came from, the final
// decks, how the game ended and where the gap moved, and went-well / went-badly
// points from general rules. docs/match-review.md has the rules and what they
// were measured on.

const (
	thinGap     = 4   // Coppers and Estates trashed, you minus them
	provinceGap = 2   // Provinces (and Colonies), you minus them
	altGap      = 10  // VP from sources other than Province, Colony, Duchy, Estate and Curse
	curseGap    = 3   // Curses given minus received
	actionGap   = 6   // Action cards in the final deck, you minus them
	tempoGap    = 0.5 // $ per turn on turns 3-12, you minus them, on boards with less than altGap of other VP
)

type vpSource struct {
	Label  string `json:"label"`
	VP     int    `json:"vp"`
	Detail string `json:"detail,omitempty"`
}

type deckCard struct {
	Card string `json:"card"`
	N    int    `json:"n"`
	Kind string `json:"kind"` // action, treasure, victory or curse
}

// sideView is one player's side of the final position.
type sideView struct {
	VP        []vpSource `json:"vp"`
	Total     int        `json:"total"` // the VP above, added up
	Deck      []deckCard `json:"deck"`
	Cards     int        `json:"cards"`
	Provinces int        `json:"provinces"`       // Provinces and Colonies
	Thinned   int        `json:"thinned"`         // Coppers and Estates trashed
	Cursed    int        `json:"curses_received"` // Curses that came into the deck
	Actions   int        `json:"actions"`         // Action cards in the final deck
}

type endView struct {
	By        string `json:"by"` // you, them, or "" when the log doesn't say
	How       string `json:"how"`
	Turn      int    `json:"turn"` // the last turn's number
	Text      string `json:"text"`
	GapBefore int    `json:"gap_before"` // your VP minus theirs, before the last turn
	GapAfter  int    `json:"gap_after"`
}

type swingView struct {
	Turn      int    `json:"turn"`
	By        string `json:"by"` // you or them
	GapBefore int    `json:"gap_before"`
	GapAfter  int    `json:"gap_after"`
	Gained    string `json:"gained"`
	Text      string `json:"text"`
}

// gapPoint is the VP gap (yours minus theirs) after one player's turn.
type gapPoint struct {
	Turn int  `json:"turn"`
	Mine bool `json:"mine"`
	Gap  int  `json:"gap"`
}

type point struct {
	Rule string `json:"rule"`
	Good bool   `json:"good"`
	Text string `json:"text"`
}

type matchView struct {
	You    sideView   `json:"you"`
	Them   sideView   `json:"them"`
	Note   string     `json:"note,omitempty"` // when the log's last decision is missing
	End    endView    `json:"end"`
	Swing  *swingView `json:"swing,omitempty"`
	Gap    []gapPoint `json:"gap"`
	Points []point    `json:"points"`
}

// usualCoins is your mean $ per turn on turns 3-12 over your other games of the last 90 days.
func usualCoins(games []*advisor.Features) (coins float64, n int) {
	for _, f := range games {
		if len(f.Coins) >= 3 {
			coins += coinsMid(f.Coins)
			n++
		}
	}
	if n > 0 {
		coins /= float64(n)
	}
	return coins, n
}

var vpOrder = []string{"Province", "Colony", "Duchy", "Estate", "Other Victory cards", "VP tokens", "Landscapes and states", "Curse"}

// vpSources groups a player's VP breakdown by where the points came from.
func vpSources(client *cartographer.Client, b cartographer.VPBreakdown) (out []vpSource, total int) {
	out = []vpSource{}
	sums := map[string]int{}
	detail := map[string][]string{}
	keys := make([]string, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if b[keys[i]] != b[keys[j]] {
			return b[keys[i]] > b[keys[j]]
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		label := k
		switch k {
		case "Province", "Colony", "Duchy", "Estate", "Curse", "VP tokens":
		default:
			if c := client.Cards[k]; c != nil && c.Is("Victory") {
				label = "Other Victory cards"
			} else {
				label = "Landscapes and states"
			}
			detail[label] = append(detail[label], fmt.Sprintf("%s %+d", k, b[k]))
		}
		sums[label] += b[k]
	}
	for _, label := range vpOrder {
		if v, ok := sums[label]; ok && v != 0 {
			out = append(out, vpSource{Label: label, VP: v, Detail: strings.Join(detail[label], ", ")})
			total += v
		}
	}
	return out, total
}

func deckCards(client *cartographer.Client, d cartographer.Deck) []deckCard {
	out := []deckCard{}
	for name, n := range d {
		if n <= 0 {
			continue
		}
		kind := "action"
		if c := client.Cards[name]; c != nil {
			switch {
			case c.Is("Curse"):
				kind = "curse"
			case c.Is("Victory"):
				kind = "victory"
			case c.Is("Treasure"):
				kind = "treasure"
			}
		}
		out = append(out, deckCard{name, n, kind})
	}
	rank := map[string]int{"action": 0, "treasure": 1, "victory": 2, "curse": 3}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case rank[a.Kind] != rank[b.Kind]:
			return rank[a.Kind] < rank[b.Kind]
		case a.N != b.N:
			return a.N > b.N
		}
		return a.Card < b.Card
	})
	return out
}

// otherVP is the points from sources other than Province, Colony, Duchy, Estate and Curses.
func (s sideView) otherVP() int {
	n := 0
	for _, v := range s.VP {
		switch v.Label {
		case "Other Victory cards", "VP tokens", "Landscapes and states":
			n += v.VP
		}
	}
	return n
}

func sideOf(client *cartographer.Client, turns []cartographer.Turn, player string, bd cartographer.VPBreakdown, deck cartographer.Deck) sideView {
	v := sideView{Cards: deckSize(deck), Provinces: deck["Province"] + deck["Colony"], Deck: deckCards(client, deck)}
	v.VP, v.Total = vpSources(client, bd)
	for _, c := range v.Deck {
		if c.Kind == "action" {
			v.Actions += c.N
		}
	}
	prevCurses := 0
	for _, t := range turns {
		if n := t.Decks[player]["Curse"]; n > prevCurses {
			v.Cursed += n - prevCurses
			prevCurses = n
		} else {
			prevCurses = n
		}
		if t.Player == player {
			for _, c := range t.Trashed {
				if c.Card != nil && (c.Card.Name == "Copper" || c.Card.Name == "Estate") {
					v.Thinned += c.N
				}
			}
		}
	}
	return v
}

func sign(n int) string { return fmt.Sprintf("%+d", n) }

// matchViewOf reads one game. theirs is the opponent's features, nil if the game has none.
func matchViewOf(client *cartographer.Client, s *advisor.Saved, turns []cartographer.Turn, f, theirs *advisor.Features, usual []*advisor.Features) *matchView {
	me, opp := f.Me, f.Opponent
	byTurn, final := scoresByTurn(client, s.Game, cartographer.ScoreInput{Landscapes: client.Landscapes(s.Score)})
	if len(byTurn) != len(turns) {
		return nil
	}
	decks := s.Game.FinalDecks()
	v := &matchView{
		You:  sideOf(client, turns, me, final[me], decks[me]),
		Them: sideOf(client, turns, opp, final[opp], decks[opp]),
		Gap:  []gapPoint{},
	}
	if !f.Complete {
		v.Note = fmt.Sprintf("The log's last decision is missing: the points and decks here are %d to %d, the game's score %d to %d.", v.You.Total, v.Them.Total, f.MyVP, f.OppVP)
		if len(f.Final) == 2 {
			v.Note = fmt.Sprintf("The log's last decision is missing: the points and decks here are %d to %d, the game's score %d to %d.", v.You.Total, v.Them.Total, f.Final[0], f.Final[1])
		}
	}
	gap := func(i int) int {
		if i < 0 {
			return 0
		}
		return byTurn[i][me].Total() - byTurn[i][opp].Total()
	}
	best, move := -1, 0
	for i, t := range turns {
		v.Gap = append(v.Gap, gapPoint{Turn: t.Number, Mine: t.Player == me, Gap: gap(i)})
		if d := gap(i) - gap(i-1); abs(d) > abs(move) {
			best, move = i, d
		}
	}
	if best >= 0 {
		t := turns[best]
		sw := &swingView{Turn: t.Number, By: "you", GapBefore: gap(best - 1), GapAfter: gap(best), Gained: cardList(append(slices.Clone(t.Bought), t.Gained...))}
		who := "your"
		if t.Player != me {
			sw.By, who = "them", opp+"'s"
		}
		sw.Text = fmt.Sprintf("The largest single turn was %s turn %d: the gap went from %s to %s", who, t.Number, sign(sw.GapBefore), sign(sw.GapAfter))
		if sw.Gained != "" {
			sw.Text += ", with " + sw.Gained
		}
		sw.Text += "."
		v.Swing = sw
	}
	v.End = endOf(client, s, turns, me, opp, f, gap)
	v.Points = pointsOf(client, s, turns, f, theirs, v, usual)
	return v
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// endOf says how the game ended and who took its last turn. A resignation leaves
// no trace of who resigned, and a log that stops before the last decision looks
// like one, so the text says only what the log shows.
func endOf(client *cartographer.Client, s *advisor.Saved, turns []cartographer.Turn, me, opp string, f *advisor.Features, gap func(int) int) endView {
	last := len(turns) - 1
	e := endView{How: f.Ending, Turn: turns[last].Number, GapBefore: gap(last - 1), GapAfter: gap(last)}
	name := opp
	switch turns[last].Player {
	case me:
		e.By, name = "you", "you"
	default:
		e.By = "them"
	}
	supply := s.Game.Supply(client)
	switch f.Ending {
	case "provinces":
		pile := "Province"
		if _, ok := supply.Initial["Colony"]; ok {
			pile = "Colony"
		}
		e.Text = fmt.Sprintf("Ended on %ss: %d left, %s took the last turn (turn %d).", pile, max(supply.Left[pile], 0), name, e.Turn)
	case "piles":
		e.Text = fmt.Sprintf("Ended on piles (%s empty), %s took the last turn (turn %d).", strings.Join(supply.EmptyPiles(), ", "), name, e.Turn)
	default:
		e.By = ""
		e.Text = fmt.Sprintf("No Province or pile ending in the log: a resignation, or the log stops early. It doesn't say who resigned. The last turn was turn %d, the score then %d to %d.", e.Turn, f.MyVP, f.OppVP)
		if len(f.Final) == 2 {
			e.Text = fmt.Sprintf("No Province or pile ending in the log: a resignation, or the log stops early. It doesn't say who resigned. The last turn was turn %d, the score %d to %d.", e.Turn, f.Final[0], f.Final[1])
		}
	}
	return e
}

func numWord(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// kingdomCopies counts a deck's cards by kingdom pile.
func kingdomCopies(client *cartographer.Client, kingdom []string, d cartographer.Deck) map[string]int {
	out := map[string]int{}
	pile := map[string]string{}
	for _, k := range kingdom {
		pile[client.PileOf(k)] = k
	}
	for name, n := range d {
		if k, ok := pile[client.PileOf(name)]; ok {
			if c := client.Cards[name]; c != nil && c.Is("Victory") { // Castles, Nobles: the VP rule has them
				continue
			}
			out[k] += n
		}
	}
	return out
}

func pointsOf(client *cartographer.Client, s *advisor.Saved, turns []cartographer.Turn, f, theirs *advisor.Features, v *matchView, usual []*advisor.Features) []point {
	out := []point{}
	opp := f.Opponent
	add := func(rule string, good bool, format string, args ...any) {
		out = append(out, point{rule, good, fmt.Sprintf(format, args...)})
	}

	// thinning
	if d := v.You.Thinned - v.Them.Thinned; d >= thinGap || d <= -thinGap {
		add("thin", d > 0, "Coppers and Estates trashed: %d, %s %d.", v.You.Thinned, opp, v.Them.Thinned)
	}

	// Province split, with the first Province's turn when neither side has much other VP
	if d := v.You.Provinces - v.Them.Provinces; d >= provinceGap || d <= -provinceGap {
		text := fmt.Sprintf("Provinces: %d to %s's %d", v.You.Provinces, opp, v.Them.Provinces)
		if theirs != nil && f.FirstProvince > 0 && theirs.FirstProvince > 0 && v.You.otherVP() < altGap && v.Them.otherVP() < altGap {
			text += fmt.Sprintf("; your first on turn %d, theirs on turn %d", f.FirstProvince, theirs.FirstProvince)
		}
		add("provinces", d > 0, "%s.", text)
	}

	// other sources of points
	if d := v.You.otherVP() - v.Them.otherVP(); d >= altGap || d <= -altGap {
		add("alt", d > 0, "Points from sources other than Provinces, Duchies, Estates and Curses: %d of your %d VP, %s %d of %d.", v.You.otherVP(), v.You.Total, opp, v.Them.otherVP(), v.Them.Total)
	}

	// Curses
	given, received := v.Them.Cursed, v.You.Cursed
	if d := given - received; d >= curseGap || d <= -curseGap {
		add("curse", d > 0, "Curses: you gave %d and took %d.", given, received)
	}

	// what each built: Action cards in the final deck
	if d := v.You.Actions - v.Them.Actions; d >= actionGap || d <= -actionGap {
		add("actions", d > 0, "Action cards in the final deck: %d, %s %d.", v.You.Actions, opp, v.Them.Actions)
	}

	// the last turn
	if f.Complete && v.End.How != "resign" {
		switch {
		case f.Result == 0 && v.End.By == "them" && v.End.GapBefore > 0:
			add("end", false, "You led by %d before %s's last turn, which ended the game and moved the gap to %s.", v.End.GapBefore, opp, sign(v.End.GapAfter))
		case f.Result == 1 && v.End.By == "you" && v.End.GapBefore < 0:
			add("end", true, "You trailed by %d before your last turn, which ended the game and moved the gap to %s.", -v.End.GapBefore, sign(v.End.GapAfter))
		}
	}

	// $ per turn against the opponent in the same game, left out when other VP matter
	if theirs != nil && len(f.Coins) >= 3 && len(theirs.Coins) >= 3 && v.You.otherVP() < altGap && v.Them.otherVP() < altGap {
		mine, other := coinsMid(f.Coins), coinsMid(theirs.Coins)
		if d := mine - other; d >= tempoGap || d <= -tempoGap {
			text := fmt.Sprintf("$%.1f per turn on turns 3-12, %s $%.1f", mine, opp, other)
			if u, n := usualCoins(usual); n >= 20 {
				text += fmt.Sprintf("; your usual is $%.1f", u)
			}
			add("tempo", d > 0, "%s.", text)
		}
	}
	return out
}

// findings are the points that came up on the same side in 2 or more games of the match.
type finding struct {
	Rule  string `json:"rule"`
	Good  bool   `json:"good"`
	Games []int  `json:"games"`
	Text  string `json:"text"`
}

var ruleLabels = map[string][2]string{ // bad, good
	"thin":      {"Trashed fewer Coppers and Estates than the opponent", "Trashed more Coppers and Estates than the opponent"},
	"provinces": {"Fewer Provinces than the opponent", "More Provinces than the opponent"},
	"alt":       {"Fewer points from other sources than the opponent", "More points from other sources than the opponent"},
	"curse":     {"Took more Curses than you gave", "Gave more Curses than you took"},
	"actions":   {"Fewer Action cards than the opponent in the final deck", "More Action cards than the opponent in the final deck"},
	"end":       {"Led before the opponent's last turn and lost", "Trailed before your last turn and won"},
	"tempo":     {"Less $ per turn than the opponent", "More $ per turn than the opponent"},
}

func findingsOf(games []reportGame) []finding {
	type key struct {
		rule string
		good bool
	}
	by := map[key][]int{}
	for i, g := range games {
		if g.View == nil {
			continue
		}
		seen := map[key]bool{}
		for _, p := range g.View.Points {
			k := key{p.Rule, p.Good}
			if !seen[k] {
				seen[k] = true
				by[k] = append(by[k], i+1)
			}
		}
	}
	out := []finding{}
	for k, ns := range by {
		if len(ns) < 2 {
			continue
		}
		won := 0
		for _, n := range ns {
			if games[n-1].Result == 1 {
				won++
			}
		}
		label := ruleLabels[k.rule][0]
		if k.good {
			label = ruleLabels[k.rule][1]
		}
		out = append(out, finding{k.rule, k.good, ns, fmt.Sprintf("%s: %s, of which you won %d.", label, gameLabel(strNums(ns)), won)})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Games) != len(out[j].Games) {
			return len(out[i].Games) > len(out[j].Games)
		}
		return out[i].Text < out[j].Text
	})
	return out
}

func strNums(ns []int) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = fmt.Sprint(n)
	}
	return out
}

// seatSplit is your result by seat.
type seatSplit struct {
	Games int     `json:"games"`
	Score float64 `json:"score"`
}

func seatsOf(games []reportGame) (first, second seatSplit) {
	for _, g := range games {
		s := &second
		if g.First {
			s = &first
		}
		s.Games++
		s.Score += g.Result
	}
	return first, second
}

// viewMarkdown is the match view of one game, after the game's heading and facts.
func (g reportGame) viewMarkdown(b *strings.Builder) {
	v := g.View
	if v == nil {
		return
	}
	side := func(name string, s sideView) {
		parts := make([]string, len(s.VP))
		for i, x := range s.VP {
			parts[i] = fmt.Sprintf("%s %d", x.Label, x.VP)
			if x.Detail != "" {
				parts[i] += " (" + x.Detail + ")"
			}
		}
		fmt.Fprintf(b, "%s: %d VP: %s\n", name, s.Total, listOr(parts))
	}
	side("Points, you", v.You)
	side("Points, "+g.Opponent, v.Them)
	deck := func(name string, s sideView) {
		parts := make([]string, len(s.Deck))
		for i, c := range s.Deck {
			parts[i] = fmt.Sprintf("%d %s", c.N, c.Card)
		}
		fmt.Fprintf(b, "Final deck, %s (%d cards): %s\n", name, s.Cards, strings.Join(parts, ", "))
	}
	deck("you", v.You)
	deck(g.Opponent, v.Them)
	if v.Note != "" {
		fmt.Fprintf(b, "Note: %s\n", v.Note)
	}
	fmt.Fprintf(b, "Ending: %s\n", v.End.Text)
	if v.Swing != nil {
		fmt.Fprintf(b, "Swing: %s\n", v.Swing.Text)
	}
	for _, good := range []bool{true, false} {
		title := "Went badly"
		if good {
			title = "Went well"
		}
		n := 0
		for _, p := range v.Points {
			if p.Good == good {
				if n == 0 {
					fmt.Fprintf(b, "%s:\n", title)
				}
				fmt.Fprintf(b, "- %s\n", p.Text)
				n++
			}
		}
		if n == 0 {
			fmt.Fprintf(b, "%s: nothing the rules pick out.\n", title)
		}
	}
}
