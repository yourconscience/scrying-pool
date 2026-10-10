package main

import (
	"fmt"
	"sort"
	"strings"
)

// turnRow sums up one player's turn from the log.
type turnRow struct {
	Turn     int            `json:"turn"`
	Player   string         `json:"player"`
	Coins    int            `json:"coins"`   // $ from treasures and +$ this turn
	Bought   []string       `json:"bought"`  // cards and landscapes bought
	Gained   []string       `json:"gained"`  // other gains (Horses, Groom, Shaman, ...)
	Trashed  []string       `json:"trashed"` // what the turn's player trashed
	Actions  int            `json:"actions"` // Action cards played
	Drawn    int            `json:"drawn"`   // cards drawn, including the next hand
	Shuffles int            `json:"shuffles"`
	Deck     int            `json:"deck"` // deck size after the turn
	VP       int            `json:"vp"`   // VP after the turn (basic Victory cards only)
	deck     map[string]int // snapshot, for later analysis
}

// turnTable replays the log turn by turn. Decks change on every player's
// events, including the other player's turns (attacks, start-of-turn gains).
func (g *Game) turnTable() []turnRow {
	decks := map[string]map[string]int{}
	for _, p := range g.Players {
		decks[p] = map[string]int{}
	}
	var rows []turnRow
	var cur *turnRow
	finish := func() {
		if cur == nil {
			return
		}
		cur.deck = copyDeck(decks[cur.Player])
		cur.Deck = deckSize(cur.deck)
		cur.VP, _ = deckVP(cur.deck)
		rows = append(rows, *cur)
		cur = nil
	}
	for _, e := range g.Events {
		if e.Verb == "turn" {
			finish()
			cur = &turnRow{Turn: e.Turn, Player: e.Player, Bought: []string{}, Gained: []string{}, Trashed: []string{}}
		}
		if d := deckChange(e.Verb); d != 0 {
			for _, c := range e.Cards {
				decks[e.Player][c.Card] += d * c.N
			}
		}
		if cur == nil || e.Player != cur.Player {
			continue
		}
		cur.Coins += e.Coins
		switch e.Verb {
		case "buy", "buy-gain":
			cur.Bought = addCards(cur.Bought, e.Cards)
		case "gain":
			cur.Gained = addCards(cur.Gained, e.Cards)
		case "trash":
			cur.Trashed = addCards(cur.Trashed, e.Cards)
		case "draw":
			for _, c := range e.Cards {
				cur.Drawn += c.N
			}
		case "shuffle":
			cur.Shuffles++
		case "play":
			for _, c := range e.Cards {
				if card, ok := cardIndex[cardKey(c.Card)]; ok && hasType(card, "Action") {
					cur.Actions += c.N
				}
			}
		}
	}
	finish()
	return rows
}

// addCards adds cards to a "2× Horse, Groom" list, keeping first-seen order.
func addCards(list []string, cs []cardN) []string {
	counts := map[string]int{}
	var order []string
	for _, item := range list {
		n, name := 1, item
		if a, b, ok := strings.Cut(item, "× "); ok {
			fmt.Sscan(a, &n)
			name = b
		}
		counts[name] += n
		order = append(order, name)
	}
	for _, c := range cs {
		name := c.Card
		if name == "" {
			name = "card"
		}
		if counts[name] == 0 {
			order = append(order, name)
		}
		counts[name] += c.N
	}
	out := make([]string, 0, len(order))
	for _, name := range order {
		if counts[name] > 1 {
			out = append(out, fmt.Sprintf("%d× %s", counts[name], name))
		} else {
			out = append(out, name)
		}
	}
	return out
}

func copyDeck(d map[string]int) map[string]int {
	out := make(map[string]int, len(d))
	for k, v := range d {
		if v != 0 {
			out[k] = v
		}
	}
	return out
}

func deckSize(d map[string]int) int {
	n := 0
	for _, v := range d {
		n += v
	}
	return n
}

// reviewMarkdown is the first version of a game review: final decks and the turn table.
func (g *Game) reviewMarkdown() string {
	var w strings.Builder
	rows := g.turnTable()
	decks := g.finalDecks()
	title := strings.Join(g.Players, " vs ")
	fmt.Fprintf(&w, "# %s\n\n", md(title))
	if len(g.Header) > 0 {
		w.WriteString(md(strings.Join(g.Header, " · ")) + "\n\n")
	}
	w.WriteString("## Final decks\n\n")
	for _, p := range g.Players {
		vp, unscored := deckVP(decks[p])
		fmt.Fprintf(&w, "- **%s** (%d VP", md(p), vp)
		if len(unscored) > 0 {
			fmt.Fprintf(&w, " plus unscored %s", md(strings.Join(unscored, ", ")))
		}
		fmt.Fprintf(&w, ", %d cards): %s\n", deckSize(decks[p]), md(deckLine(decks[p])))
	}
	w.WriteString("\n## Turns\n\n| Turn | Player | $ | Bought | Also gained | Trashed | Actions | Drawn | Deck | VP |\n|--:|---|--:|---|---|---|--:|--:|--:|--:|\n")
	for _, r := range rows {
		fmt.Fprintf(&w, "| %d | %s | %d | %s | %s | %s | %d | %d | %d | %d |\n",
			r.Turn, md(r.Player), r.Coins, md(strings.Join(r.Bought, ", ")), md(strings.Join(r.Gained, ", ")),
			md(strings.Join(r.Trashed, ", ")), r.Actions, r.Drawn, r.Deck, r.VP)
	}
	if len(g.Unknown) > 0 {
		fmt.Fprintf(&w, "\n> **Note:** %d log lines weren't understood and are left out, first: line %d %q\n", len(g.Unknown), g.Unknown[0].Line, g.Unknown[0].Text)
	}
	return w.String()
}

// deckLine lists a deck largest pile first: "7 Groom, 7 Hostelry, 6 Estate, ...".
func deckLine(d map[string]int) string {
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
		name := x.k
		if name == "" {
			name = "unknown"
		}
		parts[i] = fmt.Sprintf("%d %s", x.v, name)
	}
	return strings.Join(parts, ", ")
}
