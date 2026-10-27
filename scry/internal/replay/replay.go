// Package replay plays a saved dominion.games game again on the rules engine,
// making the choices its log shows and drawing the cards it names, and reports
// the first line where the engine's log differs from the real one. It is the
// engine's conformance check against real games (docs/simulator.md).
package replay

import (
	"errors"
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
	kingdom, lands, err := KingdomOf(t, score)
	if errors.Is(err, errNoKingdom) {
		kingdom, lands, err = inferKingdom(t, rg.Events, score), inferLandscapes(t, rg.Events, score), nil
	}
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
	kingdom, pool := divineWind(t, kingdom, lands, r.real)
	if ferry, ok := t.KingdomPile("Ferryman"); ok && pool == nil && slices.Contains(kingdom, ferry) {
		pool = otherKingdom(t, kingdom, score) // Ferryman's pile: "# other cards: Imp, Develop, Horse"
	}
	g, err := engine.New(t, engine.Setup{Kingdom: kingdom, Pool: pool, Landscapes: lands, Names: r.names, Agents: [2]engine.Agent{r, r}, Start: start, Colony: colony, Partial: true})
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
				defer func() { r.rep.Stop = recover().(stop).why }() // fail panics, and this is past run's recover
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
	c          *cartographer.Client
	t          *engine.Table
	g          *engine.Game
	rep        *Report
	names      [2]string
	real       []cartographer.Event // the real log's events the engine writes too
	line       map[int]int          // real event index by log line
	english    map[string]string    // template text by name: templates with the same text are the same line
	pos        int                  // the next real event the engine hasn't written
	matched    int                  // the real event the last engine entry matched
	entry      int                  // that engine entry
	turn       string
	takes      [2][]cartographer.DeckTake
	took       [2]int
	offHand    map[int][]string     // named cards a line took from outside the hand
	hamlet     int                  // the engine line that played the Hamlet whose first discard was answered
	hidden     map[engine.Inst]bool // cards the replay chose for draws the log doesn't name, while no line has shown them
	schemeTurn string               // the turn Scheme's picks were counted in
	schemes    int                  // Scheme picks answered in it
	flat       bool                 // the log writes the choice the engine nests at the top level
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
	for x := range r.hidden { // played, trashed, set aside: shown
		if z := r.g.Where(x).Zone; z != engine.Hand && z != engine.Deck && z != engine.Discard && z != engine.Revealed {
			delete(r.hidden, x)
		}
	}
	e := l.Entries[i]
	if e.Kind == 0 && e.Template == "" {
		return // the engine's header line
	}
	if r.pos >= len(r.real) {
		panic(stop{}) // the real game ended here
	}
	x := r.real[r.pos]
	if (e.Template == "GETS_COIN" || e.Template == "GETS_COINS") && i > 0 && strings.HasPrefix(l.Entries[i-1].Template, "USES_COFFER") &&
		x.Template != "GETS_COIN" && x.Template != "GETS_COINS" {
		return // "uses 1 Coffers. (+$1)" has its "gets +$1." only sometimes
	}
	if e.Level == 0 {
		r.flat = false
	}
	if why := r.same(e, x, false); why != "" {
		r.fail(why, r.show(e))
	}
	if e.Kind == 'T' {
		r.turn = e.Text
		if x.Turn > 1 || x.Player != r.names[0] {
			r.rep.Matched++ // the previous turn matched to its end
		}
		r.mendHand(r.pos)
	}
	r.matched, r.entry = r.pos, i
	r.pos++
	if e.Template == "DRAW" || e.Template == "DRAW_FROM" { // Hunting Lodge's "draws 5 cards.", then "plays a Sailor."
		r.mendHand(r.matched)
	}
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
		text := "Turn " + strconv.Itoa(x.Turn) + " - " + x.Player
		if x.Extra != "" {
			text += " [" + x.Extra + "]" // "Turn 1 - gamrevinu [Island Folk]"
		}
		if x.Template != "NEW_TURN" || text != e.Text {
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
	if e.Level != x.Level && !(r.flat && e.Level == x.Level+1) {
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
	if x := r.real[r.pos]; x.Template == r.g.Log.Pending() && r.pos+1 < len(r.real) {
		return r.real[r.pos+1], true // the answer's line writes "begins their Cleanup phase." first: Patient's "sets a Dungeon aside."
	}
	return r.real[r.pos], true
}

// lines a decision's answer writes, by the decision's kind
var answers = map[engine.Kind][]string{
	engine.PlayAction:    {"PLAY"},
	engine.PlayTreasures: {"PLAY_TREASURES_FOR", "PLAY"},
	engine.Buy:           {"BUY_AND_GAIN", "BUY"},
	engine.ToGain:        {"GAIN", "GAIN_WITH", "GAIN_SETTING_ASIDE"},     // "(The Earth's Gift)" at the start of a turn; Blockade's
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
	engine.ToPick: {"REVEAL", "PUT_IN_HAND", "PUT_IN_HAND_FROM", "PUT_IN_DISCARD", "PUT_ON_BOTTOM_OF", "RECEIVES", // Druid's and Fool's Boons
		"ROTATES", "KEEPS_IN_HAND", "PLAYER_ADDS_FAVOR_TOKEN_TO", "BOTTOMDECKS"}, // Battle Plan's pile, Coastal Haven, Family of Inventors, Sibyl
}

// villager is the line after "uses 1 Villager.", written before the play it's for.
func (r *replayer) villager(d *engine.Decision, x cartographer.Event) cartographer.Event {
	if d.Kind == engine.PlayAction && x.Template == "USES_VILLAGER" && x.Player == r.names[d.Player] && r.pos+1 < len(r.real) {
		return r.real[r.pos+1]
	}
	return x
}

// firstN is the first n options.
func firstN(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
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
	if d.Kind == engine.ToTrash {
		if cs, ok := r.moreTrash(d); ok {
			return r.pick(d, cs)
		}
	}
	switch d.Kind {
	case engine.ToPick:
		switch r.t.Defs[d.Source].Name {
		case "Black Market":
			return r.blackMarket(d)
		case "Divine Wind": // the new piles the score file has
			return firstN(d.Min)
		case "Scheme":
			return r.schemePick(d)
		}
		if x, ok := r.peek(); ok && x.Template == "PLAYER_MOVES_TOKEN_TO" && x.Player == r.names[d.Player] { // Plan's "moves Trashing token to the Fortress pile."
			if i := r.pilePick(d, x); i >= 0 {
				return []int{i}
			}
		}
	case engine.ToDiscard:
		if r.t.Defs[d.Source].Name == "Hamlet" {
			return r.hamletDiscard(d)
		}
	case engine.ToReveal:
		if x, ok := r.peek(); ok && x.Template == "TRASH" && x.Player == r.names[d.Player] && x.Level == d.Level { // Hovel: "trashes a Hovel."
			cs := x.Args[1].Cards
			for i, o := range d.Options {
				if len(cs) == 1 && cs[0].Card != nil && r.t.Defs[o.Card].Name == cs[0].Card.Name {
					return []int{i}
				}
			}
		}
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
		if x, ok := r.peek(); ok && r.villager(d, x).Template == "PLAY_USING_WAY" && x.Player == r.names[d.Player] {
			return r.pickWay(d, r.villager(d, x))
		}
	}
	x, ok := r.peek()
	if !ok {
		panic(stop{})
	}
	x = r.villager(d, x)
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
	if cs := x.Args[1].Cards; d.Kind == engine.Buy && len(cs) == 1 && cs[0].N > 1 {
		return r.pick(d, []cartographer.CardN{{Card: cs[0].Card, N: 1}}) // "buys and gains 2 Silvers": one per buy, moreOfLast the rest
	}
	return r.pick(d, x.Args[1].Cards)
}

// hamletDiscard answers Hamlet's discards, for +1 Action and then for +1 Buy:
// the line after the log's discard says which it was ("discards an Estate.",
// "gets +1 Buy.").
func (r *replayer) hamletDiscard(d *engine.Decision) []int {
	play := -1
	for i := len(r.g.Log.Entries) - 1; i >= 0; i-- {
		if r.g.Log.Entries[i].Level < d.Level {
			play = i
			break
		}
	}
	first := play != r.hamlet
	r.hamlet = play
	x, ok := r.peek()
	if !ok || x.Template != "DISCARD" || x.Player != r.names[d.Player] || x.Level != d.Level {
		return nil
	}
	if first && r.pos+1 < len(r.real) && r.real[r.pos+1].Template == "GETS_BUY" {
		return nil
	}
	return r.pick(d, x.Args[1].Cards)
}

// blackMarket picks the top of the Black Market deck: the card the next lines
// gain and the ones they put back under it ("gains a Bridge.", "puts a Moat
// and a Spy on the bottom of the Black Market Deck."), then any others.
func (r *replayer) blackMarket(d *engine.Decision) []int {
	var names []string
	gained := false
	for _, x := range r.real[r.pos:] {
		if x.Template == "NEW_TURN" || x.Level < d.Level {
			break
		}
		if x.Level != d.Level || x.Player != r.names[d.Player] {
			continue
		}
		if x.Template == "GAIN" && !gained || x.Template == "PUT_ON_BOTTOM_OF" {
			gained = gained || x.Template == "GAIN"
			for _, cn := range x.Args[1].Cards {
				if cn.Card != nil {
					for range cn.N {
						names = append(names, cn.Card.Name)
					}
				}
			}
		}
		if x.Template == "PUT_ON_BOTTOM_OF" {
			break
		}
	}
	used := map[int]bool{}
	var out []int
	for _, n := range names {
		for i, o := range d.Options {
			if !used[i] && norm(r.t.Defs[o.Card].Name) == norm(n) {
				used[i] = true
				out = append(out, i)
				break
			}
		}
	}
	for i := range d.Options {
		if len(out) >= d.Min {
			break
		}
		if !used[i] {
			used[i] = true
			out = append(out, i)
		}
	}
	return out[:min(len(out), d.Max)]
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
	level := d.Level
	if start < len(r.real) && start > 0 && r.real[start].Level == 0 && d.Level == 1 && (r.flat || r.real[start-1].Level == 1) &&
		r.real[start].Template != "NEW_TURN" && r.real[start].Player == r.names[d.Player] {
		level = 0 // the log writes the choice at the top level: Hunting Lodge's "discards 4 other cards and a Copper."
	}
	for i := start; i < len(r.real) && len(out) < d.Max; i++ {
		x := r.real[i]
		if x.Template == "NEW_TURN" || x.Level < level {
			break
		}
		if x.Template == "SHUFFLES" || x.Level > level {
			continue
		}
		k := -1
		for j, o := range d.Options {
			if !used[j] && o.Line != "" && (o.Line == x.Template || r.english[o.Line] == r.english[x.Template]) && r.sameNumber(o, x) && sameCount(o, x) && r.sameCard(o, x) {
				k = j
				break
			}
		}
		if k < 0 && x.Template == "DRAW" && d.Max-len(out) >= 2 { // two draws on one line: Blacksmith's "draws 4 cards." under Elder
			n := 0
			for _, cn := range x.Args[1].Cards {
				n += cn.N
			}
			for j, o := range d.Options {
				for j2, o2 := range d.Options {
					if k < 0 && j < j2 && !used[j] && !used[j2] && o.Line == "DRAW" && o2.Line == "DRAW" && o.Count > 0 && o2.Count > 0 && o.Count+o2.Count == n {
						used[j], k = true, j2
						out = append(out, j)
					}
				}
			}
		}
		if k < 0 {
			break
		}
		used[k] = true
		out = append(out, k)
		if level != d.Level {
			r.flat = true
		}
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

// sameCard: a mode that names a card matches a line that names it: Kitsune's
// "gains a Curse." for the other player, not its "gain a Silver".
func (r *replayer) sameCard(o engine.Option, x cartographer.Event) bool {
	if o.Card == 0 {
		return true
	}
	for _, cn := range x.Args[1].Cards {
		if cn.Card != nil && norm(cn.Card.Name) == norm(r.t.Defs[o.Card].Name) {
			return true
		}
	}
	return false
}

// sameNumber: a numbered mode matches the number its line shows, when it shows one.
func (r *replayer) sameNumber(o engine.Option, x cartographer.Event) bool {
	if o.Label == "" || o.Label[0] < '0' || o.Label[0] > '9' {
		return true
	}
	n := x.Args[1].Number
	return n == nil || *n == o.Mode
}

// sameCount: a mode that draws so many cards matches a line that draws as many (Innkeeper's +3 Cards).
func sameCount(o engine.Option, x cartographer.Event) bool {
	if o.Count == 0 {
		return true
	}
	n := 0
	for _, cn := range x.Args[1].Cards {
		n += cn.N
	}
	return n == o.Count
}

// schemePick answers each Scheme's pick in Clean-up with one of the turn's
// "topdecks a card." lines, by the card the next hand draws.
func (r *replayer) schemePick(d *engine.Decision) []int {
	if r.schemeTurn != r.turn {
		r.schemeTurn, r.schemes = r.turn, 0
	}
	name := r.names[d.Player]
	var tops []cartographer.Event
	var drawn []cartographer.CardN
	for _, x := range r.real[r.pos:] {
		if x.Template == "NEW_TURN" {
			break
		}
		if x.Player == name && x.Template == "TOPDECK" {
			tops = append(tops, x)
		}
		if x.Player == name && x.Template == "DRAW" && x.Level == 0 {
			drawn = append(drawn, x.Args[1].Cards...)
		}
	}
	if r.schemes >= len(tops) || len(d.Options) == 0 {
		return nil
	}
	want := tops[r.schemes].Args[1].Cards
	r.schemes++
	var short []cartographer.CardN // drawn more often than the deck has it: it's the one topdecked
	inDeck := map[string]int{}
	for _, x := range r.g.P[d.Player].Z[engine.Deck] {
		inDeck[norm(r.g.Def(x).Name)]++
	}
	for _, cn := range drawn {
		if cn.Card != nil && cn.N > inDeck[norm(cn.Card.Name)] {
			short = append(short, cn)
		}
	}
	for _, cs := range [][]cartographer.CardN{want, short, drawn} {
		for _, cn := range cs {
			for i, o := range d.Options {
				if cn.Card != nil && norm(cn.Card.Name) == norm(r.t.Defs[o.Card].Name) {
					return []int{i}
				}
			}
		}
	}
	return []int{0}
}

// pilePick is the option on the pile a line names: "to the Fortress pile".
func (r *replayer) pilePick(d *engine.Decision, x cartographer.Event) int {
	for _, a := range x.Args {
		name := strings.TrimSpace(a.Text)
		if !strings.HasSuffix(name, " pile") {
			continue
		}
		id, ok := r.t.KingdomPile(strings.TrimSuffix(strings.TrimPrefix(name, "the "), " pile"))
		if !ok {
			continue
		}
		for i, o := range d.Options {
			if r.g.PileOf(o.Card) == r.g.PileOf(id) {
				return i
			}
		}
	}
	return -1
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
	if x.Player != r.names[r.whose(d)] {
		return false
	}
	if x.Level != d.Level {
		// the log sometimes writes a card's choice and what follows at the top level:
		// "gets +2 Actions.", then "discards 4 other cards and a Copper." (Hunting Lodge)
		if !(x.Level == 0 && d.Level == 1 && r.pos > 0 && (r.flat || r.real[r.pos-1].Level == 1)) {
			return false
		}
		r.flat = true
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

// whose is the player whose line answers a decision: the owner of the cards it offers,
// so the attacker's pick of what Pillage's victim discards is the victim's line.
func (r *replayer) whose(d *engine.Decision) int {
	if len(d.Options) > 0 && d.Options[0].Inst >= 0 {
		if p := r.g.Owner(d.Options[0].Inst); p >= 0 {
			return p
		}
	}
	return d.Player
}

// moreTrash: the last real line trashed more cards than the engine has yet,
// the rest of "trashes a Copper and a Ghost Town." for Remake's second trash.
func (r *replayer) moreTrash(d *engine.Decision) ([]cartographer.CardN, bool) {
	l := r.g.Log
	if r.entry >= len(l.Entries) || r.matched < 0 || r.pos == 0 {
		return nil, false
	}
	e, x := l.Entries[r.entry], r.real[r.matched]
	if e.Template != "TRASH" || x.Template != "TRASH" || x.Player != r.names[d.Player] || x.Level != d.Level {
		return nil, false
	}
	done := map[string]int{}
	for _, c := range e.Args[1].([]engine.CardID) {
		done[norm(r.t.Defs[c].Name)]++
	}
	var rest []cartographer.CardN
	n := 0
	for _, cn := range x.Args[1].Cards {
		k := cn.N
		if cn.Card != nil {
			m := min(k, done[norm(cn.Card.Name)])
			done[norm(cn.Card.Name)] -= m
			k -= m
		}
		if k > 0 && n < d.Max {
			k = min(k, d.Max-n)
			rest = append(rest, cartographer.CardN{Card: cn.Card, N: k})
			n += k
		}
	}
	return rest, len(rest) > 0
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
			if !found && d.Kind != engine.ToPick {
				if i := r.swapIn(d, cn.Card.Name, used); i >= 0 {
					used[i] = true
					out = append(out, i)
					continue
				}
			}
			if !found && d.Kind != engine.ToPick { // a pick takes its cards from a line that names others too: Hunter's
				r.fail(fmt.Sprintf("the log has %s %s %s, which the engine doesn't offer", r.names[d.Player], d.Kind, cn.Card.Name), "")
			}
		}
	}
	// an unnamed card is one the hand can spare: not a Copper it plays next turn
	// ("discards a card and an Estate.", then "plays 3 Coppers.")
	var need map[string]int
	left := map[string]int{}
	if unknown > 0 {
		x, _ := r.peek()
		need, _, _, _ = r.shown(r.whose(d), x.Line, 1)
		for _, cn := range cards {
			if cn.Card != nil {
				need[cn.Card.Name] -= cn.N // the line's own cards
			}
		}
		for i, o := range d.Options {
			if !used[i] {
				left[r.t.Defs[o.Card].Name]++
			}
		}
		if o := d.Options[0]; o.Inst >= 0 && r.g.Where(o.Inst).Zone == engine.Revealed { // what the next lines name stays: Miller's discarded Estate
			need = r.namedBelow(r.pos)
		}
	}
	for ; unknown > 0; unknown-- {
		k := -1
		for i, o := range d.Options {
			if n := r.t.Defs[o.Card].Name; !used[i] && (k < 0 || left[n] > need[n]) {
				if k = i; left[n] > need[n] {
					break
				}
			}
		}
		if k < 0 {
			break
		}
		used[k] = true
		out = append(out, k)
		left[r.t.Defs[d.Options[k].Card].Name]--
	}
	if d.Kind == engine.ToPick && len(out) > d.Max { // one of the cards the line names: Hunter's Action of "puts a Copper and a Mystic into their hand"
		out = out[:d.Max]
	}
	if x, ok := r.peek(); ok && d.Kind == engine.ToTrash && x.Template == "TRASH" && len(out) > d.Max && d.Max > 0 {
		out = out[:d.Max] // two trashes on one line: Remake's "trashes 2 Coppers." when the first gains nothing; moreTrash the rest
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
				delete(r.hidden, deck[i])
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
		if y := r.swapOut(p, take.Card); y >= 0 {
			r.g.PutOnTop(y)
			return
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
	if k, ok := r.line[take.Line]; ok && r.real[k].Template == "LOOK_AT" { // a card the lines below name
		want := r.namedBelow(k)
		for _, x := range r.g.P[p].Z[engine.Revealed] {
			want[r.g.Def(x).Name]--
		}
		for i := len(deck) - 1; i >= 0; i-- {
			if n := r.g.Def(deck[i]).Name; want[n] > 0 && in[n] > later[n] {
				r.hide(deck[i])
				r.g.PutOnTop(deck[i])
				return
			}
		}
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
		r.hide(deck[pick])
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
		r.hide(deck[best])
		r.g.PutOnTop(deck[best])
	}
}

func (r *replayer) hide(x engine.Inst) {
	if r.hidden == nil {
		r.hidden = map[engine.Inst]bool{}
	}
	r.hidden[x] = true
}

// hiddenInHand: a card the replay chose for an unnamed draw, still in the hand.
func (r *replayer) hiddenInHand(p int, x engine.Inst) bool {
	return r.hidden[x] && r.g.Where(x) == engine.Loc{Zone: engine.Hand, Owner: int16(p)}
}

// mendHand mends guessed hidden draws as a turn begins and after each draw:
// the cards the turn shows from the hand before its next draw or gain into it
// must be there
// ("uses 1 Villager.", "plays a Marauder."). A hand card drawn unseen that
// the turn doesn't show trades places with one in the deck, or with one
// discarded unseen.
func (r *replayer) mendHand(at int) {
	name := r.real[at].Player
	p := r.seat(name)
	stop := math.MaxInt // the next take from the deck
	if r.took[p] < len(r.takes[p]) {
		stop = r.takes[p][r.took[p]].Line
	}
	need := map[string]int{}
	for _, y := range r.real[at+1:] {
		if y.Template == "NEW_TURN" || y.Line >= stop {
			break
		}
		if y.Player != name {
			continue
		}
		if strings.HasPrefix(y.Template, "GAIN") || strings.HasPrefix(y.Template, "PUT_IN_HAND") {
			break
		}
		if !fromHand[y.Template] || y.Template == "REVEAL" {
			continue
		}
		off := map[string]int{}
		for _, c := range r.offHand[y.Line] {
			off[c]++
		}
		for _, cn := range y.Args[1].Cards {
			if cn.Card == nil {
				continue
			}
			if y.Cause == cn.Card.Name || strings.TrimSpace(y.Args[2].Text) == cn.Card.Name { // "trashes an Acting Troupe." from play, "trashes a Cabin Boy. (Cabin Boy)"
				continue
			}
			if d, ok := r.t.Card(cn.Card.Name); ok && !d.Is(engine.TShadow) { // a Shadow card may be played from the deck
				need[cn.Card.Name] += max(0, cn.N-off[cn.Card.Name])
			}
		}
	}
	have := map[string]int{}
	for _, h := range r.g.P[p].Z[engine.Hand] {
		have[r.g.Def(h).Name]++
	}
	for n, k := range need {
		for ; have[n] < k; have[n]++ {
			x := engine.Inst(-1)
			for _, d := range r.g.P[p].Z[engine.Deck] {
				if x < 0 && r.g.Def(d).Name == n {
					x = d
				}
			}
			for _, d := range r.g.P[p].Z[engine.Discard] {
				if x < 0 && r.hidden[d] && r.g.Def(d).Name == n {
					x = d
				}
			}
			y := engine.Inst(-1)
			for _, h := range r.g.P[p].Z[engine.Hand] {
				if m := r.g.Def(h).Name; y < 0 && r.hidden[h] && have[m] > need[m] {
					y = h
				}
			}
			if x < 0 || y < 0 {
				break
			}
			r.g.Swap(x, y)
			delete(r.hidden, x)
			have[r.g.Def(y).Name]--
		}
	}
}

// namedBelow counts the cards the lines after line at name, by its player at
// its level, until the level drops: what becomes of the cards it looked at
// (Miller's "looks at 4 cards.", "puts a card into their hand.", "discards 2
// other cards and an Estate.").
func (r *replayer) namedBelow(at int) map[string]int {
	x := r.real[at]
	need := map[string]int{}
	for _, y := range r.real[at+1:] {
		if y.Template == "NEW_TURN" || y.Level < x.Level || y.Template == "LOOK_AT" || y.Template == "SHUFFLES" {
			break
		}
		if y.Player != x.Player || y.Level != x.Level {
			continue
		}
		for _, cn := range y.Args[1].Cards {
			if cn.Card != nil {
				need[cn.Card.Name] += cn.N
			}
		}
	}
	return need
}

// swapIn mends a guess when the log names a card the hand doesn't have: a card
// drawn unseen that the hand can best spare trades places with one in the
// deck. It returns the option now holding the named card, or -1.
func (r *replayer) swapIn(d *engine.Decision, name string, used map[int]bool) int {
	p := r.whose(d)
	x := engine.Inst(-1)
	for _, y := range r.g.P[p].Z[engine.Deck] { // the bottom-most
		if r.g.Def(y).Name == name {
			x = y
			break
		}
	}
	for _, y := range r.g.P[p].Z[engine.Discard] { // else one discarded unseen: Clean-up's, or "discards a card."
		if x < 0 && r.hidden[y] && r.g.Def(y).Name == name {
			x = y
		}
	}
	if x < 0 {
		return -1
	}
	next, _ := r.peek()
	need, _, _, _ := r.shown(p, next.Line, 1)
	left := map[string]int{}
	for _, y := range r.g.P[p].Z[engine.Hand] {
		left[r.g.Def(y).Name]++
	}
	y, spare := engine.Inst(-1), 0
	for _, h := range r.g.P[p].Z[engine.Hand] {
		if n := r.g.Def(h).Name; r.hidden[h] && n != name && (y < 0 || left[n]-need[n] > spare) {
			y, spare = h, left[n]-need[n]
		}
	}
	if y < 0 {
		return -1
	}
	r.g.Swap(x, y)
	delete(r.hidden, x)
	for i, o := range d.Options {
		if o.Inst == y && !used[i] {
			d.Options[i].Card, d.Options[i].Inst = r.g.CardOf(x), x
			return i
		}
	}
	if d.Max == len(d.Options) { // any number of them: one more
		d.Max++
	}
	d.Options = append(d.Options, engine.Option{Card: r.g.CardOf(x), Inst: x})
	return len(d.Options) - 1
}

// swapOut mends a guess when the log draws a card the deck doesn't have: a
// card of that name drawn unseen goes back to the deck, trading places with
// one the log doesn't name later. It returns that card, or -1.
func (r *replayer) swapOut(p int, name string) engine.Inst {
	y := engine.Inst(-1)
	for _, h := range r.g.P[p].Z[engine.Hand] {
		if r.g.Def(h).Name == name && r.hiddenInHand(p, h) {
			y = h
			break
		}
	}
	deck := r.g.P[p].Z[engine.Deck]
	if y < 0 || len(deck) == 0 {
		return -1
	}
	in, later := map[string]int{}, r.laterTakes(p, r.takes[p][r.took[p]-1].Line)
	for _, x := range deck {
		in[r.g.Def(x).Name]++
	}
	for _, x := range deck {
		if n := r.g.Def(x).Name; n != name && in[n] > later[n] {
			r.g.Swap(x, y)
			r.hide(x)
			return y
		}
	}
	return -1
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
	need, next, first, last := r.shown(p, line, windows)
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
	// and one a line puts into the hand from elsewhere: Border Guard's "puts a Copper into their hand." of the two it revealed
	if i, ok := r.line[line]; ok {
		for _, x := range r.real[i+1:] {
			if x.Player != r.names[p] || x.Template != "PUT_IN_HAND" && x.Template != "PUT_IN_HAND_FROM" {
				continue
			}
			for _, cn := range x.Args[1].Cards {
				if cn.Card != nil && x.Line < last[cn.Card.Name] {
					need[cn.Card.Name] -= min(cn.N, need[cn.Card.Name])
				}
			}
		}
	}
	for n, k := range next {
		need[n] += k
	}
	return need, first
}

// shown counts the cards player p shows from their hand by name from line on,
// in that hand and with windows 2 the next one (next); first and last are the
// lines where the first hand shows each card first and last.
func (r *replayer) shown(p int, line, windows int) (need, next, first, last map[string]int) {
	need, next, first, last = map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	cur, second := need, false
	i, ok := r.line[line]
	if !ok {
		return
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
	return
}

var fromHand = map[string]bool{"PLAY": true, "PLAY_TREASURES_FOR": true, "PLAY_USING_WAY": true, "DISCARD": true, "TRASH": true,
	"TOPDECK": true, "REVEAL_HAND": true, "REACTS_WITH": true, "REVEAL": true,
	"TRASH_WITH": true, // "trashes a Copper. (Sailor)" at the start of a turn
	"SET_ASIDE":  true} // Patient's "sets a Dungeon aside." in Clean-up

// divineWind splits a kingdom of the piles before and after Divine Wind came
// true: the ones the log names before then, and others to make 10, come first;
// the rest are the new ones.
func divineWind(t *engine.Table, kingdom, lands []engine.CardID, real []cartographer.Event) ([]engine.CardID, []engine.CardID) {
	wind, ok := t.Landscape("Divine Wind")
	if !ok || !slices.Contains(lands, wind) || len(kingdom) <= 10 {
		return kingdom, nil
	}
	seen := map[engine.CardID]int{} // 1 before, 2 after
	after := false
	for _, x := range real {
		if x.Template == "COMES_TRUE" {
			after = true
		}
		for _, a := range x.Args {
			for _, cn := range a.Cards {
				if cn.Card == nil {
					continue
				}
				if id, ok := t.KingdomPile(cn.Card.Name); ok && seen[id] == 0 {
					seen[id] = 1
					if after {
						seen[id] = 2
					}
				}
			}
		}
	}
	var first, rest []engine.CardID
	for _, want := range []int{1, 0, 2} {
		for _, k := range kingdom {
			if seen[k] != want {
				continue
			}
			if len(first) < 10 {
				first = append(first, k)
			} else {
				rest = append(rest, k)
			}
		}
	}
	return first, rest
}

var errNoKingdom = errors.New("the score file has no kingdom line")

// namesIn lists the names a log and a score screen show: its lines' cards and
// texts ("uses Way of the Pig."), and the score screen's lines (Tower).
func namesIn(events []cartographer.Event, score string) []string {
	var out []string
	for _, e := range events {
		for _, a := range e.Args {
			for _, cn := range a.Cards {
				if cn.Card != nil {
					out = append(out, cn.Card.Name)
				}
			}
			if len(a.Cards) == 0 && a.Player == "" && a.Number == nil {
				out = append(out, strings.TrimSpace(a.Text))
			}
		}
	}
	for _, l := range strings.Split(score, "\n") {
		out = append(out, strings.TrimSpace(l))
	}
	return out
}

// inferKingdom guesses a kingdom the score file doesn't give from the piles
// the log and the score screen name; piles nothing touched stay out.
func inferKingdom(t *engine.Table, events []cartographer.Event, score string) []engine.CardID {
	seen := map[engine.CardID]bool{}
	var out []engine.CardID
	for _, n := range namesIn(events, score) {
		if id, ok := t.KingdomPile(n); ok && t.Defs[id].Kind == "kingdom" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// inferLandscapes does the same for the landscapes.
func inferLandscapes(t *engine.Table, events []cartographer.Event, score string) []engine.CardID {
	seen := map[engine.CardID]bool{}
	var out []engine.CardID
	for _, n := range namesIn(events, score) {
		if id, ok := t.Landscape(n); ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// otherKingdom is the kingdom cards a score file lists besides the kingdom.
func otherKingdom(t *engine.Table, kingdom []engine.CardID, score string) []engine.CardID {
	var out []engine.CardID
	for _, l := range strings.Split(score, "\n") {
		if rest, ok := strings.CutPrefix(l, "# other cards:"); ok {
			for _, n := range strings.Split(rest, ",") {
				if id, ok := t.KingdomPile(strings.TrimSpace(n)); ok && !slices.Contains(kingdom, id) {
					out = append(out, id)
				}
			}
		}
	}
	return out
}

// KingdomOf reads the kingdom and landscapes from a score file's "# kingdom:"
// and "# landscapes:" lines.
func KingdomOf(t *engine.Table, score string) ([]engine.CardID, []engine.CardID, error) {
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
		return nil, nil, errNoKingdom
	}
	sort.Slice(kingdom, func(i, j int) bool { return kingdom[i] < kingdom[j] })
	return kingdom, lands, nil
}
