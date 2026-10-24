package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

// Log arguments: a seat shows as the player's initial, a card list as
// "2 Coppers and an Estate", a name as the bare card name ("(Merchant)").
type (
	seat int
	name CardID
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
	tr      *Entry  // Treasures being played: consecutive plays make one line
	after   []Entry // the lines those plays caused, written after it (Merchant's +$)
}

func NewLog(t *Table) *Log { return &Log{T: t} }

func (l *Log) add(e Entry) {
	if l == nil {
		return
	}
	if l.tr != nil {
		if e.Level > 0 && e.Kind == 0 {
			l.after = append(l.after, e)
			return
		}
		l.flush()
	}
	l.Entries = append(l.Entries, e)
}

func (l *Log) treasure(p int, c CardID, coins int) {
	if l == nil {
		return
	}
	if l.tr != nil && l.tr.Args[0] != seat(p) {
		l.flush()
	}
	if l.tr == nil {
		l.tr = &Entry{Template: "PLAY_TREASURES_FOR", Args: []any{seat(p), []CardID{}, 0}}
	}
	l.tr.Args[1] = append(l.tr.Args[1].([]CardID), c)
	l.tr.Args[2] = l.tr.Args[2].(int) + coins
}

func (l *Log) flush() {
	if l.tr == nil {
		return
	}
	l.Entries = append(l.Entries, *l.tr)
	l.Entries = append(l.Entries, l.after...)
	l.tr, l.after = nil, nil
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
			if s = strings.TrimSpace(s); s != "" {
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
	case seat:
		return players[a][:1], nil
	case int:
		return strconv.Itoa(a), nil
	case name:
		return l.T.Defs[a].Name, nil
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
			card := c.Cards[l.T.Defs[id].Name]
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
