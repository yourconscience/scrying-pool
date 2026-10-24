package engine

import (
	"fmt"
	"math/rand/v2"
)

// Inst is one physical card in the game: an index into Game.cards. Supply cards
// are instances too, so a gain moves a card and the total never changes.
type Inst int32

// Zone is where a card is.
type Zone uint8

const (
	Deck     Zone = iota // the draw pile; its top is the end of the slice
	Hand                 //
	Discard              //
	InPlay               //
	Aside                // set aside, e.g. by Library
	Revealed             // looked at or revealed from the deck, until the card goes somewhere
	nPlayerZones
	Supply = nPlayerZones // a Supply pile
	Trash  = Supply + 1
)

// zone names as Cartographer writes them
var zoneNames = [...]string{"deck", "hand", "discard", "in play", "set aside", "revealed", "supply", "trash"}

func (z Zone) String() string { return zoneNames[z] }

// Loc is a zone and whose it is: a player for player zones, a pile for Supply.
type Loc struct {
	Zone  Zone
	Owner int16
}

type Player struct {
	Name    string
	Z       [nPlayerZones][]Inst
	Actions int
	Buys    int
	Coins   int
	Turns   int
}

type Pile struct {
	Card  CardID
	Cards []Inst
}

// MaxTurns stops a game that doesn't end, e.g. two bots that never buy Provinces.
const MaxTurns = 100

type Game struct {
	T       *Table
	Kingdom []CardID
	P       [2]Player
	Piles   []Pile
	Trashed []Inst
	Cur     int // whose turn it is
	Agents  [2]Agent
	Log     *Log // nil: no log

	// AfterTurn runs after every turn's Clean-up, e.g. to compare zones with a replay.
	AfterTurn func(g *Game)

	cards []CardID // by Inst
	loc   []Loc    // by Inst
	rng   *rand.Rand
	turn  int     // the current turn's number for Cur
	watch []watch // effects that last until Clean-up, e.g. Merchant's

	copper, silver, gold, estate, duchy, province, curse CardID
}

// Result is how a game ended.
type Result struct {
	VP     [2]int
	Turns  [2]int
	Winner int  // 0 or 1; -1 for a tie
	Capped bool // stopped at MaxTurns without ending
}

// New sets up a two-player game: the basic Supply, ten kingdom piles, starting
// decks of 7 Coppers and 3 Estates. Player 0 goes first.
func New(t *Table, kingdom []CardID, names [2]string, agents [2]Agent, seed uint64) (*Game, error) {
	g := &Game{T: t, Kingdom: kingdom, Agents: agents, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
	g.copper, g.silver, g.gold = t.MustCard("Copper"), t.MustCard("Silver"), t.MustCard("Gold")
	g.estate, g.duchy, g.province = t.MustCard("Estate"), t.MustCard("Duchy"), t.MustCard("Province")
	g.curse = t.MustCard("Curse")
	for i := range g.P {
		g.P[i].Name = names[i]
	}
	if len(kingdom) != 10 {
		return nil, fmt.Errorf("a kingdom has 10 cards, got %d", len(kingdom))
	}
	seen := map[CardID]bool{}
	for _, k := range kingdom {
		d := t.Defs[k]
		if d.Kind != "kingdom" || d.Rules == nil {
			return nil, fmt.Errorf("%s can't be played yet", d.Name)
		}
		if seen[k] {
			return nil, fmt.Errorf("%s is in the kingdom twice", d.Name)
		}
		seen[k] = true
	}
	// two players: 60 Coppers less the 14 in starting decks, 8 of each Victory card, 10 Curses
	basics := []struct {
		c CardID
		n int
	}{{g.copper, 46}, {g.silver, 40}, {g.gold, 30}, {g.estate, 8}, {g.duchy, 8}, {g.province, 8}, {g.curse, 10}}
	for _, b := range basics {
		g.addPile(b.c, b.n)
	}
	for _, k := range kingdom {
		n := 10
		if t.Defs[k].Victory() {
			n = 8
		}
		g.addPile(k, n)
	}
	for p := range g.P {
		for _, s := range []struct {
			c CardID
			n int
		}{{g.estate, 3}, {g.copper, 7}} {
			for range s.n {
				g.newCard(s.c, Loc{Discard, int16(p)}) // the first shuffle makes the deck
			}
		}
	}
	return g, nil
}

func (g *Game) addPile(c CardID, n int) {
	i := len(g.Piles)
	g.Piles = append(g.Piles, Pile{Card: c})
	for range n {
		g.newCard(c, Loc{Supply, int16(i)})
	}
}

func (g *Game) newCard(c CardID, l Loc) Inst {
	x := Inst(len(g.cards))
	g.cards = append(g.cards, c)
	g.loc = append(g.loc, l)
	s := g.slice(l)
	*s = append(*s, x)
	return x
}

// CardOf is the card kind of an instance.
func (g *Game) CardOf(x Inst) CardID { return g.cards[x] }

// Def is an instance's card.
func (g *Game) Def(x Inst) *Def { return g.T.Defs[g.cards[x]] }

// Where an instance is.
func (g *Game) Where(x Inst) Loc { return g.loc[x] }

func (g *Game) slice(l Loc) *[]Inst {
	switch l.Zone {
	case Supply:
		return &g.Piles[l.Owner].Cards
	case Trash:
		return &g.Trashed
	}
	return &g.P[l.Owner].Z[l.Zone]
}

// move takes a card from one zone to the top of another. Every card move goes
// through here but a shuffle. A card that isn't where the move expects it (it was shuffled,
// trashed, played by something else) stays put: the lose-track rule.
func (g *Game) move(x Inst, from Zone, to Loc) bool {
	l := g.loc[x]
	if l.Zone != from {
		return false
	}
	s := g.slice(l)
	for i := len(*s) - 1; i >= 0; i-- {
		if (*s)[i] == x {
			*s = append((*s)[:i], (*s)[i+1:]...)
			break
		}
	}
	t := g.slice(to)
	*t = append(*t, x)
	g.loc[x] = to
	return true
}

// pile finds a card's Supply pile.
func (g *Game) pile(c CardID) int {
	for i := range g.Piles {
		if g.Piles[i].Card == c {
			return i
		}
	}
	return -1
}

// Left is how many cards a Supply pile has; 0 when the card has no pile.
func (g *Game) Left(c CardID) int {
	if i := g.pile(c); i >= 0 {
		return len(g.Piles[i].Cards)
	}
	return 0
}

// EmptyPiles counts empty Supply piles.
func (g *Game) EmptyPiles() int {
	n := 0
	for _, p := range g.Piles {
		if len(p.Cards) == 0 {
			n++
		}
	}
	return n
}

// Cost is a card's current cost in $. Base cards never change costs.
func (g *Game) Cost(c CardID) int { return g.T.Defs[c].Cost }

// Owned lists every card a player has, in all their zones.
func (g *Game) Owned(p int) []Inst {
	var out []Inst
	for _, z := range g.P[p].Z {
		out = append(out, z...)
	}
	return out
}

// VP is a player's score now.
func (g *Game) VP(p int) int {
	vp := 0
	for _, x := range g.Owned(p) {
		if r := g.Def(x).Rules; r != nil && r.VP != nil {
			vp += r.VP(g, p)
		}
	}
	return vp
}

// shuffle turns the discard pile into the deck.
func (g *Game) shuffle(p int, level int) {
	pl := &g.P[p]
	d := pl.Z[Discard]
	g.rng.Shuffle(len(d), func(i, j int) { d[i], d[j] = d[j], d[i] })
	for _, x := range d {
		g.loc[x] = Loc{Deck, int16(p)}
	}
	pl.Z[Deck] = append(pl.Z[Deck], d...)
	pl.Z[Discard] = d[:0:0]
	g.Log.add(Entry{Level: level, Kind: 'S', Template: "SHUFFLES", Args: []any{seat(p)}})
}

// top takes the deck's top card into a zone, shuffling first if the deck is
// empty. It returns -1 when there is no card to take.
func (g *Game) top(p int, to Zone, level int) Inst {
	pl := &g.P[p]
	if len(pl.Z[Deck]) == 0 {
		if len(pl.Z[Discard]) == 0 {
			return -1
		}
		g.shuffle(p, level)
	}
	x := pl.Z[Deck][len(pl.Z[Deck])-1]
	g.move(x, Deck, Loc{to, int16(p)})
	return x
}

// draw draws up to n cards. When the deck runs out the log shows the shuffle
// first, then one line with every card drawn, as dominion.games does.
func (g *Game) draw(p, n, level int, template string, from CardID) []Inst {
	var out []Inst
	for range n {
		x := g.top(p, Hand, level)
		if x < 0 {
			break
		}
		out = append(out, x)
	}
	if len(out) > 0 {
		e := Entry{Level: level, Template: template, Args: []any{seat(p), g.ids(out)}}
		if template == "DRAW_FROM" {
			e.Args = append(e.Args, name(from))
		}
		g.Log.add(e)
	}
	return out
}

func (g *Game) ids(xs []Inst) []CardID {
	out := make([]CardID, len(xs))
	for i, x := range xs {
		out[i] = g.cards[x]
	}
	return out
}

// gain takes the top card of a Supply pile to a player's zone; -1 if the pile is empty.
func (g *Game) gain(p int, c CardID, to Zone) Inst {
	i := g.pile(c)
	if i < 0 || len(g.Piles[i].Cards) == 0 {
		return -1
	}
	s := g.Piles[i].Cards
	x := s[len(s)-1]
	g.move(x, Supply, Loc{to, int16(p)})
	return x
}

// Play runs the game to its end.
func (g *Game) Play() Result {
	g.Log.add(Entry{Template: "", Text: "Game #simulated"})
	for p := range g.P {
		g.Log.add(Entry{Template: "STARTS_WITH", Args: []any{seat(p), []CardID{g.estate, g.estate, g.estate}}})
		g.Log.add(Entry{Template: "STARTS_WITH", Args: []any{seat(p), repeat(g.copper, 7)}})
	}
	for p := range g.P {
		g.shuffle(p, 0)
	}
	for p := range g.P {
		g.draw(p, 5, 0, "DRAW", 0)
	}
	for {
		g.P[g.Cur].Turns++
		g.turn = g.P[g.Cur].Turns
		g.takeTurn()
		if g.AfterTurn != nil {
			g.AfterTurn(g)
		}
		if g.over() {
			return g.result(false)
		}
		if g.capped() {
			return g.result(true)
		}
		g.Cur = 1 - g.Cur
	}
}

func repeat(c CardID, n int) []CardID {
	out := make([]CardID, n)
	for i := range out {
		out[i] = c
	}
	return out
}

// over: the Provinces or any three Supply piles are empty.
func (g *Game) over() bool {
	return g.Left(g.province) == 0 || g.EmptyPiles() >= 3
}

func (g *Game) capped() bool { return g.Cur == 1 && g.P[1].Turns >= MaxTurns }

func (g *Game) result(capped bool) Result {
	r := Result{Capped: capped, Winner: -1}
	for p := range g.P {
		r.VP[p], r.Turns[p] = g.VP(p), g.P[p].Turns
	}
	switch {
	case r.VP[0] > r.VP[1]:
		r.Winner = 0
	case r.VP[1] > r.VP[0]:
		r.Winner = 1
	case r.Turns[0] > r.Turns[1]: // a tie goes to the player who had fewer turns
		r.Winner = 1
	}
	return r
}

func (g *Game) takeTurn() {
	p := g.Cur
	pl := &g.P[p]
	pl.Actions, pl.Buys, pl.Coins = 1, 1, 0
	g.Log.add(Entry{Kind: 'T', Text: fmt.Sprintf("Turn %d - %s", g.turn, pl.Name)})

	// Action phase
	for pl.Actions > 0 {
		opts := g.options(p, Hand, func(d *Def) bool { return d.Action() })
		if len(opts) == 0 {
			break
		}
		got := g.ask(&Decision{Player: p, Kind: PlayAction, Options: opts, Min: 0, Max: 1})
		if len(got) == 0 {
			break
		}
		pl.Actions--
		g.play(p, got[0].Inst, 0, false)
	}

	// Buy phase: Treasures, then buys
	for {
		opts := g.options(p, Hand, func(d *Def) bool { return d.Treasure() })
		if len(opts) == 0 {
			break
		}
		got := g.ask(&Decision{Player: p, Kind: PlayTreasures, Options: opts, Min: 0, Max: len(opts)})
		if len(got) == 0 {
			break
		}
		for _, o := range got {
			g.play(p, o.Inst, 0, false)
		}
	}
	for pl.Buys > 0 {
		var opts []Option
		for _, pile := range g.Piles {
			if len(pile.Cards) > 0 && g.Cost(pile.Card) <= pl.Coins && g.T.Defs[pile.Card].Potion == 0 && g.T.Defs[pile.Card].Debt == 0 {
				opts = append(opts, Option{Card: pile.Card, Inst: -1})
			}
		}
		got := g.ask(&Decision{Player: p, Kind: Buy, Options: opts, Min: 0, Max: 1})
		if len(got) == 0 {
			break
		}
		c := got[0].Card
		pl.Buys--
		pl.Coins -= g.Cost(c)
		g.gain(p, c, Discard)
		g.Log.add(Entry{Template: "BUY_AND_GAIN", Args: []any{seat(p), []CardID{c}}})
	}

	// Clean-up, except after the last turn: dominion.games ends the game and its log there
	if g.over() || g.capped() {
		return
	}
	for _, z := range []Zone{Hand, InPlay} {
		for len(pl.Z[z]) > 0 {
			g.move(pl.Z[z][0], z, Loc{Discard, int16(p)})
		}
	}
	g.watch = g.watch[:0]
	pl.Actions, pl.Buys, pl.Coins = 0, 0, 0
	g.draw(p, 5, 0, "DRAW", 0)
}

// options lists a player's cards in a zone that pass a test.
func (g *Game) options(p int, z Zone, ok func(*Def) bool) []Option {
	var out []Option
	for _, x := range g.P[p].Z[z] {
		if ok == nil || ok(g.Def(x)) {
			out = append(out, Option{Card: g.cards[x], Inst: x})
		}
	}
	return out
}

// ask puts a decision to the player's agent and checks the answer.
func (g *Game) ask(d *Decision) []Option {
	if len(d.Options) == 0 && d.Min == 0 {
		return nil
	}
	if d.Max > len(d.Options) {
		d.Max = len(d.Options)
	}
	if d.Min > d.Max {
		d.Min = d.Max
	}
	if d.Max == 0 {
		return nil
	}
	got := g.Agents[d.Player].Decide(&View{g: g, p: d.Player}, d)
	if len(got) < d.Min || len(got) > d.Max {
		panic(fmt.Sprintf("engine: %s answered %d options for %s, want %d..%d", g.P[d.Player].Name, len(got), d.Kind, d.Min, d.Max))
	}
	out := make([]Option, len(got))
	seen := map[int]bool{}
	for i, k := range got {
		if k < 0 || k >= len(d.Options) || seen[k] {
			panic(fmt.Sprintf("engine: %s answered option %d for %s of %d", g.P[d.Player].Name, k, d.Kind, len(d.Options)))
		}
		seen[k] = true
		out[i] = d.Options[k]
	}
	return out
}

// play puts a card into play from the hand (or plays it where it is, again)
// and runs its rules. level is the play line's log nesting.
func (g *Game) play(p int, x Inst, level int, again bool) {
	d := g.Def(x)
	if !again {
		g.move(x, g.loc[x].Zone, Loc{InPlay, int16(p)})
	}
	c := &Ctx{G: g, P: p, Me: x, level: level + 1}
	switch {
	case d.Treasure() && !d.Action() && level == 0 && !again:
		g.Log.treasure(p, g.cards[x], d.Rules.Coins)
	case again:
		g.Log.add(Entry{Level: level, Template: "PLAY_AGAIN", Args: []any{seat(p), []CardID{g.cards[x]}}})
	default:
		g.Log.add(Entry{Level: level, Template: "PLAY", Args: []any{seat(p), []CardID{g.cards[x]}}})
	}
	if d.Attack() {
		c.react()
	}
	if d.Rules.Coins != 0 {
		g.P[p].Coins += d.Rules.Coins
	}
	if d.Rules.Play != nil {
		d.Rules.Play(c)
	}
	for i := 0; i < len(g.watch); i++ {
		w := g.watch[i]
		if w.played != nil && w.played(&Ctx{G: g, P: w.p, Me: w.src, level: level + 1}, x) {
			g.watch = append(g.watch[:i], g.watch[i+1:]...)
			i--
		}
	}
}

// watch is an effect waiting for something this turn, e.g. Merchant's +$1 for
// the first Silver. Returns true when it is used up.
type watch struct {
	p      int
	src    Inst
	played func(c *Ctx, x Inst) bool
}

// Zones counts a player's cards by zone and name, as Cartographer's replay does.
func (g *Game) Zones(p int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for z, xs := range g.P[p].Z {
		if len(xs) == 0 {
			continue
		}
		m := map[string]int{}
		for _, x := range xs {
			m[g.Def(x).Name]++
		}
		out[Zone(z).String()] = m
	}
	return out
}

// Check tests the game's invariants: every card is in exactly the zone its
// location says, and no counter is negative.
func (g *Game) Check() error {
	seen := make([]bool, len(g.cards))
	visit := func(l Loc, xs []Inst) error {
		for _, x := range xs {
			if seen[x] {
				return fmt.Errorf("%s (#%d) is in two zones", g.Def(x).Name, x)
			}
			seen[x] = true
			if g.loc[x] != l {
				return fmt.Errorf("%s (#%d) is in %s of %d but its location says %s of %d", g.Def(x).Name, x, l.Zone, l.Owner, g.loc[x].Zone, g.loc[x].Owner)
			}
		}
		return nil
	}
	for p := range g.P {
		for z, xs := range g.P[p].Z {
			if err := visit(Loc{Zone(z), int16(p)}, xs); err != nil {
				return err
			}
		}
		if pl := g.P[p]; pl.Actions < 0 || pl.Buys < 0 || pl.Coins < 0 {
			return fmt.Errorf("%s has %d Actions, %d Buys, $%d", pl.Name, pl.Actions, pl.Buys, pl.Coins)
		}
	}
	for i, pile := range g.Piles {
		for _, x := range pile.Cards {
			if g.cards[x] != pile.Card {
				return fmt.Errorf("%s is in the %s pile", g.Def(x).Name, g.T.Defs[pile.Card].Name)
			}
		}
		if err := visit(Loc{Supply, int16(i)}, pile.Cards); err != nil {
			return err
		}
	}
	if err := visit(Loc{Trash, 0}, g.Trashed); err != nil {
		return err
	}
	for x, ok := range seen {
		if !ok {
			return fmt.Errorf("%s (#%d) is in no zone", g.Def(Inst(x)).Name, x)
		}
	}
	return nil
}
