package cartographer

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LogHeader starts every log copied by scry ("scrying-pool log v1"): one log line
// per line, "<indent level><T|S>\t<the line's spans joined by |>".
const LogHeader = "# scrying-pool log v1"

// Arg is one template argument, read as a player, a card list or a number when it is one.
type Arg struct {
	Text   string
	Player string  `json:",omitempty"`
	Cards  []CardN `json:",omitempty"`
	Number *int    `json:",omitempty"`
}

// Event is one log line read through a client template.
type Event struct {
	Line     int    // line number in the copied log
	Level    int    // nesting under the card that caused it
	Turn     int    // the turn owner's turn number, 0 before turn 1
	TurnOf   string // whose turn it is
	Extra    string // for NEW_TURN: what gave an extra turn, e.g. "Voyage"
	Template string // the client's template name, e.g. "BUY_AND_GAIN"
	Cause    string // the card of the nearest line one level up, e.g. "Invest" for an exile it caused
	CauseOf  string // that line's template, e.g. "TRASH" when a trashed Lich discards itself
	Player   string // Argument0 when it is a player
	Args     map[int]Arg
}

// Unknown is a line no template matches. A complete parser has none.
type Unknown struct {
	Line int
	Text string
}

type Game struct {
	Header   []string          // the lines before the game starts, e.g. "Game #184519122, unrated."
	Players  []string          // in turn order
	Initials map[string]string // how log lines show each player, e.g. "g" for "gamrevinu"
	Events   []Event
	Unknown  []Unknown
}

// "Turn 11 - Alice [Voyage]" is an extra turn; extra turns share the turn numbering.
var reTurn = regexp.MustCompile(`^Turn (\d+) - (.+?)(?: \[([^\]]+)\])?$`)

// Parse reads a copied log. names renames players, e.g. the bot that took a seat
// ("Lord Rattington") back to the person who played it.
func (c *Client) Parse(text string, names map[string]string) (*Game, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if strings.TrimSpace(lines[0]) != LogHeader {
		return nil, errors.New("not a Scrying Pool game log: the first line should be " + LogHeader)
	}
	type row struct {
		n, level int
		kind     string
		pieces   []string
	}
	var rows []row
	for i, l := range lines[1:] {
		head, body, ok := strings.Cut(l, "\t")
		if !ok || body == "" {
			continue
		}
		kind := strings.TrimLeft(head, "0123456789")
		level, _ := strconv.Atoi(strings.TrimSuffix(head, kind))
		rows = append(rows, row{i + 2, level, kind, strings.Split(body, "|")})
	}

	rename := func(n string) string {
		if to, ok := names[n]; ok {
			return to
		}
		return n
	}
	g := &Game{}
	addPlayer := func(name string) {
		for _, p := range g.Players {
			if p == name {
				return
			}
		}
		g.Players = append(g.Players, name)
	}
	// Each player is shown by an initial; the line after "Turn N - name" starts with it.
	initials := map[string]string{}
	for i, r := range rows {
		if r.kind != "T" {
			continue
		}
		m := reTurn.FindStringSubmatch(r.pieces[0])
		if m == nil {
			continue
		}
		name := rename(m[2])
		addPlayer(name)
		if i+1 < len(rows) && rows[i+1].kind != "T" && len(rows[i+1].pieces) > 1 && len(rows[i+1].pieces[0]) <= 2 {
			initials[rows[i+1].pieces[0]] = name
		}
		if _, ok := initials[m[2][:1]]; !ok {
			initials[m[2][:1]] = name
		}
	}
	// players who never had a turn (resigned at once) only appear in "starts with" lines
	for _, r := range rows {
		if len(r.pieces) > 1 && r.pieces[1] == "starts with" {
			if _, ok := initials[r.pieces[0]]; !ok {
				initials[r.pieces[0]] = r.pieces[0]
				addPlayer(r.pieces[0])
			}
		}
	}

	g.Initials = initials

	turn, turnOf, started := 0, "", false
	type parent struct{ card, template string }
	var stack []parent // line at each nesting level, for Cause
	for _, r := range rows {
		text := strings.Join(r.pieces, " ")
		if r.kind == "T" && text == "Between Turns" { // effects between turns, e.g. Mission
			g.Events = append(g.Events, Event{Line: r.n, Turn: turn, Template: "BETWEEN_TURNS"})
			stack = stack[:0]
			continue
		}
		if r.kind == "T" {
			if m := reTurn.FindStringSubmatch(text); m != nil {
				turn, _ = strconv.Atoi(m[1])
				turnOf, started = rename(m[2]), true
				g.Events = append(g.Events, Event{Line: r.n, Turn: turn, TurnOf: turnOf, Player: turnOf, Extra: m[3], Template: "NEW_TURN"})
				stack = stack[:0]
				continue
			}
		}
		t, args := c.matchLine(text, initials)
		if t == nil {
			if !started && len(g.Events) == 0 {
				g.Header = append(g.Header, text)
			} else {
				g.Unknown = append(g.Unknown, Unknown{r.n, text})
			}
			continue
		}
		e := Event{Line: r.n, Level: r.level, Turn: turn, TurnOf: turnOf, Template: t.Name, Args: map[int]Arg{}}
		for n, s := range args {
			a := Arg{Text: s}
			if p, ok := initials[s]; ok {
				a.Player = p
			} else if v, err := strconv.Atoi(s); err == nil {
				a.Number = &v
			} else if cs, ok := c.CardList(s); ok {
				a.Cards = cs
			}
			e.Args[n] = a
		}
		e.Player = e.Args[0].Player
		// the nearest line above with a card; levels can skip ("plays a Coven. (Royal Galley)" at 0, its effects at 2)
		for i := min(r.level, len(stack)) - 1; i >= 0; i-- {
			if stack[i].card != "" {
				e.Cause, e.CauseOf = stack[i].card, stack[i].template
				break
			}
		}
		card := ""
		if cs := e.Args[1].Cards; len(cs) > 0 && cs[0].Card != nil {
			card = cs[0].Card.Name
		}
		if e.Template == "PLAY_USING_WAY" { // the Way's text applies, e.g. Way of the Camel exiles a Gold; Way of the Chameleon follows the card's
			if cs := e.Args[2].Cards; len(cs) > 0 && cs[0].Card != nil && cs[0].Card.Name != "Way of the Chameleon" {
				card = cs[0].Card.Name
			}
		}
		if r.level < len(stack) {
			stack = stack[:r.level]
		}
		for len(stack) < r.level {
			stack = append(stack, parent{})
		}
		stack = append(stack, parent{card, e.Template})
		g.Events = append(g.Events, e)
	}
	revealHiddenGains(g)
	return g, nil
}

// revealHiddenGains names cards the log hides from this seat when the line just
// before revealed them: Pickaxe logs "L reveals a Sextant" then "L gains a card".
func revealHiddenGains(g *Game) {
	for i := 1; i < len(g.Events); i++ {
		e, prev := &g.Events[i], g.Events[i-1]
		cs := e.Args[1].Cards
		if !gains[e.Template] || len(cs) != 1 || cs[0].Card != nil || prev.Template != "REVEAL" || prev.Player != e.Player {
			continue
		}
		if rc := prev.Args[1].Cards; len(rc) == 1 && rc[0].Card != nil && rc[0].N == cs[0].N {
			a := e.Args[1]
			a.Cards = []CardN{rc[0]}
			e.Args[1] = a
		}
	}
}

// matchLine finds the most specific template for a line. A template that starts
// with Argument0 must have a player or a card there, so "Approaching Army comes
// true" and "g gains a Silver" can't be mixed up. Only a line no template reads
// is tried again with empty arguments: the client prints an unnamed source as
// nothing, as in "g fails to take a Turn (from )." after a Lich.
func (c *Client) matchLine(text string, initials map[string]string) (*Template, map[int]string) {
	for _, empty := range []bool{false, true} {
		for _, t := range c.Templates {
			if t.generic() {
				continue
			}
			args := t.match(text, empty)
			if args == nil {
				continue
			}
			if strings.HasPrefix(t.English, "Argument0 ") {
				a0 := args[0]
				if _, ok := initials[a0]; !ok {
					if _, ok := c.CardList(a0); !ok {
						continue
					}
				}
			}
			return t, args
		}
	}
	return nil, nil
}

// String renders an event for debugging, e.g. "12 gamrevinu BUY_AND_GAIN [Hostelry]".
func (e Event) String() string {
	var parts []string
	for n := 1; n <= 3; n++ {
		if a, ok := e.Args[n]; ok {
			if len(a.Cards) > 0 {
				var cs []string
				for _, c := range a.Cards {
					cs = append(cs, c.String())
				}
				parts = append(parts, "["+strings.Join(cs, ", ")+"]")
			} else {
				parts = append(parts, a.Text)
			}
		}
	}
	return fmt.Sprintf("%d %s %s %s", e.Line, e.Player, e.Template, strings.Join(parts, " "))
}
