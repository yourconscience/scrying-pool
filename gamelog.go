package main

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Game logs copied from dominion.games (see web/bookmarklet.js), in the
// "scrying-pool log v1" format: one log line per line,
// "<indent level><T|S>\t<pieces joined by |>", where T marks a new turn and
// S a shuffle. Pieces are the client's own spans, so a card is one piece
// ("2 Coppers", "a Shaman", "Sinister Plot").

const logHeader = "# scrying-pool log v1"

type cardN struct {
	Card string `json:"card"` // "" when the client hides which cards ("5 cards")
	N    int    `json:"n"`
}

type logEvent struct {
	Line   int     `json:"line"`
	Turn   int     `json:"turn"`    // the turn owner's turn number, 0 before turn 1
	TurnOf string  `json:"turn_of"` // whose turn it is
	Player string  `json:"player"`  // who does it
	Level  int     `json:"level"`   // nesting under the card that caused it
	Verb   string  `json:"verb"`
	Cards  []cardN `json:"cards,omitempty"`
	Coins  int     `json:"coins,omitempty"` // $ from "(+$N)" or "gets +$N"
	Gets   int     `json:"gets,omitempty"`  // N in "gets +N Actions"
	What   string  `json:"what,omitempty"`  // Action, Buy, ...
	Cause  string  `json:"cause,omitempty"` // card in "(Shaman)"; for "turn", what gave an extra turn ("Voyage")
}

type unknownLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type Game struct {
	Header  []string      `json:"header"`  // lines before the first player action, e.g. "Game #184519122, unrated."
	Players []string      `json:"players"` // in turn order
	Events  []logEvent    `json:"events"`
	Unknown []unknownLine `json:"unknown"`
}

// verbs maps the client's wording to our event names.
var verbs = map[string]string{
	"starts with":          "start",
	"shuffles their deck.": "shuffle",
	"starts their turn.":   "turn",
	"draws":                "draw",
	"plays":                "play",
	"buys and gains":       "buy-gain",
	"buys":                 "buy",
	"gains":                "gain",
	"trashes":              "trash",
	"returns":              "return",
	"discards":             "discard",
	"discards and reveals": "discard",
	"reveals their hand:":  "reveal",
	"gets +":               "get",
	"gets +$":              "coins",
}

// "Turn 11 - Alice [Voyage]" is an extra turn; extra turns share the turn numbering.
var reTurnLine = regexp.MustCompile(`^Turn (\d+) - (.+?)(?: \[([^\]]+)\])?$`)

// parseGameLog reads a copied log. names renames players, e.g. the bot that
// took a seat ("Lord Rattington") back to the person who played it.
func parseGameLog(text string, names map[string]string) (*Game, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != logHeader {
		return nil, errors.New(`not a Scrying Pool game log: the first line should be "` + logHeader + `" (use the Copy log bookmark)`)
	}
	type raw struct {
		n      int
		level  int
		kind   string
		pieces []string
	}
	var rows []raw
	for i, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		head, body, ok := strings.Cut(l, "\t")
		if !ok {
			return nil, fmt.Errorf("line %d: missing tab", i+2)
		}
		kind := strings.TrimLeft(head, "0123456789")
		level, _ := strconv.Atoi(strings.TrimSuffix(head, kind))
		var pieces []string
		if body != "" {
			pieces = strings.Split(body, "|")
		}
		rows = append(rows, raw{i + 2, level, kind, pieces})
	}

	// Who is who: each "Turn N - name" line is usually followed by
	// "<initial>|…"; the initial is the name's first letter. Players who never
	// got a turn (resigned at once) only appear in "starts with" lines.
	initials := map[string]string{}
	g := &Game{}
	addPlayer := func(name string) {
		if !contains(g.Players, name) {
			g.Players = append(g.Players, name)
		}
	}
	for i, r := range rows {
		if r.kind != "T" || len(r.pieces) == 0 {
			continue
		}
		m := reTurnLine.FindStringSubmatch(r.pieces[0])
		if m == nil {
			continue
		}
		name := rename(m[2], names)
		addPlayer(name)
		if i+1 < len(rows) && rows[i+1].kind != "T" && len(rows[i+1].pieces) > 1 && len(rows[i+1].pieces[0]) <= 2 {
			initials[rows[i+1].pieces[0]] = name
		}
		if _, ok := initials[m[2][:1]]; !ok {
			initials[m[2][:1]] = name
		}
	}
	for _, r := range rows {
		if len(r.pieces) > 1 && r.pieces[1] == "starts with" {
			if _, ok := initials[r.pieces[0]]; !ok {
				initials[r.pieces[0]] = r.pieces[0]
				addPlayer(r.pieces[0])
			}
		}
	}
	if len(g.Players) == 0 {
		return nil, errors.New("no players found in the log")
	}

	turn, turnOf := 0, ""
	for _, r := range rows {
		p := r.pieces
		if r.kind == "T" && len(p) > 0 {
			if m := reTurnLine.FindStringSubmatch(p[0]); m != nil {
				turn, _ = strconv.Atoi(m[1])
				turnOf = rename(m[2], names)
				g.Events = append(g.Events, logEvent{Line: r.n, Turn: turn, TurnOf: turnOf, Player: turnOf, Verb: "turn", Cause: m[3]})
				continue
			}
		}
		player, isPlayer := "", false
		if len(p) > 0 {
			player, isPlayer = initials[p[0]]
		}
		if !isPlayer || len(p) < 2 {
			if turn == 0 && len(g.Events) == 0 {
				if len(p) > 0 {
					g.Header = append(g.Header, strings.Join(p, " "))
				}
			} else if len(p) > 0 {
				g.Unknown = append(g.Unknown, unknownLine{r.n, strings.Join(p, " ")})
			}
			continue
		}
		verb, ok := verbs[p[1]]
		if !ok {
			g.Unknown = append(g.Unknown, unknownLine{r.n, strings.Join(p, " ")})
			continue
		}
		e := logEvent{Line: r.n, Turn: turn, TurnOf: turnOf, Player: player, Level: r.level, Verb: verb}
		if err := e.readArgs(p[2:]); err != nil {
			g.Unknown = append(g.Unknown, unknownLine{r.n, strings.Join(p, " ") + " (" + err.Error() + ")"})
			continue
		}
		g.Events = append(g.Events, e)
	}
	return g, nil
}

// readArgs reads the pieces after the verb: card lists joined by "," and "and",
// "(+$N)" coin amounts, "(Card)" causes, "+N Action." amounts, "to the Horse pile".
func (e *logEvent) readArgs(p []string) error {
	for i := 0; i < len(p); i++ {
		s := p[i]
		switch {
		case s == "," || s == "and" || s == "." || s == ")" || s == "to":
			continue
		case strings.HasSuffix(s, "(+$"): // ". (+$" then N then ")"
			if i+1 < len(p) {
				e.Coins, _ = strconv.Atoi(p[i+1])
				i++
			}
		case s == ". (" || s == "(":
			if i+1 < len(p) {
				e.Cause = p[i+1]
				i++
			}
		case e.Verb == "get" && i == 0:
			n, err := strconv.Atoi(s)
			if err != nil || i+1 >= len(p) {
				return errors.New("unreadable amount")
			}
			e.Gets, e.What = n, strings.TrimSuffix(strings.TrimSuffix(p[i+1], "."), "s")
			i++
		case e.Verb == "coins" && i == 0:
			e.Coins, _ = strconv.Atoi(s)
		case strings.HasSuffix(s, " pile"):
			continue // "the Horse pile": where a returned card goes
		default:
			c, ok := readCardPiece(s)
			if !ok {
				return fmt.Errorf("unknown card %q", s)
			}
			e.Cards = append(e.Cards, c)
		}
	}
	return nil
}

var reCount = regexp.MustCompile(`^(a|an|the|\d+)\s+(.+)$`)

// readCardPiece reads "2 Coppers", "an Estate", "Sinister Plot" or "5 cards".
func readCardPiece(s string) (cardN, bool) {
	n, name := 1, s
	if m := reCount.FindStringSubmatch(s); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			n = v
		}
		name = m[2]
	}
	switch name {
	case "card", "cards", "other card", "other cards":
		return cardN{"", n}, true
	}
	if c, ok := cardByLogName(name); ok {
		return cardN{c, n}, true
	}
	return cardN{}, false
}

var pluralIndex = func() map[string]string {
	m := map[string]string{}
	for _, c := range cardIndex {
		for _, form := range pluralForms(c.Name) {
			if _, taken := m[cardKey(form)]; !taken {
				m[cardKey(form)] = c.Name
			}
		}
	}
	return m
}()

// pluralForms lists the names the client may use for a card: "Shaman",
// "Shamans", "Hostelries", "Fortresses", "King's Courts".
func pluralForms(name string) []string {
	forms := []string{name, name + "s"}
	switch {
	case strings.HasSuffix(name, "y") && !strings.HasSuffix(name, "ey"):
		forms = append(forms, strings.TrimSuffix(name, "y")+"ies")
	case strings.HasSuffix(name, "s"), strings.HasSuffix(name, "x"), strings.HasSuffix(name, "ch"), strings.HasSuffix(name, "sh"):
		forms = append(forms, name+"es")
	}
	return forms
}

func cardByLogName(name string) (string, bool) {
	if c, ok := cardIndex[cardKey(name)]; ok {
		return c.Name, true
	}
	c, ok := pluralIndex[cardKey(name)]
	return c, ok
}

func rename(name string, names map[string]string) string {
	if to, ok := names[name]; ok {
		return to
	}
	return name
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Deck tracking.

// deckChange says how an event changes the owner's deck: +1 adds its cards, -1 removes them.
func deckChange(verb string) int {
	switch verb {
	case "start", "gain", "buy-gain":
		return 1
	case "trash", "return":
		return -1
	}
	return 0
}

// finalDecks replays every gain, trash and return. Cards the client hides
// ("trashes 2 cards") can't be tracked; they are counted in the "" entry.
func (g *Game) finalDecks() map[string]map[string]int {
	decks := map[string]map[string]int{}
	for _, p := range g.Players {
		decks[p] = map[string]int{}
	}
	for _, e := range g.Events {
		d := deckChange(e.Verb)
		if d == 0 {
			continue
		}
		for _, c := range e.Cards {
			if decks[e.Player] == nil {
				decks[e.Player] = map[string]int{}
			}
			decks[e.Player][c.Card] += d * c.N
			if decks[e.Player][c.Card] == 0 {
				delete(decks[e.Player], c.Card)
			}
		}
	}
	return decks
}

// basicVP covers the cards whose VP is fixed. Others (Gardens, Duke, ...) need
// game state and are reported as unscored for now.
var basicVP = map[string]int{"Estate": 1, "Duchy": 3, "Province": 6, "Colony": 10, "Curse": -1}

func deckVP(deck map[string]int) (vp int, unscored []string) {
	for card, n := range deck {
		if v, ok := basicVP[card]; ok {
			vp += v * n
			continue
		}
		if c, ok := cardIndex[cardKey(card)]; ok && hasType(c, "Victory") {
			unscored = append(unscored, card)
		}
	}
	sort.Strings(unscored)
	return vp, unscored
}

func hasType(c Card, t string) bool {
	for _, x := range c.Types {
		if x == t {
			return true
		}
	}
	return false
}
