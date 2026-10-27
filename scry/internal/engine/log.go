package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

// Log arguments besides []CardID (a card list, "2 Coppers and an Estate") and
// int (a number): a Seat shows as the player's initial, a Name as the bare card
// name ("(Merchant)").
type (
	Seat     int
	Name     CardID
	PileName CardID // "the Horse pile"
)

// Entry is one log line: a dominion.games template and its arguments.
type Entry struct {
	Level    int
	Kind     byte   // 0, 'T' for a turn line, 'S' for a shuffle
	Template string // the client's template name; "" for plain text
	Text     string // a turn line or plain text
	Args     []any  // seat, []CardID, int or name, as Argument0, 1, ...
}

// Log records a game as dominion.games would show it with every card named.
type Log struct {
	T       *Table
	Entries []Entry
	// Hook runs when entry i is added, or changed by merging a line into it.
	Hook    func(i int)
	tr      *Entry  // Treasures being played: consecutive plays make one line
	after   []Entry // the lines those plays caused, written after it (Merchant's +$)
	pending *Entry  // a lazy line
	// handTrash: the TRASH line being added is of cards from the hand; lastHandTrash
	// is the entry such a line went to, or -1
	handTrash     bool
	lastHandTrash int
	byNorm        map[string]*cartographer.Card
}

func NewLog(t *Table) *Log { return &Log{T: t, lastHandTrash: -1} }

// addAside adds a line that doesn't bring out the pending one: the other
// player's cards leaving play in Clean-up don't begin it.
func (l *Log) addAside(e Entry) {
	if l == nil {
		return
	}
	p := l.pending
	l.pending = nil
	l.add(e)
	l.pending = p
}

func (l *Log) add(e Entry) {
	if l == nil {
		return
	}
	if l.pending != nil && e.Kind != 'T' {
		p := *l.pending
		l.pending = nil
		l.add(p)
	}
	if l.tr != nil {
		if e.Level > l.tr.Level && e.Kind == 0 { // its own lines; one at its level ends it: Storyteller's "draws a card."
			l.after = append(l.after, e)
			return
		}
		l.flush()
	}
	// drawing or trashing from the hand again with nothing between: one line, Blacksmith's
	// two choices under Elder, Remake's "trashes 2 Coppers." when the first gains nothing
	// (but Monastery's from the hand, then from play, are two)
	merge := e.Template == "DRAW" || e.Template == "TRASH" && l.handTrash && l.lastHandTrash == len(l.Entries)-1
	if n := len(l.Entries); n > 0 && merge && len(e.Args) == 2 {
		last := &l.Entries[n-1]
		if last.Template == e.Template && last.Level == e.Level && len(last.Args) == 2 && last.Args[0] == e.Args[0] {
			last.Args[1] = append(last.Args[1].([]CardID), e.Args[1].([]CardID)...)
			l.changed(n - 1)
			return
		}
	}
	// buying or gaining the same card again: "buys and gains 2 Silvers", Ball's "gains 2 Poachers"
	if n := len(l.Entries); n > 0 && (e.Template == "BUY_AND_GAIN" || e.Template == "GAIN" && len(e.Args) == 2) {
		last := &l.Entries[n-1]
		if last.Template == e.Template && last.Level == e.Level && last.Args[0] == e.Args[0] && last.Args[1].([]CardID)[0] == e.Args[1].([]CardID)[0] {
			last.Args[1] = append(last.Args[1].([]CardID), e.Args[1].([]CardID)...)
			l.changed(n - 1)
			return
		}
	}
	l.Entries = append(l.Entries, e)
	if e.Template == "TRASH" && l.handTrash {
		l.lastHandTrash = len(l.Entries) - 1
	}
	l.changed(len(l.Entries) - 1)
}

// last is the index of the last line, -1 for none.
func (l *Log) last() int {
	if l == nil {
		return -1
	}
	return len(l.Entries) - 1
}

// size counts the lines written or waiting to be.
func (l *Log) size() int {
	if l == nil {
		return 0
	}
	n := len(l.Entries) + len(l.after)
	if l.pending != nil {
		n++
	}
	return n
}

// extendLast adds a card to the last line's card list.
func (l *Log) extendLast(c CardID) {
	if l == nil {
		return
	}
	if n := len(l.after); n > 0 {
		l.after[n-1].Args[1] = append(l.after[n-1].Args[1].([]CardID), c)
		return
	}
	n := len(l.Entries) - 1
	l.Entries[n].Args[1] = append(l.Entries[n].Args[1].([]CardID), c)
	l.changed(n)
}

// unGain makes a "buys and gains" line a plain "buys" when the card wasn't gained.
func (l *Log) unGain(i int) {
	if l == nil || i < 0 {
		return
	}
	if e := &l.Entries[i]; e.Template == "BUY_AND_GAIN" && len(e.Args[1].([]CardID)) == 1 {
		e.Template = "BUY"
		l.changed(i)
	}
}

func (l *Log) changed(i int) {
	if l.Hook != nil {
		l.Hook(i)
	}
}

// treasure adds a Treasure play to the current line. Treasures that make no $
// have one of their own, without it: "plays a Potion.", "plays an Abundance and a Ducat."
func (l *Log) treasure(p int, c CardID, coins, level int, plain bool) {
	if l == nil {
		return
	}
	if l.tr != nil && (l.tr.Args[0] != Seat(p) || l.tr.Level != level || (l.tr.Template == "PLAY") != plain) {
		l.flush()
	}
	if l.tr == nil {
		l.tr = &Entry{Level: level, Template: "PLAY_TREASURES_FOR", Args: []any{Seat(p), []CardID{}, 0}}
		if plain {
			l.tr = &Entry{Level: level, Template: "PLAY", Args: []any{Seat(p), []CardID{}}}
		}
	}
	l.tr.Args[1] = append(l.tr.Args[1].([]CardID), c)
	if !plain {
		l.tr.Args[2] = l.tr.Args[2].(int) + coins
	}
}

func (l *Log) flush() {
	if l == nil || l.tr == nil {
		return
	}
	tr, after := *l.tr, l.after
	l.tr, l.after = nil, nil
	l.Entries = append(l.Entries, tr)
	l.changed(len(l.Entries) - 1)
	for _, e := range after {
		l.Entries = append(l.Entries, e)
		l.changed(len(l.Entries) - 1)
	}
}

var reArg = regexp.MustCompile(`Argument(\d+)`)

// Render writes the log in Scrying Pool's copied-log format, which Cartographer
// reads. The English templates and card nouns come from the client tables.
func (l *Log) Render(c *cartographer.Client, players [2]string) (string, error) {
	l.flush()
	english := map[string]string{}
	for _, t := range c.Templates {
		english[t.Name] = t.English
	}
	var b strings.Builder
	b.WriteString(cartographer.LogHeader + "\n")
	for _, e := range l.Entries {
		b.WriteString(strconv.Itoa(e.Level))
		if e.Kind != 0 {
			b.WriteByte(e.Kind)
		}
		b.WriteByte('\t')
		if e.Template == "" {
			b.WriteString(e.Text + "\n")
			continue
		}
		tmpl, ok := english[e.Template]
		if !ok {
			return "", fmt.Errorf("the client has no template %s", e.Template)
		}
		var pieces []string
		addText := func(s string) {
			if s = strings.TrimSpace(c.ShowCards(s)); s != "" {
				pieces = append(pieces, s)
			}
		}
		last := 0
		for _, m := range reArg.FindAllStringSubmatchIndex(tmpl, -1) {
			addText(tmpl[last:m[0]])
			last = m[1]
			n, _ := strconv.Atoi(tmpl[m[2]:m[3]])
			if n >= len(e.Args) {
				return "", fmt.Errorf("%s has no Argument%d", e.Template, n)
			}
			s, err := l.arg(c, players, e.Args[n])
			if err != nil {
				return "", err
			}
			addText(s)
		}
		addText(tmpl[last:])
		b.WriteString(strings.Join(pieces, "|") + "\n")
	}
	return b.String(), nil
}

func (l *Log) arg(c *cartographer.Client, players [2]string, a any) (string, error) {
	switch a := a.(type) {
	case Seat:
		return players[a][:1], nil
	case int:
		return strconv.Itoa(a), nil
	case string: // a mat: "Island mat"
		return a, nil
	case Name:
		if a == 0 {
			return "", nil
		}
		return l.clientName(c, CardID(a)), nil
	case PileName:
		return "the " + l.clientName(c, l.T.PileCard(CardID(a))) + " pile", nil
	case []CardID:
		var order []CardID
		count := map[CardID]int{}
		for _, id := range a {
			if count[id] == 0 {
				order = append(order, id)
			}
			count[id]++
		}
		var parts []string
		for _, id := range order {
			card := l.clientCard(c, id)
			if card == nil {
				return "", fmt.Errorf("the client has no card %s", l.T.Defs[id].Name)
			}
			if n := count[id]; n == 1 {
				parts = append(parts, card.Noun)
			} else {
				parts = append(parts, fmt.Sprintf("%d %s", n, card.Plural))
			}
		}
		switch len(parts) {
		case 0:
			return "nothing", nil // "discards nothing."
		case 1:
			return parts[0], nil
		case 2:
			return parts[0] + " and " + parts[1], nil
		}
		return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1], nil
	}
	return "", fmt.Errorf("log argument %v of type %T", a, a)
}

// clientCard is the client's card for an engine card, by name ignoring case and punctuation.
func (l *Log) clientCard(c *cartographer.Client, id CardID) *cartographer.Card {
	name := l.T.Defs[id].Name
	if card := c.Cards[name]; card != nil {
		return card
	}
	if l.byNorm == nil {
		l.byNorm = map[string]*cartographer.Card{}
		for _, card := range c.Cards {
			l.byNorm[normName(card.Name)] = card
		}
	}
	return l.byNorm[normName(name)]
}

func (l *Log) clientName(c *cartographer.Client, id CardID) string {
	if card := l.clientCard(c, id); card != nil {
		return card.Name
	}
	return l.T.Defs[id].Name
}

func normName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// lazy holds a line that is written only if something else is logged before
// the next lazy call: "starts their turn" when a start-of-turn effect happens.
func (l *Log) lazy(e *Entry) {
	if l != nil {
		l.pending = e
	}
}

// Pending is the template of the lazy line not yet written, if any.
func (l *Log) Pending() string {
	if l == nil || l.pending == nil {
		return ""
	}
	return l.pending.Template
}

// emit writes the lazy line now, e.g. "starts their turn" for an Outpost that does nothing then.
func (l *Log) emit() {
	if l != nil && l.pending != nil {
		p := *l.pending
		l.pending = nil
		l.add(p)
	}
}
