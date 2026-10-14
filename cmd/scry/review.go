package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yourconscience/scrying-pool/cartographer"
)

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

// scoresByTurn scores the game after each turn (index i is turns[i]) and at the end.
func scoresByTurn(c *cartographer.Client, g *cartographer.Game, in cartographer.ScoreInput) (byTurn []map[string]cartographer.VPBreakdown, final map[string]cartographer.VPBreakdown) {
	for _, end := range turnEnds(g) {
		prefix := *g
		prefix.Events = g.Events[:end]
		byTurn = append(byTurn, c.Score(&prefix, in))
	}
	return byTurn, c.Score(g, in)
}

// turnEnds gives, for each turn, the index of the first event after it.
func turnEnds(g *cartographer.Game) []int {
	var ends []int
	for i, e := range g.Events {
		if e.Template == "NEW_TURN" {
			ends = append(ends, i)
		}
	}
	if len(ends) == 0 {
		return nil
	}
	return append(ends[1:], len(g.Events))
}

// reviewMarkdown is the first version of a game review: final decks and the turn table.
func reviewMarkdown(c *cartographer.Client, g *cartographer.Game, in cartographer.ScoreInput) string {
	var w strings.Builder
	turns := g.Turns()
	decks := g.FinalDecks()
	byTurn, final := scoresByTurn(c, g, in)
	fmt.Fprintf(&w, "# %s\n\n", md(strings.Join(g.Players, " vs ")))
	if len(g.Header) > 0 {
		w.WriteString(md(strings.Join(g.Header, " · ")) + "\n\n")
	}
	w.WriteString("## Final decks\n\n")
	for _, p := range g.Players {
		fmt.Fprintf(&w, "- **%s** (%d VP: %s; %d cards): %s\n", md(p), final[p].Total(), md(vpLine(final[p])), deckSize(decks[p]), md(deckLine(decks[p])))
	}
	w.WriteString("\n## Turns\n\n| Turn | Player | $ | Bought | Also gained | Trashed | Actions | Drawn | Deck | VP |\n|--:|---|--:|---|---|---|--:|--:|--:|--:|\n")
	for i, t := range turns {
		turn := fmt.Sprint(t.Number)
		if t.Extra != "" {
			turn += " (" + t.Extra + ")"
		}
		d := t.Decks[t.Player]
		vp := byTurn[i][t.Player].Total()
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

// vpLine lists where VP come from, largest first: "Province 24, VP tokens 5, Curse -1".
func vpLine(b cartographer.VPBreakdown) string {
	type kv struct {
		k string
		v int
	}
	var xs []kv
	for k, v := range b {
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
		parts[i] = fmt.Sprintf("%s %d", x.k, x.v)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}
