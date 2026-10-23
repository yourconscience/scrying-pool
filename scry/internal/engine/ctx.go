package engine

import "fmt"

// Ctx is a card's effect as it runs: whose card it is, and how deep its log
// lines nest. Card code is straight-line Go over these calls; each move logs
// the line dominion.games would.
type Ctx struct {
	G     *Game
	P     int    // the player whose card it is
	Me    Inst   // the card, -1 for a landscape, Boon or State
	Src   CardID // the card whose rules run: Me's card, a Way, a landscape
	level int    // log nesting of the effect's lines

	from       bool    // the effect happens away from the card's play (a Duration's next turn): lines name the card
	holders    []Inst  // the cards that played this one (Throne Room), which stay in play with it
	unaffected [2]bool // players a Reaction protected from this Attack
	moved      []Zone  // where the cards the last moveAll moved came from
}

func (c *Ctx) Other() int { return 1 - c.P }

// Level is the log nesting of the effect's lines.
func (c *Ctx) Level() int { return c.level }

// Deeper is the same effect one log level down, e.g. for what a gain causes.
func (c *Ctx) Deeper() *Ctx {
	d := *c
	d.level++
	return &d
}

// As is the effect for another player, e.g. an Attack's victim gaining a Curse
// that then triggers their own cards.
func (c *Ctx) As(p int) *Ctx {
	d := *c
	d.P = p
	return &d
}

// Hand is a player's hand.
func (c *Ctx) Hand(p int) []Inst { return c.G.P[p].Z[Hand] }

// Zone is a player's cards in a zone.
func (c *Ctx) Zone(p int, z Zone) []Inst { return c.G.P[p].Z[z] }

// Pl is a player's state.
func (c *Ctx) Pl(p int) *Player { return &c.G.P[p] }

// Name is the card the effect is from.
func (c *Ctx) Name() string { return c.G.T.Defs[c.Src].Name }

// Card finds a card kind by name.
func (c *Ctx) Card(name string) CardID { return c.G.T.MustCard(name) }

// Log writes a line at the effect's level.
func (c *Ctx) Log(template string, args ...any) {
	c.G.Log.add(Entry{Level: c.level, Template: template, Args: args})
}

// from picks a template's "(Card)" variant when the effect is away from the card's play.
func (c *Ctx) withFrom(template string, args []any) (string, []any) {
	if c.from {
		return template + "_FROM", append(args, Name(c.Src))
	}
	return template, args
}

// Draw: +Cards.
func (c *Ctx) Draw(n int) []Inst { return c.DrawFor(c.P, n) }

// DrawFor draws cards for any player, e.g. Council Room's other player.
func (c *Ctx) DrawFor(p, n int) []Inst {
	var from CardID
	if c.from {
		from = c.Src
	}
	return c.G.draw(p, n, c.level, from)
}

func (c *Ctx) PlusActions(n int) {
	c.G.P[c.P].Actions += n
	t, a := c.withFrom(plural(n, "GETS_ACTION", "GETS_ACTIONS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

func (c *Ctx) PlusBuys(n int) {
	c.G.P[c.P].Buys += n
	t, a := c.withFrom(plural(n, "GETS_BUY", "GETS_BUYS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

func (c *Ctx) PlusCoins(n int) {
	pl := &c.G.P[c.P]
	if n > 0 && pl.MinusCoin {
		pl.MinusCoin = false
		c.Log("RETURNS_MINUS_COIN_TOKEN", Seat(c.P), Name(c.Card("MinusCoin")), Name(0))
		n--
		if n == 0 {
			return
		}
	}
	pl.Coins += n
	t, a := c.withFrom(plural(n, "GETS_COIN", "GETS_COINS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

// CoinsFrom is +$ that a card gives later, e.g. Merchant's for the first Silver.
func (c *Ctx) CoinsFrom(n int) {
	d := *c
	d.from = true
	d.PlusCoins(n)
}

func (c *Ctx) PlusPotions(n int) { c.G.P[c.P].Potions += n }

func (c *Ctx) PlusCoffers(n int) {
	c.G.P[c.P].Coffers += n
	t, a := c.withFrom(plural(n, "GETS_COFFER", "GETS_COFFERS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

func (c *Ctx) PlusVillagers(n int) {
	c.G.P[c.P].Villagers += n
	t, a := c.withFrom(plural(n, "GETS_VILLAGER", "GETS_VILLAGERS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

func (c *Ctx) PlusFavors(n int) {
	c.G.P[c.P].Favors += n
	t, a := c.withFrom(plural(n, "GETS_FAVOR", "GETS_FAVORS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

// PlusVP gives VP tokens.
func (c *Ctx) PlusVP(n int) {
	c.G.P[c.P].VP += n
	if c.from {
		c.Log("PLAYER_GETS_VP_FROM", Seat(c.P), n, Name(c.Src))
	} else {
		c.Log("PLAYER_GETS_VP", Seat(c.P), n)
	}
}

// TakeDebt takes debt tokens.
func (c *Ctx) TakeDebt(n int) { c.G.takeDebt(c.P, n, c.level) }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Choose asks player p to pick between min and max of the options.
func (c *Ctx) Choose(p int, k Kind, opts []Option, min, max int) []Option {
	return c.G.ask(&Decision{Player: p, Kind: k, Source: c.Src, Options: opts, Min: min, Max: max, Level: c.level})
}

// ChooseOne asks for one option, or none when may is true; nil when there is none.
func (c *Ctx) ChooseOne(p int, k Kind, opts []Option, may bool) *Option {
	min := 1
	if may {
		min = 0
	}
	got := c.Choose(p, k, opts, min, 1)
	if len(got) == 0 {
		return nil
	}
	return &got[0]
}

// Mode is one way a card can be used: its label, and the template of the
// first log line it writes, so a replay can tell which one a log chose.
type Mode struct {
	Label string
	Line  string
}

// Modes asks player p to pick between min and max of a card's modes; it returns their indexes.
func (c *Ctx) Modes(p, min, max int, ms ...Mode) []int {
	opts := make([]Option, len(ms))
	for i, m := range ms {
		opts[i] = Option{Inst: -1, Mode: i, Label: m.Label, Line: m.Line}
	}
	got := c.Choose(p, ToChoose, opts, min, max)
	out := make([]int, len(got))
	for i, o := range got {
		out[i] = o.Mode
	}
	return out
}

// Mode1 asks for one mode.
func (c *Ctx) Mode1(p int, ms ...Mode) int { return c.Modes(p, 1, 1, ms...)[0] }

// Number asks player p for a number from lo to hi.
func (c *Ctx) Number(p, lo, hi int, line string) int {
	var opts []Option
	for n := lo; n <= hi; n++ {
		opts = append(opts, Option{Inst: -1, Mode: n, Label: fmt.Sprint(n), Line: line})
	}
	return c.Choose(p, ToChoose, opts, 1, 1)[0].Mode
}

// Yes asks player p whether to do something optional; line is what doing it writes first.
func (c *Ctx) Yes(p int, label, line string) bool {
	return c.Mode1(p, Mode{"no", ""}, Mode{label, line}) == 1
}

// NameCard asks player p to name a card, from all the cards that could be in
// a deck; the caller logs it (NAMES, or Wishing Well's own line).
func (c *Ctx) NameCard(p int) CardID {
	var opts []Option
	for _, pile := range c.G.Piles {
		for _, id := range pile.kinds(c.G) {
			opts = append(opts, Option{Card: id, Inst: -1})
		}
	}
	return c.Choose(p, ToName, opts, 1, 1)[0].Card
}

// Options lists a player's cards in a zone that pass a test (nil: all).
func (c *Ctx) Options(p int, z Zone, ok func(*Def) bool) []Option { return c.G.options(p, z, ok) }

// Cards turns options into the instances they are.
func Cards(opts []Option) []Inst {
	out := make([]Inst, len(opts))
	for i, o := range opts {
		out[i] = o.Inst
	}
	return out
}

// OptionsOf makes options of instances.
func (c *Ctx) OptionsOf(xs []Inst) []Option {
	out := make([]Option, len(xs))
	for i, x := range xs {
		out[i] = Option{Card: c.G.cards[x], Inst: x}
	}
	return out
}

// In keeps the cards that are still in a zone.
func (c *Ctx) In(z Zone, xs []Inst) []Inst {
	var out []Inst
	for _, x := range xs {
		if c.G.loc[x].Zone == z {
			out = append(out, x)
		}
	}
	return out
}

// SupplyOptions lists the non-empty Supply piles whose top card passes a test.
func (c *Ctx) SupplyOptions(ok func(*Def) bool) []Option {
	var out []Option
	for _, p := range c.G.Piles {
		if p.Supply && len(p.Cards) > 0 {
			top := c.G.cards[p.Cards[len(p.Cards)-1]]
			if ok == nil || ok(c.G.T.Defs[top]) {
				out = append(out, Option{Card: top, Inst: -1})
			}
		}
	}
	return out
}

// CostsUpTo tests for a card costing at most $n (no Potion, no debt).
func (c *Ctx) CostsUpTo(n int) func(*Def) bool { return c.CostsAtMost(Upto(n)) }

// CostsAtMost tests for a card costing at most a cost.
func (c *Ctx) CostsAtMost(m Cost) func(*Def) bool {
	return func(d *Def) bool { return c.G.CostOf(d.ID).LE(m) }
}

// CostOf is a card kind's cost now.
func (c *Ctx) CostOf(id CardID) Cost { return c.G.CostOf(id) }

// Gain gains a card from its pile to a zone, logged as "gains"; -1 if it can't.
func (c *Ctx) Gain(p int, card CardID, to Zone) Inst {
	return c.GainAs(p, card, to, "GAIN")
}

// GainAs gains a card with a template of its own, e.g. GAIN_TO_HAND.
func (c *Ctx) GainAs(p int, card CardID, to Zone, template string) Inst {
	i := c.G.PileOf(card)
	if i < 0 || c.G.Top(i) != card {
		return -1
	}
	s := c.G.Piles[i].Cards
	x := s[len(s)-1]
	c.logGain(p, x, template)
	c.G.gainX(p, x, to, c.level+1, c.Src, false)
	return x
}

func (c *Ctx) logGain(p int, x Inst, template string) {
	args := []any{Seat(p), []CardID{c.G.cards[x]}}
	if c.from && template == "GAIN" {
		c.Log("GAIN_WITH", append(args, Name(c.Src))...)
		return
	}
	c.Log(template, args...)
}

// GainInst gains a particular card, e.g. one from the trash.
func (c *Ctx) GainInst(p int, x Inst, to Zone, template string) {
	c.logGain(p, x, template)
	c.G.gainX(p, x, to, c.level+1, c.Src, false)
}

// ChooseGain asks player p to gain a card from the Supply that passes a test.
func (c *Ctx) ChooseGain(p int, ok func(*Def) bool, to Zone) Inst {
	o := c.ChooseOne(p, ToGain, c.SupplyOptions(ok), false)
	if o == nil {
		return -1
	}
	return c.Gain(p, o.Card, to)
}

// gainX moves a card to a player's zone as a gain and runs what the gain
// causes; the caller logged it.
func (g *Game) gainX(p int, x Inst, to Zone, level int, by CardID, bought bool) {
	from := g.loc[x]
	if !g.move(x, from.Zone, Loc{to, int16(p)}) {
		return
	}
	if from.Zone == Supply {
		if pile := &g.Piles[from.Owner]; pile.Trade {
			pile.Trade = false
			g.TradeRoute++
		}
	}
	if p == g.Cur {
		g.T2.Gained = append(g.T2.Gained, g.cards[x])
	}
	if ex := g.copies(p, Exile, g.cards[x]); len(ex) > 0 {
		c := g.ctx(p, x, level)
		if c.Yes(p, "discard from Exile", "DISCARD_FROM_EXILE") {
			for _, y := range ex {
				g.move(y, Exile, Loc{Discard, int16(p)})
			}
			c.Log("DISCARD_FROM_EXILE", Seat(p), g.ids(ex))
		}
	}
	g.fire(&Ev{Kind: EvGain, P: p, X: x, C: g.cards[x], To: to, By: by, Bought: bought, Level: level})
}

func (g *Game) copies(p int, z Zone, c CardID) []Inst {
	var out []Inst
	for _, y := range g.P[p].Z[z] {
		if g.cards[y] == c {
			out = append(out, y)
		}
	}
	return out
}

// moveAll moves a player's cards from their zones and logs one line for those
// that moved; a card that has left the player's zones (trashed) stays where it is.
func (c *Ctx) moveAll(p int, xs []Inst, to Loc, template string, args ...any) []Inst {
	var moved []Inst
	var from []Zone
	for _, x := range xs {
		l := c.G.loc[x]
		if l.Zone < nPlayerZones && int(l.Owner) == p && (l.Zone != to.Zone || to.Zone == Deck) && c.G.move(x, l.Zone, to) {
			moved = append(moved, x)
			from = append(from, l.Zone)
		}
	}
	if len(moved) > 0 && template != "" {
		c.Log(template, append([]any{Seat(p), c.G.ids(moved)}, args...)...)
	}
	c.moved = from
	return moved
}

// Trash trashes a player's cards and runs what trashing them causes.
func (c *Ctx) Trash(p int, xs []Inst) []Inst {
	moved := c.moveAll(p, xs, Loc{Trash, 0}, "TRASH")
	from := c.moved
	for i, x := range moved {
		c.G.fire(&Ev{Kind: EvTrash, P: p, X: x, C: c.G.cards[x], From: from[i], By: c.Src, Level: c.level + 1})
	}
	return moved
}

// Discard discards a player's cards (not in Clean-up).
func (c *Ctx) Discard(p int, xs []Inst) []Inst {
	moved := c.moveAll(p, xs, Loc{Discard, int16(p)}, "DISCARD")
	from := c.moved
	for i, x := range moved {
		c.G.fire(&Ev{Kind: EvDiscard, P: p, X: x, C: c.G.cards[x], From: from[i], By: c.Src, Level: c.level + 1})
	}
	return moved
}

// Topdeck puts cards onto the deck; the last one ends on top.
func (c *Ctx) Topdeck(p int, xs []Inst) []Inst { return c.moveAll(p, xs, Loc{Deck, int16(p)}, "TOPDECK") }

// Bottomdeck puts cards on the bottom of the deck.
func (c *Ctx) Bottomdeck(p int, xs []Inst) {
	var moved []Inst
	for _, x := range xs {
		if c.G.Owner(x) == p {
			c.G.bottom(x, p)
			moved = append(moved, x)
		}
	}
	if len(moved) > 0 {
		c.Log("BOTTOMDECKS", Seat(p), c.G.ids(moved))
	}
}

// ToHand puts cards into a player's hand with a template, e.g. PUT_IN_HAND.
func (c *Ctx) ToHand(p int, xs []Inst, template string, args ...any) []Inst {
	return c.moveAll(p, xs, Loc{Hand, int16(p)}, template, args...)
}

// MoveTo moves a player's cards to one of their zones with a template ("" logs nothing).
func (c *Ctx) MoveTo(p int, xs []Inst, z Zone, template string, args ...any) []Inst {
	return c.moveAll(p, xs, Loc{z, int16(p)}, template, args...)
}

// SetAside sets cards aside, logged as "sets X aside".
func (c *Ctx) SetAside(p int, xs []Inst) []Inst { return c.MoveTo(p, xs, Aside, "SET_ASIDE") }

// SetAsideWith sets a card aside with this card, e.g. Library's skipped Actions.
func (c *Ctx) SetAsideWith(p int, xs ...Inst) []Inst {
	return c.MoveTo(p, xs, Aside, "SET_ASIDE_WITH", Name(c.Src))
}

// ReturnTo puts a card back on its pile.
func (c *Ctx) ReturnTo(p int, x Inst) bool {
	i := c.G.PileOf(c.G.cards[x])
	l := c.G.loc[x]
	if i < 0 || l.Zone == Supply {
		return false
	}
	if !c.G.move(x, l.Zone, Loc{Supply, int16(i)}) {
		return false
	}
	c.Log("RETURN_TO", Seat(p), []CardID{c.G.cards[x]}, PileName(c.G.Piles[i].Card))
	return true
}

// LookAt logs cards a player looks at without moving them.
func (c *Ctx) LookAt(p int, xs []Inst) {
	if len(xs) > 0 {
		c.Log("LOOK_AT", Seat(p), c.G.ids(xs))
	}
}

// Reveal logs cards a player reveals.
func (c *Ctx) Reveal(p int, xs []Inst) {
	if len(xs) > 0 {
		c.Log("REVEAL", Seat(p), c.G.ids(xs))
	}
}

func (c *Ctx) RevealHand(p int) {
	if len(c.G.P[p].Z[Hand]) > 0 {
		c.Log("REVEAL_HAND", Seat(p), c.G.ids(c.G.P[p].Z[Hand]))
	}
}

// FromTop takes up to n cards off the top of a deck into Revealed, shuffling
// if needed, and logs them with a template (REVEAL, LOOK_AT; "" for none).
func (c *Ctx) FromTop(p, n int, template string) []Inst {
	var out []Inst
	for range n {
		x := c.G.top(p, Revealed, c.level)
		if x < 0 {
			break
		}
		out = append(out, x)
	}
	if len(out) > 0 && template != "" {
		c.Log(template, Seat(p), c.G.ids(out))
	}
	return out
}

// RevealUntil reveals cards from the top of a deck until one passes a test or
// the deck and discard run out; it returns the revealed cards, the found one last (or -1).
func (c *Ctx) RevealUntil(p int, ok func(*Def) bool, n int) ([]Inst, []Inst) {
	var all, found []Inst
	for len(found) < n {
		x := c.G.top(p, Revealed, c.level)
		if x < 0 {
			break
		}
		all = append(all, x)
		if ok(c.G.Def(x)) {
			found = append(found, x)
		}
	}
	if len(all) > 0 {
		c.Log("REVEAL", Seat(p), c.G.ids(all))
	}
	return all, found
}

// DrawOne draws a single card without logging it; -1 when there is none.
func (c *Ctx) DrawOne(p int) Inst { return c.G.top(p, Hand, c.level) }

// TopCard takes a deck's top card into Revealed without logging; -1 when there is none.
func (c *Ctx) TopCard(p int) Inst { return c.G.top(p, Revealed, c.level) }

// DiscardTop discards a deck's top card; -1 when there is none.
func (c *Ctx) DiscardTop(p int) Inst {
	x := c.G.top(p, Revealed, c.level)
	if x >= 0 {
		c.Discard(p, []Inst{x})
	}
	return x
}

// PlayCard plays a card from where it is, e.g. Vassal's discarded Action or
// Throne Room's choice; times > 0 plays it again where it is.
func (c *Ctx) PlayCard(x Inst, times int) {
	c.G.play(c.P, x, c.level, playHow{times: times, holders: append(append([]Inst{}, c.holders...), c.Me)})
}

// PlayFrom plays a card with this card named on the play line ("(Prince)").
func (c *Ctx) PlayFrom(x Inst) {
	c.G.play(c.P, x, c.level-1, playHow{by: c.Src, holders: append(append([]Inst{}, c.holders...), c.Me)})
}

// Attack runs an Attack's effect on each other player a Reaction didn't protect.
func (c *Ctx) Attack(fn func(victim int)) {
	v := c.Other()
	if !c.unaffected[v] {
		fn(v)
	}
}

// Stay keeps this card in play for a later turn: its Next runs then.
func (c *Ctx) Stay() *Dur {
	d := &Dur{P: c.P, X: c.Me, C: c.Src, Hold: append([]Inst{}, c.holders...), Made: c.G.turns}
	c.G.durs = append(c.G.durs, d)
	return d
}

// Later runs a card's Next at the start of the player's next turn without the
// card staying in play (an Event's, a Boon's).
func (c *Ctx) Later() *Dur {
	d := &Dur{P: c.P, X: -1, C: c.Src, Made: c.G.turns}
	c.G.durs = append(c.G.durs, d)
	return d
}

// ExtraTurn gives the player an extra turn after this one.
func (c *Ctx) ExtraTurn() { c.G.extra = append(c.G.extra, c.Src) }

// Turn is a player's Ctx for a whole-turn fact: what they played and gained.
func (c *Ctx) T2() *turnState { return &c.G.T2 }

// Count counts a player's cards in a zone that pass a test.
func (c *Ctx) Count(p int, z Zone, ok func(*Def) bool) int {
	n := 0
	for _, x := range c.G.P[p].Z[z] {
		if ok == nil || ok(c.G.Def(x)) {
			n++
		}
	}
	return n
}

// Is tests a card kind by name.
func Is(name string) func(*Def) bool { return func(d *Def) bool { return d.Name == name } }

// Of tests for any of the types.
func Of(t Types) func(*Def) bool { return func(d *Def) bool { return d.Is(t) } }

// TrashFromSupply trashes the top card of a Supply pile, e.g. Lurker's.
func (c *Ctx) TrashFromSupply(p int, card CardID) Inst {
	i := c.G.PileOf(card)
	if i < 0 || c.G.Top(i) != card {
		return -1
	}
	s := c.G.Piles[i].Cards
	x := s[len(s)-1]
	c.G.move(x, Supply, Loc{Trash, 0})
	c.Log("TRASH", Seat(p), []CardID{card})
	c.G.fire(&Ev{Kind: EvTrash, P: p, X: x, C: card, From: Supply, By: c.Src, Level: c.level + 1})
	return x
}

// Pass moves a card from one player's hand to another's.
func (c *Ctx) Pass(from, to int, x Inst) {
	if c.G.move(x, Hand, Loc{Hand, int16(to)}) {
		c.Log("PASS", Seat(from), []CardID{c.G.cards[x]}, Seat(to))
	}
}

// Insert puts a card into a player's deck with n cards above it (0: on top).
func (c *Ctx) Insert(p int, x Inst, n int) {
	if n == 0 {
		c.Topdeck(p, []Inst{x})
		return
	}
	l := c.G.loc[x]
	if !c.G.move(x, l.Zone, Loc{Deck, int16(p)}) {
		return
	}
	d := c.G.P[p].Z[Deck]
	at := len(d) - 1 - n
	if at < 0 {
		at = 0
	}
	copy(d[at+1:], d[at:len(d)-1])
	d[at] = x
	c.Log("INSERT_IN_DECK", Seat(p), []CardID{c.G.cards[x]})
}
