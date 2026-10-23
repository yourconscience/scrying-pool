package engine

// Ctx is a card's effect as it runs: whose card it is, and how deep its log
// lines nest. Card code is straight-line Go over these calls; each move logs
// the line dominion.games would.
type Ctx struct {
	G     *Game
	P     int  // the player whose card it is
	Me    Inst // the card
	level int  // log nesting of the effect's lines

	unaffected [2]bool // players a Reaction protected from this Attack
}

func (c *Ctx) Other() int { return 1 - c.P }

// Hand is a player's hand.
func (c *Ctx) Hand(p int) []Inst { return c.G.P[p].Z[Hand] }

func (c *Ctx) log(template string, args ...any) {
	c.G.Log.add(Entry{Level: c.level, Template: template, Args: args})
}

// Draw: +Cards.
func (c *Ctx) Draw(n int) []Inst { return c.DrawFor(c.P, n) }

// DrawFor draws cards for any player, e.g. Council Room's other player.
func (c *Ctx) DrawFor(p, n int) []Inst { return c.G.draw(p, n, c.level, "DRAW", 0) }

func (c *Ctx) PlusActions(n int) {
	c.G.P[c.P].Actions += n
	c.log(plural(n, "GETS_ACTION", "GETS_ACTIONS"), seat(c.P), n)
}

func (c *Ctx) PlusBuys(n int) {
	c.G.P[c.P].Buys += n
	c.log(plural(n, "GETS_BUY", "GETS_BUYS"), seat(c.P), n)
}

func (c *Ctx) PlusCoins(n int) {
	c.G.P[c.P].Coins += n
	c.log(plural(n, "GETS_COIN", "GETS_COINS"), seat(c.P), n)
}

// CoinsFrom is +$ that a card gives later, e.g. Merchant's for the first Silver.
func (c *Ctx) CoinsFrom(n int) {
	c.G.P[c.P].Coins += n
	c.log(plural(n, "GETS_COIN_FROM", "GETS_COINS_FROM"), seat(c.P), n, name(c.G.cards[c.Me]))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Choose asks player p to pick between min and max of the options.
func (c *Ctx) Choose(p int, k Kind, opts []Option, min, max int) []Option {
	return c.G.ask(&Decision{Player: p, Kind: k, Source: c.G.cards[c.Me], Options: opts, Min: min, Max: max})
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

// SupplyOptions lists the non-empty Supply piles whose card passes a test.
func (c *Ctx) SupplyOptions(ok func(*Def) bool) []Option {
	var out []Option
	for _, p := range c.G.Piles {
		if len(p.Cards) > 0 && ok(c.G.T.Defs[p.Card]) {
			out = append(out, Option{Card: p.Card, Inst: -1})
		}
	}
	return out
}

// CostsUpTo tests for a card costing at most $n, and no Potion or debt.
func (c *Ctx) CostsUpTo(n int) func(*Def) bool {
	return func(d *Def) bool { return c.G.Cost(d.ID) <= n && d.Potion == 0 && d.Debt == 0 }
}

// Gain gains a card to a zone. The log says only "gains"; the zone follows from the card that gains it.
func (c *Ctx) Gain(p int, card CardID, to Zone) Inst {
	x := c.G.gain(p, card, to)
	if x >= 0 {
		c.log("GAIN", seat(p), []CardID{card})
	}
	return x
}

// ChooseGain asks player p to gain a card from the Supply that passes a test.
func (c *Ctx) ChooseGain(p int, ok func(*Def) bool, to Zone) Inst {
	got := c.Choose(p, ToGain, c.SupplyOptions(ok), 1, 1)
	if len(got) == 0 {
		return -1
	}
	return c.Gain(p, got[0].Card, to)
}

// moveAll moves a player's cards from their zones and logs one line for those
// that moved; a card that has left the player's zones (trashed) stays where it is.
func (c *Ctx) moveAll(p int, xs []Inst, to Loc, template string) {
	var moved []Inst
	for _, x := range xs {
		l := c.G.loc[x]
		if l.Zone < nPlayerZones && int(l.Owner) == p && l.Zone != to.Zone && c.G.move(x, l.Zone, to) {
			moved = append(moved, x)
		}
	}
	if len(moved) > 0 {
		c.log(template, seat(p), c.G.ids(moved))
	}
}

func (c *Ctx) Trash(p int, xs []Inst)   { c.moveAll(p, xs, Loc{Trash, 0}, "TRASH") }
func (c *Ctx) Discard(p int, xs []Inst) { c.moveAll(p, xs, Loc{Discard, int16(p)}, "DISCARD") }

// Topdeck puts cards onto the deck; the last one ends on top.
func (c *Ctx) Topdeck(p int, xs []Inst) { c.moveAll(p, xs, Loc{Deck, int16(p)}, "TOPDECK") }

// SetAsideWith sets a card aside with this card, e.g. Library's skipped Actions.
func (c *Ctx) SetAsideWith(p int, x Inst) {
	if c.G.move(x, c.G.loc[x].Zone, Loc{Aside, int16(p)}) {
		c.log("SET_ASIDE_WITH", seat(p), []CardID{c.G.cards[x]}, name(c.G.cards[c.Me]))
	}
}

// LookAt logs cards a player looks at without moving them.
func (c *Ctx) LookAt(p int, xs []Inst) {
	if len(xs) > 0 {
		c.log("LOOK_AT", seat(p), c.G.ids(xs))
	}
}

func (c *Ctx) RevealHand(p int) { c.log("REVEAL_HAND", seat(p), c.G.ids(c.G.P[p].Z[Hand])) }

// FromTop takes up to n cards off the top of a deck into Revealed, shuffling
// if needed, and logs them as revealed or looked at.
func (c *Ctx) FromTop(p, n int, template string) []Inst {
	var out []Inst
	for range n {
		x := c.G.top(p, Revealed, c.level)
		if x < 0 {
			break
		}
		out = append(out, x)
	}
	if len(out) > 0 {
		c.log(template, seat(p), c.G.ids(out))
	}
	return out
}

// DrawOne draws a single card without logging it; -1 when there is none.
func (c *Ctx) DrawOne(p int) Inst { return c.G.top(p, Hand, c.level) }

// DiscardTop discards a deck's top card; -1 when there is none.
func (c *Ctx) DiscardTop(p int) Inst {
	x := c.G.top(p, Discard, c.level)
	if x >= 0 {
		c.log("DISCARD", seat(p), []CardID{c.G.cards[x]})
	}
	return x
}

// PlayCard plays a card from where it is, e.g. Vassal's discarded Action or
// Throne Room's choice; again plays it a further time where it is.
func (c *Ctx) PlayCard(x Inst, again bool) { c.G.play(c.P, x, c.level, again) }

// Attack runs an Attack's effect on each other player a Reaction didn't protect.
func (c *Ctx) Attack(fn func(victim int)) {
	v := c.Other()
	if !c.unaffected[v] {
		fn(v)
	}
}

// react lets the other player reveal Reactions to an Attack being played.
func (c *Ctx) react() {
	v := c.Other()
	opts := c.G.options(v, Hand, func(d *Def) bool { return d.Rules != nil && d.Rules.React != nil })
	got := c.G.ask(&Decision{Player: v, Kind: ToReveal, Source: c.G.cards[c.Me], Options: opts, Min: 0, Max: 1})
	if len(got) == 0 {
		return
	}
	c.log("REACTS_WITH", seat(v), []CardID{got[0].Card})
	if c.G.Def(got[0].Inst).Rules.React(c, got[0].Inst) {
		c.unaffected[v] = true
	}
}

// WhenPlay runs fn for every card played for the rest of the turn, until it returns true.
func (c *Ctx) WhenPlay(fn func(c *Ctx, x Inst) bool) {
	c.G.watch = append(c.G.watch, watch{p: c.P, src: c.Me, played: fn})
}
