package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Prosperity, second edition. Platinum and Colony are in base.go.
func init() {
	reg("Anvil", rules{Coins: 1, Alone: true, Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToDiscard, hand, treasure, true); x >= 0 {
			c.Discard(c.P, one(x))
			c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		}
	}})
	reg("Bank", rules{Worth: func(g *engine.Game, p int) int {
		n := 0
		for _, x := range g.P[p].Z[inPlay] {
			if g.Def(x).Treasure() {
				n++
			}
		}
		return n
	}})
	reg("Bishop", rules{Play: func(c *ctx) {
		c.PlusCoins(1)
		c.PlusVP(1)
		if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
			n := c.CostOf(c.G.CardOf(x)).Coins / 2
			c.Trash(c.P, one(x))
			if n > 0 {
				c.PlusVP(n)
			}
		}
		c.Trash(c.Other(), pick(c, c.Other(), engine.ToTrash, hand, nil, 0, 1))
	}})
	reg("Charlatan", rules{
		Setup: func(g *engine.Game) { g.CurseTreasure = true },
		Play: func(c *ctx) {
			c.PlusCoins(3)
			c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
		},
	})
	reg("City", rules{Play: func(c *ctx) { // "draws a Snowy Village.", "gets +2 Actions.", then with an empty pile "draws a Copper."
		c.Draw(1)
		c.PlusActions(2)
		if c.G.EmptyPiles() >= 1 {
			c.Draw(1)
		}
		if c.G.EmptyPiles() >= 2 {
			c.PlusCoins(1)
			c.PlusBuys(1)
		}
	}})
	reg("Clerk", rules{
		Play: func(c *ctx) {
			c.PlusCoins(2)
			c.Attack(func(v int) {
				if len(c.Hand(v)) >= 5 {
					c.Topdeck(v, pick(c, v, engine.ToTopdeck, hand, nil, 1, 1))
				}
			})
		},
		Reacts:   engine.On(engine.EvTurnStart),
		CanReact: func(c *ctx, e *ev) bool { return e.P == c.P },
		React: func(c *ctx, e *ev) bool {
			c.PlayAs(c.Me, "REACTS_PLAYING")
			return true
		},
	})
	reg("Collection", rules{Coins: 2,
		Play: func(c *ctx) {
			c.PlusBuys(1)
			for i := range c.G.Watches() {
				if w := &c.G.Watches()[i]; w.P == c.P && w.C == c.Src {
					w.N++
					return
				}
			}
			c.AddWatch(engine.On(engine.EvGain), 1)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P == w.P && c.G.D(e.C).Action() {
				c.Away().PlusVP(w.N)
			}
			return false
		},
	})
	reg("Crystal Ball", rules{Coins: 1, Alone: true, Play: func(c *ctx) {
		xs := c.FromTop(c.P, 1, "LOOK_AT")
		if len(xs) == 0 {
			return
		}
		x := xs[0]
		ms := []engine.Mode{mode("trash it", "TRASH"), mode("discard it", "DISCARD"), mode("put it back", "")}
		if d := c.G.Def(x); d.Action() || d.Treasure() {
			ms = append(ms, mode("play it", "PLAY"))
		}
		switch c.Mode1(c.P, ms...) {
		case 0:
			c.Trash(c.P, xs)
		case 1:
			c.Discard(c.P, xs)
		case 2:
			c.MoveTo(c.P, xs, deck, "")
		case 3:
			c.PlayNow(x)
		}
	}})
	reg("Expand", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
			up := costUp(c, x, 3)
			c.Trash(c.P, one(x))
			c.ChooseGain(c.P, up, discard)
		}
	}})
	reg("Forge", rules{Play: func(c *ctx) {
		xs := pick(c, c.P, engine.ToTrash, hand, nil, 0, len(c.Hand(c.P)))
		total := 0
		for _, x := range xs {
			total += c.CostOf(c.G.CardOf(x)).Coins
		}
		c.Trash(c.P, xs)
		c.ChooseGain(c.P, func(d *def) bool { return c.CostOf(d.ID) == engine.Upto(total) }, discard)
	}})
	reg("Grand Market", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.PlusBuys(1)
			c.PlusCoins(2)
		},
		CanBuy: func(g *engine.Game, p int) bool {
			for _, x := range g.P[p].Z[inPlay] {
				if g.Def(x).Name == "Copper" {
					return false
				}
			}
			return true
		},
	})
	reg("Hoard", rules{Coins: 2,
		Play: func(c *ctx) { c.AddWatch(engine.On(engine.EvGain), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // this turn, for each bought Victory card gained
			if e.P == w.P && e.Bought && c.G.D(e.C).Victory() {
				c.Gain(w.P, c.Card("Gold"), discard)
			}
			return false
		},
	})
	reg("Investment", rules{Alone: true, Play: func(c *ctx) {
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 1, 1))
		if c.Mode1(c.P, mode("+$1", "GETS_COIN_FROM"), mode("trash this for VP", "TRASH")) == 0 {
			c.CoinsFrom(1)
			return
		}
		if len(c.Trash(c.P, one(c.Me))) == 0 {
			return
		}
		c.RevealHand(c.P)
		seen := map[engine.CardID]bool{}
		for _, x := range c.Hand(c.P) {
			if c.G.Def(x).Treasure() {
				seen[c.G.CardOf(x)] = true
			}
		}
		if len(seen) > 0 {
			c.PlusVP(len(seen))
		}
	}})
	reg("King's Court", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToPlay, hand, action, true)
		if x < 0 {
			return
		}
		c.PlayCard(x, 0)
		c.PlayCard(x, 1)
		c.PlayCard(x, 2)
	}})
	reg("Magnate", rules{Play: func(c *ctx) {
		c.RevealHand(c.P)
		if n := c.Count(c.P, hand, treasure); n > 0 {
			c.Draw(n)
		}
	}})
	reg("Mint", rules{
		Play: func(c *ctx) {
			if x := pick1(c, c.P, engine.ToReveal, hand, treasure, true); x >= 0 {
				c.Reveal(c.P, one(x))
				c.Gain(c.P, c.G.CardOf(x), discard)
			}
		},
		OnGain: func(c *ctx, e *ev) {
			c.Trash(c.P, cards(c.Options(c.P, inPlay, and(treasure, not(duration)))))
		},
	})
	reg("Monument", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		c.PlusVP(1)
	}})
	reg("Peddler", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.PlusCoins(1)
		},
		SelfCost: func(g *engine.Game, p int, cost *engine.Cost) {
			if g.T2.Phase != engine.BuyPhase {
				return
			}
			for _, x := range g.P[p].Z[inPlay] {
				if g.Def(x).Action() {
					cost.Coins -= 2
				}
			}
			cost.Coins = max(0, cost.Coins)
		},
	})
	reg("Quarry", rules{Coins: 1, Play: func(c *ctx) { c.T2().ReduceAct += 2 }})
	reg("Rabble", rules{Play: func(c *ctx) {
		c.Draw(3)
		c.Attack(func(v int) {
			xs := c.FromTop(v, 3, "REVEAL")
			var out []inst
			for _, x := range xs {
				if d := c.G.Def(x); d.Action() || d.Treasure() {
					out = append(out, x)
				}
			}
			c.Discard(v, out)
			topdeckAny(c, v, c.In(revealed, xs))
		})
	}})
	reg("Tiara", rules{Alone: true, Play: func(c *ctx) {
		c.PlusBuys(1)
		has := false
		for _, w := range c.G.Watches() {
			has = has || w.P == c.P && w.C == c.Src
		}
		if !has {
			c.AddWatch(engine.On(engine.EvGain), 0)
		}
		if x := pick1(c, c.P, engine.ToPlay, hand, treasure, true); x >= 0 {
			c.PlayCard(x, 0)
			c.PlayCard(x, 1)
		}
	}, Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
		if e.P == w.P && stillGained(c, e) && c.Yes(w.P, "put it onto your deck", "TOPDECK_WITH") { // "topdecks a Courtyard with Tiara."
			c.MoveTo(w.P, one(e.X), deck, "TOPDECK_WITH", engine.Name(c.Src))
			e.Done = true
		}
		return false
	}})
	reg("Vault", rules{Play: func(c *ctx) {
		c.Draw(2)
		if n := len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 0, len(c.Hand(c.P))))); n > 0 {
			c.PlusCoins(n)
		}
		v := c.Other()
		if len(c.Hand(v)) >= 2 && c.Yes(v, "discard 2 cards to draw 1", "DISCARD") {
			c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, 2, 2))
			c.DrawFor(v, 1)
		}
	}})
	reg("War Chest", rules{Alone: true, Play: func(c *ctx) {
		named := c.NameCard(c.Other())
		c.Log("NAMES", engine.Seat(c.Other()), engine.Name(named))
		c.T2().Named = append(c.T2().Named, named)
		c.ChooseGain(c.P, func(d *def) bool {
			for _, n := range c.T2().Named {
				if n == d.ID {
					return false
				}
			}
			return c.CostOf(d.ID).LE(engine.Upto(5))
		}, discard)
	}})
	reg("Watchtower", rules{
		Play:     func(c *ctx) { drawTo(c, 6) },
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return e.P == c.P && stillGained(c, e) },
		React: func(c *ctx, e *ev) bool {
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			if c.Mode1(c.P, mode("trash it", "TRASH"), mode("put it onto your deck", "TOPDECK")) == 0 {
				c.Trash(c.P, one(e.X))
			} else {
				c.Topdeck(c.P, one(e.X))
			}
			e.Done = true
			return true
		},
	})
	reg("Worker's Village", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(2)
		c.PlusBuys(1)
	}})
}

// stillGained: the gained card is where the gain put it, so a "when you gain" effect can move it.
func stillGained(c *ctx, e *ev) bool {
	return !e.Done && c.G.Where(e.X) == engine.Loc{Zone: e.To, Owner: int16(e.P)}
}
