package cards

import (
	"slices"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// The current Promo cards. Simplifications: Stash goes where the shuffle puts
// it; a card bought with Black Market is paid for and gained, without what
// buying a card causes (Haggler, Hoard), and the rest go under the deck in the
// order revealed.

// setAsideWith sets cards aside "with" this card, and writes "sets nothing
// aside with Church." when there are none.
func setAsideWith(c *ctx, p int, xs []inst) []inst {
	if len(xs) == 0 {
		c.Log("SET_ASIDE_WITH", engine.Seat(p), []engine.CardID{}, engine.Name(c.Src))
		return nil
	}
	return c.SetAsideWith(p, xs...)
}

// playFromHand plays a named card from hand if the player wants: Sauna's Avanto.
func playFromHand(c *ctx, name string) {
	if x := pick1(c, c.P, engine.ToPlay, hand, named(name), true); x >= 0 {
		c.PlayCard(x, 0)
	}
}

// topOf is a Supply pile's top card for a choice of its card; -1 for none.
func topOf(c *ctx, card engine.CardID) inst {
	i := c.G.PileOf(card)
	if i < 0 || c.G.Top(i) != card {
		return -1
	}
	s := c.G.Piles[i].Cards
	return s[len(s)-1]
}

func init() {
	reg("Black Market", rules{
		Setup: func(g *engine.Game) { // one of each card of the unused kingdom piles, shuffled
			if g.BlackMarket != nil {
				return
			}
			var us []engine.CardID
			for _, c := range g.Unused(func(d *def) bool { return d.Kind == "kingdom" }) {
				us = append(us, g.T.Members(c)...)
			}
			for i := len(us) - 1; i > 0; i-- {
				j := g.Rand(i + 1)
				us[i], us[j] = us[j], us[i]
			}
			for _, c := range us {
				g.AddPile(c, 1, false)
			}
			g.BlackMarket = us
			for _, c := range us {
				if g.T.Defs[c].Cost.Potion > 0 {
					g.AddPile(g.T.MustCard("Potion"), 16, true)
				}
			}
		},
		Play: func(c *ctx) { // "gets +$2.", "plays a Silver. (+$2)", "gains a Bridge.", "puts a Moat and a Spy on the bottom of the Black Market Deck."
			c.PlusCoins(2)
			g := c.G
			// the top three, as a pick so that a replay can take the ones its log names
			var opts []engine.Option
			for i := len(g.BlackMarket) - 1; i >= 0; i-- {
				opts = append(opts, engine.Option{Card: g.BlackMarket[i], Inst: -1})
			}
			n := min(3, len(opts))
			var shown []engine.CardID
			for _, o := range c.Choose(c.P, engine.ToPick, opts, n, n) {
				shown = append(shown, o.Card)
				i := slices.Index(g.BlackMarket, o.Card)
				g.BlackMarket = slices.Delete(g.BlackMarket, i, i+1)
			}
			c.PlayMerged(pick(c, c.P, engine.PlayTreasures, hand, treasure, 0, len(c.Hand(c.P))))
			pl := c.Pl(c.P)
			var buyable []engine.Option
			for _, card := range shown {
				if cost := c.CostOf(card); pl.Debt == 0 && cost.Coins <= pl.Coins && cost.Potion <= pl.Potions {
					buyable = append(buyable, engine.Option{Card: card, Inst: -1})
				}
			}
			if o := c.ChooseOne(c.P, engine.ToGain, buyable, true); o != nil {
				cost := c.CostOf(o.Card)
				pl.Coins -= cost.Coins
				pl.Potions -= cost.Potion
				pl.Debt += cost.Debt
				g.SetupCard(o.Card) // what it needs, once bought: a Cutthroat's Loot
				c.Gain(c.P, o.Card, discard)
				shown = slices.DeleteFunc(shown, func(card engine.CardID) bool { return card == o.Card })
			}
			if len(shown) > 0 {
				c.Log("PUT_ON_BOTTOM_OF", engine.Seat(c.P), shown, "the Black Market Deck")
				g.BlackMarket = append(slices.Clone(shown), g.BlackMarket...)
			}
		},
	})
	reg("Captain", rules{
		Play: func(c *ctx) { // "plays a Swamp Shacks.", where it is; next turn "plays a Ranger. (Captain)"
			captainPlays(c, false)
			c.Stay()
		},
		Next: func(c *ctx, _ *engine.Dur) bool {
			captainPlays(c, true)
			return true
		},
	})
	reg("Church", rules{
		Play: func(c *ctx) { // "sets 2 Coppers aside with Church."; next turn "puts 2 Coppers in hand (Church).", "trashes an Estate. (Church)"
			c.PlusActions(1)
			xs := setAsideWith(c, c.P, pick(c, c.P, engine.ToSetAside, hand, nil, 0, 3))
			c.Stay().Aside = xs
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			if xs := c.In(aside, d.Aside); len(xs) > 0 {
				c.ToHand(c.P, xs, "PUT_IN_HAND_FROM", engine.Name(c.Src))
			}
			c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, true)))
			return true
		},
	})
	reg("Dismantle", rules{Play: func(c *ctx) { // "trashes a Silver.", "gains an Estate.", "gains a Gold."
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		card := c.G.CardOf(x)
		c.Trash(c.P, one(x))
		if c.CostOf(card).Coins >= 1 {
			c.ChooseGain(c.P, cheaperThan(c, card), discard)
			c.Gain(c.P, c.Card("Gold"), discard)
		}
	}})
	reg("Envoy", rules{Play: func(c *ctx) { // "reveals 5 cards.", the other player picks: "discards a Gold.", "puts 4 cards into their hand."
		xs := c.FromTop(c.P, 5, "REVEAL")
		if len(xs) == 0 {
			return
		}
		x := pickOf(c, c.Other(), engine.ToDiscard, xs, 1, 1)
		c.Discard(c.P, x)
		c.ToHand(c.P, c.In(revealed, xs), "PUT_IN_HAND")
	}})
	reg("Governor", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		o := c.As(c.Other())
		switch c.Mode1(c.P, mode("+3 Cards, the other +1", "DRAW"), mode("gain a Gold, the other a Silver", "GAIN"), mode("trash and gain $2 more, the other $1", "TRASH")) {
		case 0: // "draws 3 cards.", the other "draws a card."
			c.Draw(3)
			o.Draw(1)
		case 1:
			c.Gain(c.P, c.Card("Gold"), discard)
			o.Gain(o.P, c.Card("Silver"), discard)
		case 2: // "trashes a Curse.", "gains a Squire."; the other "trashes a Squire." and, a line down, "gains a Torturer."
			governorRemodel(c, 2)
			governorRemodel(o.Deeper(), 1)
		}
	}})
	reg("Marchland", rules{
		VP: func(g *engine.Game, p int) int { return g.Count(p, victory) / 3 },
		OnGain: func(c *ctx, e *ev) { // "gets +1 Buy.", "discards an Estate.", "gets +$1."
			a := c.As(e.P)
			a.PlusBuys(1)
			if xs := a.Discard(e.P, pick(a, e.P, engine.ToDiscard, hand, nil, 0, len(c.Hand(e.P)))); len(xs) > 0 {
				a.PlusCoins(len(xs))
			}
		},
	})
	reg("Prince", rules{
		Play: func(c *ctx) { // "sets a Courtyard aside with Prince."; each turn "plays a Courtyard. (Prince)"
			ok := and(action, not(duration), not(func(d *def) bool { return d.Is(engine.TCommand) }), c.CostsUpTo(4))
			if x := pick1(c, c.P, engine.ToSetAside, hand, ok, true); x >= 0 && len(c.SetAsideWith(c.P, x)) == 1 {
				c.Stay().Aside = []inst{x}
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			xs := c.In(aside, d.Aside)
			for _, x := range xs {
				c.PlayWith(x)
			}
			return len(xs) == 0
		},
		Hears: engine.On(engine.EvDiscard),
		Hear: func(c *ctx, e *ev) { // discarded from play in Clean-up, it goes back aside, unlogged
			if !e.Cleanup || e.From != inPlay {
				return
			}
			for _, d := range c.Durs() {
				if slices.Contains(d.Aside, e.X) && c.G.Where(e.X).Zone == discard {
					c.G.Move(e.X, engine.Loc{Zone: aside, Owner: int16(e.P)})
				}
			}
		},
	})
	reg("Sauna", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			playFromHand(c, "Avanto")
			c.AddWatch(engine.On(engine.EvPlay), 0)
		},
		Watch: func(c *ctx, e *ev, _ *engine.Watch) bool { // this turn, each Silver played may trash a card from hand
			if e.P == c.P && c.G.D(e.C).Name == "Silver" {
				c.Here().Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, true)))
			}
			return false
		},
	})
	reg("Avanto", rules{Play: func(c *ctx) {
		c.Draw(3)
		playFromHand(c, "Sauna")
	}})
	reg("Stash", rules{Coins: 2})
	reg("Summon", rules{
		Buy: func(c *ctx) { // "gains a Royal Galley.", "sets a Royal Galley aside."; next turn "plays a Royal Galley. (Summon)"
			x := c.ChooseGain(c.P, and(action, c.CostsUpTo(4)), discard)
			if x < 0 || len(c.In(discard, one(x))) == 0 {
				return
			}
			if xs := c.SetAside(c.P, one(x)); len(xs) == 1 {
				c.Later().Aside = xs
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			for _, x := range c.In(aside, d.Aside) {
				c.PlayWith(x)
			}
			return true
		},
	})
	reg("Walled Village", rules{
		Play:  func(c *ctx) { c.Draw(1); c.PlusActions(2) },
		Hears: engine.On(engine.EvCleanup),
		Hear: func(c *ctx, e *ev) { // "begins their Cleanup phase.", "topdecks a Walled Village."
			if e.P != c.P || c.Me < 0 || len(c.In(inPlay, one(c.Me))) == 0 || c.Count(c.P, inPlay, action) > 2 {
				return
			}
			if c.Yes(c.P, "put this onto your deck", "TOPDECK") {
				c.Here().Topdeck(c.P, one(c.Me))
			}
		},
	})
}

// captainPlays plays a non-Duration non-Command Action from the Supply costing
// up to $4, leaving it there; at the start of a turn it names Captain.
func captainPlays(c *ctx, named bool) {
	ok := and(action, not(duration), not(func(d *def) bool { return d.Is(engine.TCommand) }), c.CostsUpTo(4))
	if o := c.ChooseOne(c.P, engine.ToPlay, c.SupplyOptions(ok), false); o != nil {
		if x := topOf(c, o.Card); x >= 0 {
			c.PlayThere(x, named)
		}
	}
}

// governorRemodel: may trash a card from hand to gain one costing exactly $n more.
func governorRemodel(c *ctx, n int) {
	x := pick1(c, c.P, engine.ToTrash, hand, nil, true)
	if x < 0 {
		return
	}
	ok := costExactly(c, x, n)
	c.Trash(c.P, one(x))
	c.ChooseGain(c.P, ok, discard)
}
