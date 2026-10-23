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
	Deck          Zone = iota // the draw pile; its top is the end of the slice
	Hand                      //
	Discard                   //
	InPlay                    //
	Aside                     // set aside, e.g. by Library or Haven
	Revealed                  // looked at or revealed from the deck, until the card goes somewhere
	Tavern                    // the Tavern mat: Reserve cards
	Island                    // the Island mat
	NativeVillage             // the Native Village mat
	Exile                     // Exile
	nPlayerZones
	Supply = nPlayerZones // a pile, Supply or not
	Trash  = Supply + 1
)

// zone names as Cartographer writes them
var zoneNames = [...]string{"deck", "hand", "discard", "in play", "set aside", "revealed", "Tavern mat", "Island mat", "Native Village mat", "Exile", "supply", "trash"}

func (z Zone) String() string { return zoneNames[z] }

// Loc is a zone and whose it is: a player for player zones, a pile for Supply.
type Loc struct {
	Zone  Zone
	Owner int16
}

// Adventures tokens a player moves onto piles.
const (
	TokCard   = iota // +1 Card
	TokAction        // +1 Action
	TokBuy           // +1 Buy
	TokCoin          // +$1
	TokCost          // -$2 cost
	TokTrash         // Trashing
	TokEstate        // Inheritance's Estate token
	nTokens
)

type Player struct {
	Name    string
	Z       [nPlayerZones][]Inst
	Actions int
	Buys    int
	Coins   int
	Potions int
	Turns   int

	Coffers, Villagers, VP, Debt, Favors int

	Journey   bool         // the Journey token is face up
	MinusCoin bool         // the -$1 token: the next +$ is $1 less
	MinusCard bool         // the -1 Card token: the next draw is a card less
	Tokens    [nTokens]int // the pile each token is on, -1 for none

	Projects []CardID // Projects with this player's cube
}

// Held is a State or Artifact a player has, e.g. Lost in the Woods or the Flag.
type Held struct {
	C CardID
	P int
}

// MaxTurns stops a game that doesn't end, e.g. two bots that never buy Provinces.
const MaxTurns = 100

type Game struct {
	T       *Table
	Kingdom []CardID // the kingdom piles' cards
	Land    []CardID // the landscapes: Events, Landmarks, Projects, Ways, the Ally, Traits, the Prophecy
	P       [2]Player
	Piles   []Pile
	Trashed []Inst
	Cur     int // whose turn it is
	Agents  [2]Agent
	Log     *Log // nil: no log

	// AfterTurn runs after every turn's Clean-up, e.g. to compare zones with a replay.
	AfterTurn func(g *Game)
	// BeforeTake runs before a card is taken from a player's draw pile, so a
	// replay can put the card its log names on top (PutOnTop).
	BeforeTake func(p int)

	cards  []CardID // by Inst
	loc    []Loc    // by Inst
	rng    *rand.Rand
	pileOf map[CardID]int
	turn   int // the current turn's number for Cur
	turns  int // turns taken so far by both players, extra turns too
	start  [2][]CardID

	durs    []*Dur  // Durations and other cards waiting for later turns
	watches []Watch // effects waiting for something this turn, or longer
	hears   EvMask  // the events anything in the game responds to
	extra   []CardID
	ended   bool // the game is over

	T2 turnState // this turn's facts

	// shared game state of the expansions
	Colony     bool     // Platinum and Colony are in the Supply
	Boons      []CardID // the Boon deck, top at the end
	BoonsOut   []CardID // its discard pile
	Hexes      []CardID
	HexesOut   []CardID
	Druid      []CardID // the Boons Druid uses
	Held       []Held   // States and Artifacts
	LandVP     map[CardID]int // VP tokens on Landmarks
	Sun        int            // Sun tokens on the Prophecy; 0 once it came true
	Prophecy   CardID
	Mouse      CardID // the card Way of the Mouse uses
	Obelisk    int    // the pile Obelisk chose, -1
	Riverboat  CardID
	Approach   bool // Approaching Army's Attack pile
	TradeRoute int  // coin tokens on the Trade Route mat
	prevGained [2][]CardID

	copper, silver, gold, estate, duchy, province, curse, platinum, colony, potion CardID
}

// turnState is what happened this turn so far.
type turnState struct {
	Played    []Inst   // cards played this turn, in order
	Gained    []CardID // cards the player whose turn it is gained
	Bought    []CardID
	Reduce    int // cards cost $ less (Bridge)
	Phase     Phase
	Outpost   bool // this is an Outpost turn
	Extra     CardID
	NoBuy     bool // Mission: no buying cards
	Used      map[CardID]int
	ActionsIn int // Actions played this turn
	HandSize  int // cards drawn for the next hand in Clean-up

	CopperBonus int // Coppersmith: Copper makes $ more
}

type Phase uint8

const (
	StartPhase Phase = iota
	ActionPhase
	BuyPhase
	NightPhase
	CleanupPhase
)

// Result is how a game ended.
type Result struct {
	VP     [2]int
	Turns  [2]int
	Winner int  // 0 or 1; -1 for a tie
	Capped bool // stopped at MaxTurns without ending
}

// Setup is how a game starts.
type Setup struct {
	Kingdom    []CardID
	Landscapes []CardID
	Names      [2]string // player 0 goes first
	Agents     [2]Agent
	Seed       uint64
	Start      [2][]CardID // starting decks; empty: 3 Estates and 7 Coppers (or what the kingdom changes)
	Colony     bool        // add Platinum and Colony
	Shelters   bool        // start with Shelters instead of Estates
	Bane       CardID      // Young Witch's Bane, when Young Witch is in the kingdom
	// Partial allows kingdom cards without rules, for replays of real games:
	// they can be bought and gained, and playing one stops the game.
	Partial bool
}

// MissingRules stops a game that plays a card whose rules aren't written.
type MissingRules struct{ Card string }

func (m MissingRules) Error() string { return m.Card + " has no rules yet" }

// New sets up a two-player game: the basic Supply, the kingdom piles, the landscapes.
func New(t *Table, s Setup) (*Game, error) {
	g := &Game{T: t, Kingdom: s.Kingdom, Land: s.Landscapes, Agents: s.Agents, pileOf: map[CardID]int{},
		rng: rand.New(rand.NewPCG(s.Seed, s.Seed^0x9e3779b97f4a7c15)), Obelisk: -1,
		LandVP: map[CardID]int{}, Colony: s.Colony}
	g.copper, g.silver, g.gold = t.MustCard("Copper"), t.MustCard("Silver"), t.MustCard("Gold")
	g.estate, g.duchy, g.province = t.MustCard("Estate"), t.MustCard("Duchy"), t.MustCard("Province")
	g.curse, g.platinum, g.colony, g.potion = t.MustCard("Curse"), t.MustCard("Platinum"), t.MustCard("Colony"), t.MustCard("Potion")
	for i := range g.P {
		g.P[i].Name = s.Names[i]
		for k := range g.P[i].Tokens {
			g.P[i].Tokens[k] = -1
		}
	}
	if len(s.Kingdom) < 10 && !s.Partial {
		return nil, fmt.Errorf("a kingdom has 10 cards, got %d", len(s.Kingdom))
	}
	seen := map[CardID]bool{}
	for _, k := range append(append([]CardID{}, s.Kingdom...), s.Landscapes...) {
		d := t.Defs[k]
		if d.Kind != "kingdom" && !d.Landscape() || !t.HasRules(k) && !s.Partial {
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
	if s.Colony {
		basics = append(basics, struct {
			c CardID
			n int
		}{g.platinum, 12}, struct {
			c CardID
			n int
		}{g.colony, 8})
	}
	for _, b := range basics {
		g.AddPile(b.c, b.n, true)
	}
	needPotion := false
	for _, k := range s.Kingdom {
		if t.Defs[k].Cost.Potion > 0 {
			needPotion = true
		}
	}
	if needPotion {
		g.AddPile(g.potion, 16, true)
	}
	for _, k := range s.Kingdom {
		g.addKingdomPile(k)
	}
	if s.Bane != 0 {
		g.Piles[g.addKingdomPile(s.Bane)].Bane = true
	}
	for p := range g.P {
		g.start[p] = s.Start[p]
		if len(g.start[p]) == 0 {
			g.start[p] = g.startingDeck(s.Shelters)
		}
		for _, c := range g.start[p] {
			g.newCard(c, Loc{Discard, int16(p)})
		}
	}
	for _, c := range append(append([]CardID{}, s.Kingdom...), s.Landscapes...) {
		g.setupCard(c)
	}
	g.listening()
	return g, nil
}

func (g *Game) addKingdomPile(k CardID) int {
	d := g.T.Defs[k]
	if ms, ok := pileMembers[d.Name]; ok {
		return g.addMixed(k, ms, true, shuffledPiles[d.Name], 0)
	}
	n := 10
	if d.Victory() {
		n = 8
	}
	return g.AddPile(k, n, true)
}

// setupCard runs a kingdom card's (and its pile members') or a landscape's Setup.
func (g *Game) setupCard(c CardID) {
	d := g.T.Defs[c]
	if d.Rules != nil && d.Rules.Setup != nil {
		d.Rules.Setup(g)
	}
	if d.Is(TLooter) {
		g.AddRuins()
	}
	if d.Is(TProphecy) {
		g.Prophecy, g.Sun = c, 5 // two players
	}
	for _, m := range pileMembers[d.Name] {
		if md := g.T.byName[m.name]; md.Rules != nil && md.Rules.Setup != nil {
			md.Rules.Setup(g)
		}
	}
}

// startingDeck is 7 Coppers and 3 Estates, with Shelters for the Estates and
// Heirlooms for Coppers.
func (g *Game) startingDeck(shelters bool) []CardID {
	var deck []CardID
	if shelters {
		for _, n := range []string{"Hovel", "Necropolis", "Overgrown Estate"} {
			deck = append(deck, g.T.MustCard(n))
		}
	} else {
		deck = repeat(g.estate, 3)
	}
	coppers := 7
	for _, k := range g.Kingdom {
		if h, ok := heirlooms[g.T.Defs[k].Name]; ok {
			deck = append(deck, g.T.MustCard(h))
			coppers--
		}
	}
	return append(deck, repeat(g.copper, coppers)...)
}

var heirlooms = map[string]string{"Cemetery": "Haunted Mirror", "Fool": "Lucky Coin", "Pixie": "Goat",
	"Pooka": "Cursed Gold", "Secret Cave": "Magic Lamp", "Shepherd": "Pasture", "Tracker": "Pouch"}

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

// CtxFor is a card's effect context away from its play: a landscape's, a
// setup's, an effect that names the card ("(Importer)").
func (g *Game) CtxFor(p int, c CardID, level int) *Ctx {
	cx := g.ctx(p, -1, level)
	cx.Src, cx.from = c, true
	return cx
}

// Raise moves a card of a kind to the top of its pile, for replays of mixed piles.
func (g *Game) Raise(c CardID) bool {
	i := g.PileOf(c)
	if i < 0 {
		return false
	}
	s := g.Piles[i].Cards
	for k := len(s) - 1; k >= 0; k-- {
		if g.cards[s[k]] == c {
			x := s[k]
			copy(s[k:], s[k+1:])
			s[len(s)-1] = x
			return true
		}
	}
	return false
}

// D is a card kind's Def.
func (g *Game) D(c CardID) *Def { return g.T.Defs[c] }

// Where an instance is.
func (g *Game) Where(x Inst) Loc { return g.loc[x] }

// Owner of a card in a player's zone, or -1.
func (g *Game) Owner(x Inst) int {
	if l := g.loc[x]; l.Zone < nPlayerZones {
		return int(l.Owner)
	}
	return -1
}

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

// Move moves a card from wherever it is.
func (g *Game) Move(x Inst, to Loc) { g.move(x, g.loc[x].Zone, to) }

// bottom puts a card at the bottom of a deck.
func (g *Game) bottom(x Inst, p int) {
	if !g.move(x, g.loc[x].Zone, Loc{Deck, int16(p)}) {
		return
	}
	d := g.P[p].Z[Deck]
	copy(d[1:], d[:len(d)-1])
	d[0] = x
}

// Left is how many cards the pile a card comes from has; 0 when it has none.
func (g *Game) Left(c CardID) int {
	if i := g.PileOf(c); i >= 0 {
		return len(g.Piles[i].Cards)
	}
	return 0
}

// EmptyPiles counts empty Supply piles.
func (g *Game) EmptyPiles() int {
	n := 0
	for _, p := range g.Piles {
		if p.Supply && len(p.Cards) == 0 {
			n++
		}
	}
	return n
}

// Owned lists every card a player has, in all their zones.
func (g *Game) Owned(p int) []Inst {
	var out []Inst
	for _, z := range g.P[p].Z {
		out = append(out, z...)
	}
	return out
}

// Count counts a player's cards that pass a test.
func (g *Game) Count(p int, ok func(*Def) bool) int {
	n := 0
	for _, z := range g.P[p].Z {
		for _, x := range z {
			if ok(g.Def(x)) {
				n++
			}
		}
	}
	return n
}

// VP is a player's score now.
func (g *Game) VP(p int) int {
	vp := g.P[p].VP
	for _, x := range g.Owned(p) {
		if r := g.Def(x).Rules; r != nil && r.VP != nil {
			vp += r.VP(g, p)
		}
	}
	for _, c := range g.Land {
		if r := g.T.Defs[c].Rules; r != nil && r.VP != nil && g.T.Defs[c].Is(TLandmark) {
			vp += r.VP(g, p)
		}
	}
	for _, h := range g.Held {
		if r := g.T.Defs[h.C].Rules; h.P == p && r != nil && r.VP != nil {
			vp += r.VP(g, p)
		}
	}
	return vp
}

// shuffle turns the discard pile into the deck, under what is left of it.
func (g *Game) shuffle(p int, level int) {
	pl := &g.P[p]
	d := pl.Z[Discard]
	g.rng.Shuffle(len(d), func(i, j int) { d[i], d[j] = d[j], d[i] })
	g.Log.add(Entry{Level: level, Kind: 'S', Template: "SHUFFLES", Args: []any{Seat(p)}})
	for _, x := range d {
		g.loc[x] = Loc{Deck, int16(p)}
	}
	pl.Z[Deck] = append(d, pl.Z[Deck]...)
	pl.Z[Discard] = nil
	g.fire(&Ev{Kind: EvShuffle, P: p, X: -1})
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
	if g.BeforeTake != nil {
		g.BeforeTake(p)
	}
	x := pl.Z[Deck][len(pl.Z[Deck])-1]
	g.move(x, Deck, Loc{to, int16(p)})
	return x
}

// PutOnTop moves a card in its owner's draw pile to the top.
func (g *Game) PutOnTop(x Inst) {
	l := g.loc[x]
	if l.Zone == Deck {
		g.move(x, Deck, l)
	}
}

// draw draws up to n cards. When the deck runs out the log shows the shuffle
// first, then one line with every card drawn, as dominion.games does.
func (g *Game) draw(p, n, level int, from CardID) []Inst {
	if n > 0 && g.P[p].MinusCard {
		g.P[p].MinusCard = false
		g.Log.add(Entry{Level: level, Template: "RETURNS_TOKEN", Args: []any{Seat(p), Name(g.T.MustCard("MinusCard")), Name(0)}})
		n--
	}
	var out []Inst
	for range n {
		x := g.top(p, Hand, level)
		if x < 0 {
			break
		}
		out = append(out, x)
	}
	if len(out) > 0 {
		e := Entry{Level: level, Template: "DRAW", Args: []any{Seat(p), g.ids(out)}}
		if from != 0 {
			e.Template = "DRAW_FROM"
			e.Args = append(e.Args, Name(from))
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

// Play runs the game to its end.
func (g *Game) Play() Result {
	for _, c := range g.Land { // Landmarks, Allies, Traits and Prophecies act all game
		if d := g.T.Defs[c]; d.Rules == nil && d.Is(TLandmark|TAlly|TTrait|TProphecy) {
			panic(MissingRules{d.Name})
		}
	}
	g.Log.add(Entry{Template: "", Text: "Game #simulated"})
	for _, c := range append(append([]CardID{}, g.Kingdom...), g.Land...) {
		if r := g.T.Defs[c].Rules; r != nil && r.Start != nil {
			r.Start(g, c)
		}
	}
	for p := range g.P {
		// one line for the Estates (or Shelters), one for Heirlooms, one for the Coppers
		var groups [3][]CardID
		for _, c := range g.start[p] {
			k := 0
			switch {
			case g.T.Defs[c].Is(THeirloom):
				k = 1
			case c == g.copper:
				k = 2
			}
			groups[k] = append(groups[k], c)
		}
		for _, cs := range groups {
			if len(cs) > 0 {
				g.Log.add(Entry{Template: "STARTS_WITH", Args: []any{Seat(p), cs}})
			}
		}
	}
	for p := range g.P {
		g.shuffle(p, 0)
	}
	for p := range g.P {
		g.draw(p, 5, 0, 0)
	}
	for {
		g.P[g.Cur].Turns++
		g.turn = g.P[g.Cur].Turns
		g.takeTurn(0)
		if g.AfterTurn != nil {
			g.AfterTurn(g)
		}
		for len(g.extra) > 0 && !g.over() && !g.capped() {
			src := g.extra[0]
			g.extra = g.extra[1:]
			g.takeTurn(src)
			if g.AfterTurn != nil {
				g.AfterTurn(g)
			}
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

// over: the Provinces (or Colonies) or any three Supply piles are empty.
func (g *Game) over() bool {
	return g.ended || g.Left(g.province) == 0 || g.Colony && g.Left(g.colony) == 0 || g.EmptyPiles() >= 3
}

// capped: the second player has taken MaxTurns turns, or extra turns kept
// coming until both players together took three times as many.
func (g *Game) capped() bool {
	return g.Cur == 1 && g.P[1].Turns >= MaxTurns || g.turns >= 3*MaxTurns
}

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
		pl := g.P[p]
		if pl.Actions < 0 || pl.Buys < 0 || pl.Coins < 0 || pl.Potions < 0 || pl.Coffers < 0 || pl.Villagers < 0 || pl.Debt < 0 || pl.Favors < 0 {
			return fmt.Errorf("%s has %d Actions, %d Buys, $%d, %d Potions, %d Coffers, %d Villagers, %d debt, %d Favors",
				pl.Name, pl.Actions, pl.Buys, pl.Coins, pl.Potions, pl.Coffers, pl.Villagers, pl.Debt, pl.Favors)
		}
	}
	for i, pile := range g.Piles {
		for _, x := range pile.Cards {
			if g.PileOf(g.cards[x]) != i {
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
