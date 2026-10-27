// Package replay plays a saved dominion.games game again on the rules engine,
// making the choices its log shows and drawing the cards it names, and reports
// the first line where the engine's log differs from the real one. It is the
// engine's conformance check against real games (docs/simulator.md).
package replay

import (
	"fmt"
	"math"
	"slices"
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
	kingdom, lands, err := kingdomOf(t, score)
	if err != nil {
		rep.Stop = err.Error()
		return rep
	}
	var start [2][]engine.CardID
	for _, e := range rg.Events {
		if e.Template != "STARTS_WITH" || e.Level > 0 { // "1 starts with an Armory." is Inherited's
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
	takes, offHand := rg.DeckTakes()
	r.offHand = offHand
	for p, n := range r.names {
		r.takes[p] = takes[n]
	}
	colony := strings.Contains(logText, "Platinum") || strings.Contains(logText, "Colon")
	g, err := engine.New(t, engine.Setup{Kingdom: kingdom, Landscapes: lands, Names: r.names, Agents: [2]engine.Agent{r, r}, Start: start, Colony: colony, Partial: true})
	if err != nil {
		rep.Stop = err.Error()
		return rep
	}
	r.g = g
	r.arrangePiles()
	r.placeTraits()
	for i := len(r.real) - 1; i >= 0; i-- { // "Druid sets The Field's Gift aside.": Druid's Start takes them off the top first to last
		if x := r.real[i]; x.Template == "DRUID_SELECTS_BOONS" {
			r.raiseBoon(x)
		}
	}
	for _, x := range r.real { // "Obelisk chooses the Capital City pile."
		if x.Template == "OBELISK_CHOOSES_PILE" {
			name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(x.Args[0].Text), "the "), " pile")
			if id, ok := t.KingdomPile(name); ok {
				g.Obelisk = g.PileOf(id)
			}
			break
		}
	}
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
	offHand map[int][]string // named cards a line took from outside the hand
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
	r.arrangePiles()
}

// placeTraits puts the Traits on the piles the log says: "The Cheap Trait is acquired by the Pilgrim pile."
func (r *replayer) placeTraits() {
	for _, x := range r.real {
		if x.Template != "ACQUIRES_TRAIT" {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(x.Args[0].Text), "the "), " pile")
		pc, ok := r.t.KingdomPile(name)
		trait, ok2 := r.t.Landscape(x.Args[1].Text)
		if cs := x.Args[1].Cards; len(cs) > 0 && cs[0].Card != nil {
			trait, ok2 = r.t.Landscape(cs[0].Card.Name)
		}
		if !ok || !ok2 || r.g.PileOf(pc) < 0 {
			continue
		}
		for i := range r.g.Piles {
			if r.g.Piles[i].Trait == trait {
				r.g.Piles[i].Trait = 0
			}
		}
		r.g.Piles[r.g.PileOf(pc)].Trait = trait
	}
}

// raiseBoon puts the Boon or Hex a real line takes on top of its deck.
func (r *replayer) raiseBoon(x cartographer.Event) {
	cs := x.Args[1].Cards
	if len(cs) != 1 || cs[0].Card == nil {
		return
	}
	d, ok := r.t.Card(cs[0].Card.Name)
	if !ok || !d.Is(engine.TBoon|engine.THex) {
		return
	}
	deck, out := &r.g.Boons, &r.g.BoonsOut
	if d.Is(engine.THex) {
		deck, out = &r.g.Hexes, &r.g.HexesOut
	}
	for _, s := range []*[]engine.CardID{deck, out} {
		if i := slices.Index(*s, d.ID); i >= 0 {
			*s = slices.Delete(*s, i, i+1)
			*deck = append(*deck, d.ID)
			return
		}
	}
}

// arrangePiles puts the cards the next real line gains on top of their piles,
// for piles of different cards (Knights, Ruins) whose order the log shows only as they go,
// and the Boon or Hex it takes on top of its deck.
func (r *replayer) arrangePiles() {
	x, ok := r.peek()
	if ok && x.Template == "TAKE" {
		r.raiseBoon(x)
		return
	}
	if !ok || x.Template != "BUY_AND_GAIN" && x.Template != "GAIN" && x.Template != "GAIN_WITH" && x.Template != "GAIN_TO_HAND" && x.Template != "GAIN_ON_DRAWPILE" && x.Template != "REVEAL" {
		return
	}
	cs := x.Args[1].Cards
	for k := len(cs) - 1; k >= 0; k-- {
		if cs[k].Card == nil {
			continue
		}
		if d, ok := r.t.Card(cs[k].Card.Name); ok {
			for range cs[k].N {
				r.g.Raise(d.ID)
			}
		}
	}
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
		case string:
			if strings.TrimSpace(y.Text) != a {
				return "a different text"
			}
		case engine.Name:
			if a != 0 && norm(y.Text) != norm(r.t.Defs[a].Name) {
				return "a different card"
			}
		case engine.PileName:
			if norm(y.Text) != norm("the "+r.t.Defs[r.t.PileCard(engine.CardID(a))].Name+" pile") {
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
	n := map[string]int{} // by norm: the log's "Lost In The Woods" is "Lost in the Woods"
	for _, c := range have {
		n[norm(c)]++
	}
	unknown := 0
	for _, cn := range want {
		if cn.Card == nil {
			unknown += cn.N
			continue
		}
		if n[norm(cn.Card.Name)] < cn.N {
			return "different cards"
		}
		n[norm(cn.Card.Name)] -= cn.N
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
	engine.PlayTreasures: {"PLAY_TREASURES_FOR", "PLAY"},
	engine.Buy:           {"BUY_AND_GAIN", "BUY"},
	engine.ToGain:        {"GAIN", "GAIN_WITH"},                           // "(The Earth's Gift)" at the start of a turn
	engine.ToTrash:       {"TRASH", "TRASH_WITH"},                         // "(The Flame's Gift)" at the start of a turn
	engine.ToDiscard:     {"DISCARD", "DISCARD_REVEALED", "DISCARD_WITH"}, // Hostelry: "discards and reveals 2 Coppers."; "(Lost In The Woods)"
	engine.ToTopdeck:     {"TOPDECK"},
	engine.ToPlay:        {"PLAY", "PLAY_WITH", "PLAY_TREASURES_FOR"}, // Mastermind: "plays a Smithy with Mastermind."
	engine.ToSetAside:    {"SET_ASIDE_WITH", "SET_ASIDE"},
	engine.ToReveal:      {"REACTS_WITH", "REACTS_PLAYING", "REVEAL"},
	engine.PlayNight:     {"PLAY"},
	engine.ToExile:       {"EXILE"},
	engine.ToPass:        {"PASS"},
	engine.ToReturn:      {"RETURN_TO"},
	engine.ToCall:        {"CALL"},
	engine.ToPick:        {"REVEAL", "PUT_IN_HAND", "PUT_IN_HAND_FROM", "PUT_IN_DISCARD", "PUT_ON_BOTTOM_OF", "RECEIVES"}, // Druid's and Fool's Boons
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
	switch d.Kind {
	case engine.ToChoose:
		return r.chooseModes(d)
	case engine.ToName:
		return r.chooseName(d)
	case engine.UseCoffers:
		x, ok := r.peek()
		if ok && x.Player == r.names[d.Player] && (x.Template == "USES_COFFER" || x.Template == "USES_COFFERS") && x.Args[1].Number != nil {
			for i, o := range d.Options {
				if o.Mode == *x.Args[1].Number {
					return []int{i}
				}
			}
		}
		return []int{0}
	case engine.PlayAction:
		if x, ok := r.peek(); ok && x.Template == "PLAY_USING_WAY" && x.Player == r.names[d.Player] {
			return r.pickWay(d, x)
		}
	}
	x, ok := r.peek()
	if !ok {
		panic(stop{})
	}
	if d.Kind == engine.ToPass { // both players pick before either pass is logged
		for i := r.pos; i < min(len(r.real), r.pos+3); i++ {
			if y := r.real[i]; y.Template == "PASS" && y.Player == r.names[d.Player] {
				x = y
				break
			}
		}
	}
	if !r.answers(d, x) {
		if d.Min == 0 {
			return nil
		}
		r.fail(fmt.Sprintf("the engine asks %s to %s, the log doesn't", r.names[d.Player], d.Kind), "")
	}
	if d.Min == 0 && r.offersNone(d, x.Args[1].Cards) {
		return nil
	}
	return r.pick(d, x.Args[1].Cards)
}

// offersNone: the line names cards, none of which the decision offers, so it is a
// later decision's: Monastery's Coppers in play, after nothing from hand.
func (r *replayer) offersNone(d *engine.Decision, cards []cartographer.CardN) bool {
	for _, cn := range cards {
		if cn.Card == nil {
			return false
		}
		for _, o := range d.Options {
			if r.t.Defs[o.Card].Name == cn.Card.Name {
				return false
			}
		}
	}
	return len(cards) > 0
}

// chooseModes picks the modes whose first lines are the next real lines, in order.
func (r *replayer) chooseModes(d *engine.Decision) []int {
	var out []int
	used := map[int]bool{}
	start := r.pos
	if x, ok := r.peek(); ok && x.Template == "BUY" && x.Player == r.names[d.Player] && len(x.Args[1].Cards) == 1 && x.Args[1].Cards[0].Card != nil &&
		x.Args[1].Cards[0].Card.Name == r.t.Defs[d.Source].Name {
		start++ // the overpay is chosen before "buys a Farrier." is written, then "loses 1 Coin."
	}
	for i := start; i < len(r.real) && len(out) < d.Max; i++ {
		x := r.real[i]
		if x.Template == "NEW_TURN" || x.Level < d.Level {
			break
		}
		if x.Template == "SHUFFLES" || x.Level > d.Level {
			continue
		}
		k := -1
		for j, o := range d.Options {
			if !used[j] && o.Line != "" && (o.Line == x.Template || r.english[o.Line] == r.english[x.Template]) && r.sameNumber(o, x) {
				k = j
				break
			}
		}
		if k < 0 {
			break
		}
		used[k] = true
		out = append(out, k)
	}
	if len(out) < d.Min {
		for j, o := range d.Options { // the mode that writes nothing ("no")
			if o.Line == "" && !used[j] && len(out) < d.Min {
				used[j] = true
				out = append(out, j)
			}
		}
	}
	if len(out) < d.Min {
		x, _ := r.peek()
		r.fail(fmt.Sprintf("no mode of %s writes the log's next line %s", r.t.Defs[d.Source].Name, x.Template), "")
	}
	return out
}

// sameNumber: a numbered mode matches the number its line shows, when it shows one.
func (r *replayer) sameNumber(o engine.Option, x cartographer.Event) bool {
	if o.Label == "" || o.Label[0] < '0' || o.Label[0] > '9' {
		return true
	}
	n := x.Args[1].Number
	return n == nil || *n == o.Mode
}

// chooseName picks the card the next naming line names.
func (r *replayer) chooseName(d *engine.Decision) []int {
	for i := r.pos; i < min(len(r.real), r.pos+4); i++ {
		x := r.real[i]
		switch x.Template {
		case "NAMES", "WISH_CORRECT", "WISH_WRONG", "SORCERER_GUESS", "SORCERESS_GUESS":
			name := strings.TrimSpace(x.Args[1].Text)
			if cs := x.Args[1].Cards; len(cs) == 1 && cs[0].Card != nil {
				name = cs[0].Card.Name
			}
			for j, o := range d.Options {
				if r.t.Defs[o.Card].Name == name {
					return []int{j}
				}
			}
			r.fail("the log names "+name+", which the engine doesn't offer", "")
		}
	}
	r.fail("the engine asks to name a card, the log names none", "")
	return nil
}

// pickWay picks the card and the Way of a "plays X using Way of the Y" line.
func (r *replayer) pickWay(d *engine.Decision, x cartographer.Event) []int {
	card, way := "", strings.TrimSpace(x.Args[2].Text)
	if cs := x.Args[1].Cards; len(cs) == 1 && cs[0].Card != nil {
		card = cs[0].Card.Name
	}
	if cs := x.Args[2].Cards; len(cs) == 1 && cs[0].Card != nil {
		way = cs[0].Card.Name
	}
	for i, o := range d.Options {
		if o.Mode > 0 && r.t.Defs[o.Card].Name == card && strings.EqualFold(normWay(o.Label), normWay(way)) {
			return []int{i}
		}
	}
	r.fail("the log plays "+card+" using "+way+", which the engine doesn't offer", "")
	return nil
}

func normWay(s string) string { return strings.ReplaceAll(strings.ToLower(s), " ", "") }

// norm keeps only the letters, lower case: "Farmers' Market" is "farmersmarket".
func norm(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, strings.ToLower(s))
}

// answers says whether a real event is what a decision's answer writes.
func (r *replayer) answers(d *engine.Decision, x cartographer.Event) bool {
	if x.Player != r.names[d.Player] || x.Level != d.Level {
		return false
	}
	for _, t := range answers[d.Kind] {
		if x.Template == t || r.english[x.Template] == r.english[t] {
			if d.Kind == engine.PlayTreasures && x.Template == "PLAY" { // a Treasure with lines of its own: "plays a Stockpile."
				cs := x.Args[1].Cards
				return len(cs) == 1 && cs[0].Card != nil && cs[0].Card.Is("Treasure")
			}
			if d.Kind == engine.PlayAction || d.Kind == engine.PlayNight {
				cs := x.Args[1].Cards
				want := "Action"
				if d.Kind == engine.PlayNight {
					want = "Night"
				}
				return len(cs) == 1 && cs[0].Card != nil && cs[0].Card.Is(want)
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
		// a line names its cards by kind, not in the order drawn: across a shuffle the
		// old deck's come first, "draws 3 Coppers, a Moneylender, and a Forager." with a Forager left
		for k := r.took[p]; k < len(r.takes[p]) && r.takes[p][k].Line == take.Line; k++ {
			for i := len(deck) - 1; i >= 0; i-- {
				if c := r.takes[p][k].Card; c != "" && r.g.Def(deck[i]).Name == c {
					r.takes[p][k].Card = take.Card
					r.g.PutOnTop(deck[i])
					return
				}
			}
		}
		r.fail(fmt.Sprintf("the log takes a %s from %s's deck, which has none", take.Card, r.names[p]), "")
	}
	// an unnamed card: one this player shows from their hand while it's there,
	// else one they won't need in their next hand either; never one a later
	// line takes from this deck by name before it's shuffled again (a Cardinal's
	// "reveals an Estate and Dame Sylvia.")
	in, later := map[string]int{}, r.laterTakes(p, take.Line)
	for _, x := range deck {
		in[r.g.Def(x).Name]++
	}
	// of the cards that hand shows, the one it shows first: cards shown later may
	// come from draws in that turn ("plays a Page.", "draws a Copper.", "plays 4 Coppers.")
	need, first := r.demand(p, take.Line, 1)
	pick := -1
	for i := len(deck) - 1; i >= 0; i-- {
		if n := r.g.Def(deck[i]).Name; need[n] > 0 && in[n] > later[n] && (pick < 0 || first[n] < first[r.g.Def(deck[pick]).Name]) {
			pick = i
		}
	}
	if pick >= 0 {
		r.g.PutOnTop(deck[pick])
		return
	}
	soon, _ := r.demand(p, take.Line, 2)
	best, spare := -1, -1
	for i, x := range deck {
		if n := in[r.g.Def(x).Name] - soon[r.g.Def(x).Name] - later[r.g.Def(x).Name]; n > spare {
			best, spare = i, n
		}
	}
	if best >= 0 {
		r.g.PutOnTop(deck[best])
	}
}

// laterTakes counts the cards the log takes by name from player p's deck after
// the take at line, until their next shuffle.
func (r *replayer) laterTakes(p, line int) map[string]int {
	end := math.MaxInt
	if i, ok := r.line[line]; ok {
		for _, x := range r.real[i+1:] {
			if x.Template == "SHUFFLES" && x.Player == r.names[p] {
				end = x.Line
				break
			}
		}
	}
	later := map[string]int{}
	for _, t := range r.takes[p][r.took[p]:] {
		if t.Line >= end {
			break
		}
		if t.Card != "" {
			later[t.Card]++
		}
	}
	return later
}

// demand counts the cards player p shows from their hand by name in the next
// hands while cards drawn at a line are still there (windows 1: that hand;
// 2: that hand and the next), less the named cards already in hand, which
// serve only the first: they're discarded before the next hand is drawn.
// first is the line where the first hand shows each card. A card drawn in
// that hand's turn before it is shown, known by name ("draws a card.",
// "reveals a Copper." under Chariot Race), needn't come from this draw.
func (r *replayer) demand(p int, line, windows int) (map[string]int, map[string]int) {
	need, next, first, last := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	cur, second := need, false
	i, ok := r.line[line]
	if !ok {
		return need, first
	}
	name := r.names[p]
	ev := r.real[i]
	inTurn := ev.TurnOf == name && r.g.T2.Phase != engine.CleanupPhase // a Clean-up draw is for the next turn, also one under Borrow's "skips a draw"
	for _, x := range r.real[i:] {
		if x.Template == "NEW_TURN" {
			if x.Player == name {
				inTurn = true
			} else if inTurn {
				if windows--; windows == 0 {
					break
				}
				inTurn, cur, second = false, next, true
			}
			continue
		}
		if x.Player != name || !fromHand[x.Template] {
			continue
		}
		off := map[string]int{}
		for _, c := range r.offHand[x.Line] {
			off[c]++
		}
		for _, cn := range x.Args[1].Cards {
			if cn.Card != nil {
				k := max(0, cn.N-off[cn.Card.Name])
				cur[cn.Card.Name] += k
				if _, ok := first[cn.Card.Name]; !ok && k > 0 && !second {
					first[cn.Card.Name] = x.Line
				}
				if k > 0 && !second {
					last[cn.Card.Name] = x.Line
				}
			}
		}
	}
	for _, x := range r.g.P[p].Z[engine.Hand] {
		if n := r.g.Def(x).Name; need[n] > 0 {
			need[n]--
		}
	}
	for _, t := range r.takes[p][r.took[p]:] {
		if k, ok := r.line[t.Line]; ok && r.real[k].Template == "DRAW" && t.Card != "" && t.Line > line && t.Line < last[t.Card] && need[t.Card] > 0 {
			need[t.Card]--
		}
	}
	for n, k := range next {
		need[n] += k
	}
	return need, first
}

var fromHand = map[string]bool{"PLAY": true, "PLAY_TREASURES_FOR": true, "PLAY_USING_WAY": true, "DISCARD": true, "TRASH": true,
	"TOPDECK": true, "REVEAL_HAND": true, "REACTS_WITH": true, "REVEAL": true}

// kingdomOf reads the kingdom and landscapes from a score file's "# kingdom:"
// and "# landscapes:" lines.
func kingdomOf(t *engine.Table, score string) ([]engine.CardID, []engine.CardID, error) {
	var kingdom, lands []engine.CardID
	found := false
	for _, l := range strings.Split(score, "\n") {
		if rest, ok := strings.CutPrefix(l, "# kingdom:"); ok {
			found = true
			for _, n := range strings.Split(rest, ",") {
				n = strings.TrimSpace(n)
				id, ok := t.KingdomPile(n)
				if !ok {
					return nil, nil, fmt.Errorf("kingdom card %q isn't in the card table", n)
				}
				kingdom = append(kingdom, id)
			}
		}
		if rest, ok := strings.CutPrefix(l, "# landscapes:"); ok {
			for _, n := range strings.Split(rest, ",") {
				if n = strings.TrimSpace(n); n == "" {
					continue
				}
				id, ok := t.Landscape(n)
				if !ok {
					return nil, nil, fmt.Errorf("landscape %q isn't in the card table", n)
				}
				lands = append(lands, id)
			}
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("the score file has no kingdom line")
	}
	sort.Slice(kingdom, func(i, j int) bool { return kingdom[i] < kingdom[j] })
	return kingdom, lands, nil
}
