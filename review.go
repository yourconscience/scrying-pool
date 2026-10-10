package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yourconscience/scrying-pool/cartographer"
)

// basicVP covers the cards whose VP is fixed. Others (Gardens, Duke, ...) need
// game state and are reported as unscored for now.
var basicVP = map[string]int{"Estate": 1, "Duchy": 3, "Province": 6, "Colony": 10, "Curse": -1}

func hasType(c *cartographer.Card, t string) bool {
	for _, x := range c.Types {
		if x == t {
			return true
		}
	}
	return false
}

// cardList renders cards as "2× Horse, Groom", adding up repeats in first-seen order.
func cardList(cs []cartographer.CardN) string {
	counts := map[string]int{}
	var order []string
	for _, c := range cs {
		name := "card"
		if c.Card != nil {
			name = c.Card.Name
		}
		if counts[name] == 0 {
			order = append(order, name)
		}
		counts[name] += c.N
	}
	out := make([]string, len(order))
	for i, name := range order {
		out[i] = name
		if counts[name] > 1 {
			out[i] = fmt.Sprintf("%d× %s", counts[name], name)
		}
	}
	return strings.Join(out, ", ")
}

func deckSize(d cartographer.Deck) int {
	n := 0
	for _, v := range d {
		n += v
	}
	return n
}

// deckVP scores the basic Victory cards and Curses; other Victory cards are listed as unscored.
func deckVP(c *cartographer.Client, d cartographer.Deck) (vp int, unscored []string) {
	for name, n := range d {
		if v, ok := basicVP[name]; ok {
			vp += v * n
		} else if card := c.Cards[name]; card != nil && hasType(card, "Victory") {
			unscored = append(unscored, name)
		}
	}
	sort.Strings(unscored)
	return vp, unscored
}

// reviewMarkdown is the first version of a game review: final decks and the turn table.
func reviewMarkdown(c *cartographer.Client, g *cartographer.Game) string {
	var w strings.Builder
	turns := g.Turns()
	decks := g.FinalDecks()
	fmt.Fprintf(&w, "# %s\n\n", md(strings.Join(g.Players, " vs ")))
	if len(g.Header) > 0 {
		w.WriteString(md(strings.Join(g.Header, " · ")) + "\n\n")
	}
	w.WriteString("## Final decks\n\n")
	for _, p := range g.Players {
		vp, unscored := deckVP(c, decks[p])
		if len(turns) > 0 {
			vp += turns[len(turns)-1].Counters[p].VPTokens
		}
		fmt.Fprintf(&w, "- **%s** (%d VP from cards and tokens", md(p), vp)
		if len(unscored) > 0 {
			fmt.Fprintf(&w, " plus unscored %s", md(strings.Join(unscored, ", ")))
		}
		fmt.Fprintf(&w, ", %d cards): %s\n", deckSize(decks[p]), md(deckLine(decks[p])))
	}
	w.WriteString("\n## Turns\n\n| Turn | Player | $ | Bought | Also gained | Trashed | Actions | Drawn | Deck | VP |\n|--:|---|--:|---|---|---|--:|--:|--:|--:|\n")
	for _, t := range turns {
		turn := fmt.Sprint(t.Number)
		if t.Extra != "" {
			turn += " (" + t.Extra + ")"
		}
		d := t.Decks[t.Player]
		vp, _ := deckVP(c, d)
		vp += t.Counters[t.Player].VPTokens
		fmt.Fprintf(&w, "| %s | %s | %d | %s | %s | %s | %d | %d | %d | %d |\n",
			md(turn), md(t.Player), t.Coins, md(cardList(t.Bought)), md(cardList(t.Gained)),
			md(cardList(t.Trashed)), t.Actions, t.Drawn, deckSize(d), vp)
	}
	if len(g.Unknown) > 0 {
		fmt.Fprintf(&w, "\n> **Note:** %d log lines weren't understood and are left out, first: line %d %q\n", len(g.Unknown), g.Unknown[0].Line, g.Unknown[0].Text)
	}
	return w.String()
}

// deckLine lists a deck largest pile first: "7 Groom, 7 Hostelry, 6 Estate, ...".
func deckLine(d cartographer.Deck) string {
	type kv struct {
		k string
		v int
	}
	var xs []kv
	for k, v := range d {
		xs = append(xs, kv{k, v})
	}
	sort.Slice(xs, func(i, j int) bool {
		if xs[i].v != xs[j].v {
			return xs[i].v > xs[j].v
		}
		return xs[i].k < xs[j].k
	})
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprintf("%d %s", x.v, x.k)
	}
	return strings.Join(parts, ", ")
}
