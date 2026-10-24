// Package replay plays a saved dominion.games game again on the rules engine,
// making the choices its log shows and drawing the cards it names, and reports
// the first line where the engine's log differs from the real one. It is the
// engine's conformance check against real games (docs/simulator.md).
package replay

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Report says how far the engine followed a real game.
type Report struct {
	Turns   int    // turns in the real log
	Matched int    // turns the engine wrote line for line
	Done    bool   // the engine followed the whole log
	Missing string // the card without rules that stopped the replay
	Stop    string // why it stopped, when it did
	Turn    string // the turn it stopped in, e.g. "Turn 7 - gamrevinu"
	Real    string // the real line there
	Engine  string // the engine's line there
	Key     string // the real line's template and first card, to group stops by
}

// stop aborts a replay from inside the engine.
type stop struct{ why string }

// Check replays one saved game: its log and score screen.
func Check(c *cartographer.Client, t *engine.Table, logText, score string) Report {
	var rep Report
	rg, err := c.Parse(logText, nil)
	if err != nil {
		rep.Stop = err.Error()
		return rep
	}
	if len(rg.Players) != 2 {
		rep.Stop = fmt.Sprintf("%d players", len(rg.Players))
		return rep
	}
	r := &replayer{c: c, t: t, rep: &rep, line: map[int]int{}, english: map[string]string{}}
	for _, tm := range c.Templates {
		r.english[tm.Name] = tm.English
	}
	for _, e := range rg.Events {
		if skip[e.Template] {
			continue
		}
		if e.Template == "NEW_TURN" {
			rep.Turns++
		}
		r.line[e.Line] = len(r.real)
		r.real = append(r.real, e)
	}
	copy(r.names[:], rg.Players)
	kingdom, err := kingdomOf(t, score)
	if err != nil {
		rep.Stop = err.Error()
		return rep
	}
	var start [2][]engine.CardID
	for _, e := range rg.Events {
		if e.Template != "STARTS_WITH" {
			continue
		}
		p := r.seat(e.Player)
		for _, cn := range e.Args[1].Cards {
			d, ok := t.Card(cn.Card.Name)
			if !ok {
				rep.Stop = "no card " + cn.Card.Name
				return rep
			}
			for range cn.N {
				start[p] = append(start[p], d.ID)
			}
		}
	}
	takes := rg.DeckTakes()
	for p, n := range r.names {
		r.takes[p] = takes[n]
	}
	g, err := engine.New(t, engine.Setup{Kingdom: kingdom, Names: r.names, Agents: [2]engine.Agent{r, r}, Start: start, Partial: true})
	if err != nil {
		rep.Stop = err.Error()
		return rep
	}
	r.g = g
	g.Log = engine.NewLog(t)
	g.Log.Hook = r.hook
	g.BeforeTake = r.beforeTake
	r.run()
	return rep
}

func (r *replayer) run() {
	defer func() {
		x := recover()
		switch x := x.(type) {
		case nil:
			if r.pos < len(r.real) {
				r.fail("the engine ended the game", "")
			}
		case stop:
			if x.why == "" { // the real log ended
				r.rep.Done = true
				r.rep.Matched = r.rep.Turns
			} else {
				r.rep.Stop = x.why
			}
		case engine.MissingRules:
			r.rep.Missing = x.Card
			r.rep.Stop = x.Error()
		default:
			r.rep.Stop = fmt.Sprintf("engine: %v", x)
		}
	}()
	r.g.Play()
	r.checkLast()
}

type replayer struct {
	c       *cartographer.Client
	t       *engine.Table
	g       *engine.Game
	rep     *Report
	names   [2]string
	real    []cartographer.Event // the real log's events the engine writes too
	line    map[int]int          // real event index by log line
	english map[string]string    // template text by name: templates with the same text are the same line
	pos     int                  // the next real event the engine hasn't written
	matched int                  // the real event the last engine entry matched
	entry   int                  // that engine entry
	turn    string
	takes   [2][]cartographer.DeckTake
	took    [2]int
}

// skip lists real lines the engine doesn't write.
var skip = map[string]bool{"": true, "STARTS_WITH": false}

func (r *replayer) seat(name string) int {
	if name == r.names[1] {
		return 1
	}
	return 0
}

func (r *replayer) fail(why, engineLine string) {
	r.rep.Turn = r.turn
	if r.pos < len(r.real) {
		x := r.real[r.pos]
		r.rep.Real = x.String()
		r.rep.Key = x.Template
		if cs := x.Args[1].Cards; len(cs) > 0 && cs[0].Card != nil {
			r.rep.Key += " " + cs[0].Card.Name
		} else if x.Cause != "" {
			r.rep.Key += " (" + x.Cause + ")"
		}
	}
	r.rep.Engine = engineLine
	panic(stop{why})
}

// hook matches each engine log entry with the next real event as it is written.
func (r *replayer) hook(i int) {
	l := r.g.Log
	if i > 0 && i == r.entry && i < len(l.Entries) && r.matched >= 0 {
		return // a line merged into the last one; its cards are compared when the next line comes
	}
	r.checkLast()
	e := l.Entries[i]
	if e.Kind == 0 && e.Template == "" {
		return // the engine's header line
	}
	if r.pos >= len(r.real) {
		panic(stop{}) // the real game ended here
	}
	x := r.real[r.pos]
	if why := r.same(e, x, false); why != "" {
		r.fail(why, r.show(e))
	}
	if e.Kind == 'T' {
		r.turn = e.Text
		if x.Turn > 1 || x.Player != r.names[0] {
			r.rep.Matched++ // the previous turn matched to its end
		}
	}
	r.matched, r.entry = r.pos, i
	r.pos++
}

// checkLast compares the cards of the last matched pair once its lines can't grow.
func (r *replayer) checkLast() {
	if r.g == nil || r.g.Log == nil || r.matched < 0 || r.entry >= len(r.g.Log.Entries) || r.pos == 0 {
		return
	}
	e, x := r.g.Log.Entries[r.entry], r.real[r.matched]
	if why := r.same(e, x, true); why != "" {
		r.pos = r.matched
		r.fail(why, r.show(e))
	}
}

// same says how an engine entry differs from a real event: the line, and with
// cards also its arguments. Cards the real line doesn't name match any card.
func (r *replayer) same(e engine.Entry, x cartographer.Event, cards bool) string {
	switch e.Kind {
	case 'T':
		if x.Template != "NEW_TURN" || "Turn "+strconv.Itoa(x.Turn)+" - "+x.Player != e.Text {
			return "a different turn"
		}
		return ""
	case 'S':
		if x.Template != "SHUFFLES" || x.Player != r.names[e.Args[0].(engine.Seat)] {
			return "a shuffle the log doesn't have"
		}
		return ""
	}
	if e.Template != x.Template && r.english[e.Template] != r.english[x.Template] {
		return "a different line"
	}
	if e.Level != x.Level {
		return "a different nesting"
	}
	for n, a := range e.Args {
		y := x.Args[n]
		switch a := a.(type) {
		case engine.Seat:
			if y.Player != r.names[a] {
				return "a different player"
			}
		case int:
			if y.Number == nil || *y.Number != a {
				return "a different number"
			}
		case engine.Name:
			if a != 0 && norm(y.Text) != norm(r.t.Defs[a].Name) {
				return "a different card"
			}
		case engine.PileName:
			if norm(y.Text) != norm("the "+r.t.Defs[a].Name+" pile") {
				return "a different pile"
			}
		case []engine.CardID:
			if !cards {
				continue
			}
			names := make([]string, len(a))
			for i, id := range a {
				names[i] = r.t.Defs[id].Name
			}
			if why := sameCards(names, y.Cards); why != "" {
				return why
			}
		}
	}
	return ""
}

func sameCards(have []string, want []cartographer.CardN) string {
	n := map[string]int{}
	for _, c := range have {
		n[c]++
	}
	unknown := 0
	for _, cn := range want {
		if cn.Card == nil {
			unknown += cn.N
			continue
		}
		if n[cn.Card.Name] < cn.N {
			return "different cards"
		}
		n[cn.Card.Name] -= cn.N
	}
	left := 0
	for _, k := range n {
		left += k
	}
	if left != unknown {
		return "a different number of cards"
	}
	return ""
}

// show writes an engine entry the way Event.String writes a real one.
func (r *replayer) show(e engine.Entry) string {
	if e.Kind == 'T' {
		return e.Text
	}
	var parts []string
	for _, a := range e.Args {
		parts = append(parts, fmt.Sprint(a))
	}
	return fmt.Sprintf("%d %s %s", e.Level, e.Template, strings.Join(parts, " "))
}

// peek is the next real event the engine hasn't written.
func (r *replayer) peek() (cartographer.Event, bool) {
	if r.pos >= len(r.real) {
		return cartographer.Event{}, false
	}
	return r.real[r.pos], true
}

// lines a decision's answer writes, by the decision's kind
var answers = map[engine.Kind][]string{
	engine.PlayAction:    {"PLAY"},
	engine.PlayTreasures: {"PLAY_TREASURES_FOR"},
	engine.Buy:           {"BUY_AND_GAIN", "BUY"},
	engine.ToGain:        {"GAIN"},
	engine.ToTrash:       {"TRASH"},
	engine.ToDiscard:     {"DISCARD"},
	engine.ToTopdeck:     {"TOPDECK"},
	engine.ToPlay:        {"PLAY"},
	engine.ToSetAside:    {"SET_ASIDE_WITH"},
	engine.ToReveal:      {"REACTS_WITH"},
}

// Decide answers with what the next real line shows.
func (r *replayer) Decide(v *engine.View, d *engine.Decision) []int {
	if d.Kind == engine.ToOrder {
		all := make([]int, len(d.Options))
		for i := range all {
			all[i] = i
		}
		return all
	}
	if d.Kind == engine.Buy {
		if c, ok := r.moreOfLast(d); ok {
			return r.pick(d, []cartographer.CardN{c})
		}
	}
	x, ok := r.peek()
	if !ok {
		panic(stop{})
	}
	if !r.answers(d, x) {
		if d.Min == 0 {
			return nil
		}
		r.fail(fmt.Sprintf("the engine asks %s to %s, the log doesn't", r.names[d.Player], d.Kind), "")
	}
	return r.pick(d, x.Args[1].Cards)
}

// answers says whether a real event is what a decision's answer writes.
func (r *replayer) answers(d *engine.Decision, x cartographer.Event) bool {
	if x.Player != r.names[d.Player] || x.Level != d.Level {
		return false
	}
	for _, t := range answers[d.Kind] {
		if x.Template == t || r.english[x.Template] == r.english[t] {
			if d.Kind == engine.PlayAction {
				cs := x.Args[1].Cards
				return len(cs) == 1 && cs[0].Card != nil && cs[0].Card.Is("Action")
			}
			return true
		}
	}
	return false
}

// moreOfLast: the last real line bought more copies than the engine has yet ("buys and gains 2 Silvers").
func (r *replayer) moreOfLast(d *engine.Decision) (cartographer.CardN, bool) {
	l := r.g.Log
	if r.entry >= len(l.Entries) || r.matched < 0 || r.pos == 0 {
		return cartographer.CardN{}, false
	}
	e, x := l.Entries[r.entry], r.real[r.matched]
	if e.Template != "BUY_AND_GAIN" || x.Template != "BUY_AND_GAIN" || x.Player != r.names[d.Player] {
		return cartographer.CardN{}, false
	}
	cs := x.Args[1].Cards
	if len(cs) != 1 || cs[0].Card == nil || cs[0].N <= len(e.Args[1].([]engine.CardID)) {
		return cartographer.CardN{}, false
	}
	return cartographer.CardN{Card: cs[0].Card, N: 1}, true
}

// pick turns the cards a real line names into option indexes; cards it
// doesn't name are filled with options nothing later asks for.
func (r *replayer) pick(d *engine.Decision, cards []cartographer.CardN) []int {
	used := map[int]bool{}
	var out []int
	unknown := 0
	for _, cn := range cards {
		if cn.Card == nil {
			unknown += cn.N
			continue
		}
		for range cn.N {
			found := false
			for i, o := range d.Options {
				if !used[i] && r.t.Defs[o.Card].Name == cn.Card.Name {
					used[i], found = true, true
					out = append(out, i)
					break
				}
			}
			if !found {
				r.fail(fmt.Sprintf("the log has %s %s %s, which the engine doesn't offer", r.names[d.Player], d.Kind, cn.Card.Name), "")
			}
		}
	}
	for i := range d.Options {
		if unknown == 0 {
			break
		}
		if !used[i] {
			used[i] = true
			out = append(out, i)
			unknown--
		}
	}
	if len(out) > d.Max || len(out) < d.Min {
		r.fail(fmt.Sprintf("the log has %d cards for %s, the engine wants %d to %d", len(out), d.Kind, d.Min, d.Max), "")
	}
	return out
}

// beforeTake puts the card the log takes next from a deck on top of the engine's deck.
func (r *replayer) beforeTake(p int) {
	if r.took[p] >= len(r.takes[p]) {
		panic(stop{}) // the real log ends before this draw
	}
	take := r.takes[p][r.took[p]]
	r.took[p]++
	deck := r.g.P[p].Z[engine.Deck]
	if take.Card != "" {
		for i := len(deck) - 1; i >= 0; i-- {
			if r.g.Def(deck[i]).Name == take.Card {
				r.g.PutOnTop(deck[i])
				return
			}
		}
		r.fail(fmt.Sprintf("the log takes a %s from %s's deck, which has none", take.Card, r.names[p]), "")
	}
	// an unnamed card: one this player shows from their hand while it's there,
	// else one they won't need in their next hand either
	need := r.demand(p, take.Line, 1)
	for i := len(deck) - 1; i >= 0; i-- {
		if need[r.g.Def(deck[i]).Name] > 0 {
			r.g.PutOnTop(deck[i])
			return
		}
	}
	soon := r.demand(p, take.Line, 2)
	in := map[string]int{}
	for _, x := range deck {
		in[r.g.Def(x).Name]++
	}
	best, spare := -1, -1
	for i, x := range deck {
		if n := in[r.g.Def(x).Name] - soon[r.g.Def(x).Name]; n > spare {
			best, spare = i, n
		}
	}
	if best >= 0 {
		r.g.PutOnTop(deck[best])
	}
}

// demand counts the cards player p shows from their hand by name in the next
// hands while cards drawn at a line are still there (windows 1: that hand;
// 2: that hand and the next), less the named cards already in hand.
func (r *replayer) demand(p int, line, windows int) map[string]int {
	need := map[string]int{}
	i, ok := r.line[line]
	if !ok {
		return need
	}
	name := r.names[p]
	ev := r.real[i]
	inTurn := ev.TurnOf == name && !(ev.Template == "DRAW" && ev.Level == 0) // a Clean-up draw is for the next turn
	for _, x := range r.real[i:] {
		if x.Template == "NEW_TURN" {
			if x.Player == name {
				inTurn = true
			} else if inTurn {
				if windows--; windows == 0 {
					break
				}
				inTurn = false
			}
			continue
		}
		if x.Player != name || !fromHand[x.Template] {
			continue
		}
		for _, cn := range x.Args[1].Cards {
			if cn.Card != nil {
				need[cn.Card.Name] += cn.N
			}
		}
	}
	for _, x := range r.g.P[p].Z[engine.Hand] {
		if n := r.g.Def(x).Name; need[n] > 0 {
			need[n]--
		}
	}
	return need
}

var fromHand = map[string]bool{"PLAY": true, "PLAY_TREASURES_FOR": true, "DISCARD": true, "TRASH": true,
	"TOPDECK": true, "REVEAL_HAND": true, "REACTS_WITH": true, "REVEAL": true}

var reNonWord = regexp.MustCompile(`[^a-z]`)

func norm(s string) string { return reNonWord.ReplaceAllString(strings.ToLower(s), "") }

// kingdomOf reads the kingdom from a score file's "# kingdom:" line.
func kingdomOf(t *engine.Table, score string) ([]engine.CardID, error) {
	byNorm := map[string]engine.CardID{}
	for _, d := range t.Defs {
		byNorm[norm(d.Name)] = d.ID
	}
	for _, l := range strings.Split(score, "\n") {
		rest, ok := strings.CutPrefix(l, "# kingdom:")
		if !ok {
			continue
		}
		var out []engine.CardID
		for _, n := range strings.Split(rest, ",") {
			n = strings.TrimSpace(n)
			id, ok := byNorm[norm(n)]
			if !ok {
				return nil, fmt.Errorf("kingdom card %q isn't in the card table", n)
			}
			if t.Defs[id].Kind != "kingdom" {
				return nil, fmt.Errorf("kingdom card %q is a %s", n, t.Defs[id].Kind)
			}
			out = append(out, id)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out, nil
	}
	return nil, fmt.Errorf("the score file has no kingdom line")
}
