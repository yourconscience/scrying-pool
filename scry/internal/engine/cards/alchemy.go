package cards

import (
	"slices"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Alchemy: Potions. Possession is not here yet: it needs the possessor to
// decide for the possessed player and to take what they gain.

func init() {
	reg("Alchemist", rules{
		Play: func(c *ctx) {
			c.Draw(2)
			c.PlusActions(1)
			c.AddWatch(engine.On(engine.EvCleanup), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // at the start of Clean-up
			if e.P == w.P && c.G.Where(c.Me) == (engine.Loc{Zone: inPlay, Owner: int16(w.P)}) &&
				c.Count(w.P, inPlay, named("Potion")) > 0 && c.Yes(w.P, "put it on your deck", "TOPDECK") {
				c.Topdeck(w.P, one(c.Me))
			}
			return true
		},
	})
	reg("Apothecary", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 4, "REVEAL")
		var take []inst
		for _, x := range top {
			if n := c.G.Def(x).Name; n == "Copper" || n == "Potion" {
				take = append(take, x)
			}
		}
		c.ToHand(c.P, take, "PUT_IN_HAND")
		topdeckAny(c, c.P, c.In(revealed, top))
	}})
	reg("Apprentice", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		cost := c.CostOf(c.G.CardOf(x))
		c.Trash(c.P, one(x))
		n := cost.Coins
		if cost.Potion > 0 {
			n += 2
		}
		c.Draw(n)
	}})
	reg("Familiar", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
	}})
	reg("Golem", rules{Play: func(c *ctx) {
		all, found := c.RevealUntil(c.P, func(d *def) bool { return d.Action() && d.Name != "Golem" }, 2)
		var rest []inst
		for _, x := range all {
			if !slices.Contains(found, x) {
				rest = append(rest, x)
			}
		}
		c.Discard(c.P, rest)
		if len(found) == 2 { // in either order
			found = pickOf(c, c.P, engine.ToPlay, found, 2, 2)
		}
		for _, x := range found {
			if len(c.In(revealed, one(x))) == 1 {
				c.PlayNow(x)
			}
		}
	}})
	reg("Herbalist", rules{
		Play: func(c *ctx) {
			c.PlusBuys(1)
			c.PlusCoins(1)
			c.AddWatch(engine.On(engine.EvDiscard), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // once this turn, a Treasure discarded from play
			if e.P != w.P || e.From != inPlay || !c.G.D(e.C).Treasure() || c.G.Where(e.X) != (engine.Loc{Zone: discard, Owner: int16(w.P)}) ||
				!c.Yes(w.P, "put it on your deck", "TOPDECK") {
				return false
			}
			c.Topdeck(w.P, one(e.X))
			return true
		},
	})
	reg("Philosopher's Stone", rules{Worth: func(g *engine.Game, p int) int {
		return (len(g.P[p].Z[deck]) + len(g.P[p].Z[discard])) / 5
	}})
	reg("Scrying Pool", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		look := func(v int) { // the player who played it chooses for each
			x := c.FromTop(v, 1, "REVEAL")
			if len(x) == 0 {
				return
			}
			if c.Yes(c.P, "discard it", "DISCARD") {
				c.Discard(v, x)
			} else {
				c.Topdeck(v, x)
			}
		}
		look(c.P)
		c.Attack(look)
		all, _ := c.RevealUntil(c.P, not(action), 1)
		c.ToHand(c.P, all, "PUT_IN_HAND")
	}})
	reg("Transmute", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		d := c.G.Def(x)
		c.Trash(c.P, one(x))
		if d.Action() {
			c.Gain(c.P, c.Card("Duchy"), discard)
		}
		if d.Treasure() {
			c.Gain(c.P, c.Card("Transmute"), discard)
		}
		if d.Victory() {
			c.Gain(c.P, c.Card("Gold"), discard)
		}
	}})
	reg("University", rules{Play: func(c *ctx) {
		c.PlusActions(2)
		if o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(and(action, c.CostsUpTo(5))), true); o != nil {
			c.Gain(c.P, o.Card, discard)
		}
	}})
	reg("Vineyard", rules{VP: func(g *engine.Game, p int) int {
		n := 0
		for _, x := range g.Owned(p) {
			if g.Def(x).Action() {
				n++
			}
		}
		return n / 3
	}})
}
