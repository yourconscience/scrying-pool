package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yourconscience/scrying-pool/cartographer"
)

// turnRow sums up one player's turn from the log.
type turnRow struct {
	Turn     int      `json:"turn"`
	Player   string   `json:"player"`
	Extra    string   `json:"extra,omitempty"` // what gave an extra turn, e.g. "Voyage"
	Coins    int      `json:"coins"`           // $ from treasures, +$ and Coffers this turn
	Bought   []string `json:"bought"`          // cards and landscapes bought
	Gained   []string `json:"gained"`          // other gains (Horses, Groom, Shaman, ...)
	Trashed  []string `json:"trashed"`         // what the turn's player trashed
	Actions  int      `json:"actions"`         // Action cards played
	Drawn    int      `json:"drawn"`           // cards drawn, including the next hand
	Shuffles int      `json:"shuffles"`
	Deck     int      `json:"deck"` // deck size after the turn
	VP       int      `json:"vp"`   // VP after the turn: basic Victory cards, Curses and VP tokens
}

// Templates by turn-table column.
var (
	coinArg = map[string]int{ // which argument holds the $
		"PLAY_TREASURES_FOR": 2, "PLAY_TREASURES_AGAIN_FOR": 2, "PLAY_TREASURES_THIRD_FOR": 2,
		"GETS_COIN": 1, "GETS_COINS": 1, "GETS_COIN_FROM": 1, "GETS_COINS_FROM": 1,
		"USES_COFFER": 1, "USES_COFFERS": 1,
	}
	boughtT  = map[string]bool{"BUY": true, "BUY_AND_GAIN": true}
	gainedT  = map[string]bool{"GAIN": true, "GAIN_WITH": true, "GAIN_ON_DRAWPILE": true, "GAIN_FROM_TRASH": true, "GAIN_SETTING_ASIDE": true, "GAIN_TO_HAND": true}
	trashedT = map[string]bool{"TRASH": true, "TRASH_WITH": true}
	playedT  = map[string]bool{"PLAY": true, "PLAY_AGAIN": true, "PLAY_THIRD": true, "PLAY_WITH": true, "PLAY_AGAIN_WITH": true, "PLAY_THIRD_WITH": true, "PLAY_USING_WAY": true, "REACTS_PLAYING": true}
	drawnT   = map[string]bool{"DRAW": true, "DRAW_FROM": true, "DRAWS_FROM": true}
	vpArg    = map[string]int{"PLAYER_GETS_VP": 1, "PLAYER_GETS_VP_FROM": 1, "TAKE_VP_FROM": 1}
)

// basicVP covers the cards whose VP is fixed. Others (Gardens, Duke, ...) need
// game state and are reported as unscored for now.
var basicVP = map[string]int{"Estate": 1, "Duchy": 3, "Province": 6, "Colony": 10, "Curse": -1}

func number(a cartographer.Arg) int {
	if a.Number != nil {
		return *a.Number
	}
	return 0
}

// turnTable replays the log turn by turn. Decks change on every player's
// events, including the other player's turns (attacks, start-of-turn gains).
func turnTable(c *cartographer.Client, g *cartographer.Game) []turnRow {
	decks := cartographer.Decks{}
	tokens := map[string]int{}
	var rows []turnRow
	var cur *turnRow
	finish := func() {
		if cur == nil {
			return
		}
		d := decks[cur.Player]
		cur.Deck = deckSize(d)
		cur.VP, _ = deckVP(c, d)
		cur.VP += tokens[cur.Player]
		rows = append(rows, *cur)
		cur = nil
	}
	for _, e := range g.Events {
		if e.Template == "NEW_TURN" {
			finish()
			cur = &turnRow{Turn: e.Turn, Player: e.Player, Extra: e.Extra, Bought: []string{}, Gained: []string{}, Trashed: []string{}}
			continue
		}
		decks.Apply(e)
		if n, ok := vpArg[e.Template]; ok {
			tokens[e.Player] += number(e.Args[n])
		}
		if cur == nil || e.Player != cur.Player {
			continue
		}
		cards := e.Args[1].Cards
		if n, ok := coinArg[e.Template]; ok {
			cur.Coins += number(e.Args[n])
		}
		switch {
		case e.Template == "PLUS_COIN_FROM":
			cur.Coins++
		case e.Template == "LOSES_COIN" || e.Template == "LOSES_COINS":
			cur.Coins -= number(e.Args[1])
		case boughtT[e.Template]:
			cur.Bought = addCards(cur.Bought, cards)
		case gainedT[e.Template]:
			cur.Gained = addCards(cur.Gained, cards)
		case trashedT[e.Template]:
			cur.Trashed = addCards(cur.Trashed, cards)
		case drawnT[e.Template]:
			for _, cn := range cards {
				cur.Drawn += cn.N
			}
		case e.Template == "SHUFFLES":
			cur.Shuffles++
		case playedT[e.Template]:
			for _, cn := range cards {
				if cn.Card != nil && hasType(cn.Card, "Action") {
					cur.Actions += cn.N
				}
			}
		}
	}
	finish()
	return rows
}

func hasType(c *cartographer.Card, t string) bool {
	for _, x := range c.Types {
		if x == t {
			return true
		}
	}
	return false
}

// addCards adds cards to a "2× Horse, Groom" list, keeping first-seen order.
func addCards(list []string, cs []cartographer.CardN) []string {
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
		name := "card"
		if c.Card != nil {
			name = c.Card.Name
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
	rows := turnTable(c, g)
	decks := g.FinalDecks()
	fmt.Fprintf(&w, "# %s\n\n", md(strings.Join(g.Players, " vs ")))
	if len(g.Header) > 0 {
		w.WriteString(md(strings.Join(g.Header, " · ")) + "\n\n")
	}
	w.WriteString("## Final decks\n\n")
	for _, p := range g.Players {
		vp, unscored := deckVP(c, decks[p])
		fmt.Fprintf(&w, "- **%s** (%d VP from cards", md(p), vp)
		if len(unscored) > 0 {
			fmt.Fprintf(&w, " plus unscored %s", md(strings.Join(unscored, ", ")))
		}
		fmt.Fprintf(&w, ", %d cards): %s\n", deckSize(decks[p]), md(deckLine(decks[p])))
	}
	w.WriteString("\n## Turns\n\n| Turn | Player | $ | Bought | Also gained | Trashed | Actions | Drawn | Deck | VP |\n|--:|---|--:|---|---|---|--:|--:|--:|--:|\n")
	for _, r := range rows {
		turn := fmt.Sprint(r.Turn)
		if r.Extra != "" {
			turn += " (" + r.Extra + ")"
		}
		fmt.Fprintf(&w, "| %s | %s | %d | %s | %s | %s | %d | %d | %d | %d |\n",
			md(turn), md(r.Player), r.Coins, md(strings.Join(r.Bought, ", ")), md(strings.Join(r.Gained, ", ")),
			md(strings.Join(r.Trashed, ", ")), r.Actions, r.Drawn, r.Deck, r.VP)
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
