package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Hinterlands, second edition.
func init() {
	reg("Berserker", rules{
		Play: func(c *ctx) {
			c.ChooseGain(c.P, cheaperThan(c, c.Src), discard)
			c.Attack(func(v int) {
				if n := len(c.Hand(v)) - 3; n > 0 {
					c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, n, n))
				}
			})
		},
		OnGain: func(c *ctx, e *ev) {
			if c.Count(c.P, inPlay, action) > 0 && stillGained(c, e) {
				e.Done = true
				c.PlayNow(c.Me)
			}
		},
	})
	reg("Border Village", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(2)
		},
		OnGain: func(c *ctx, e *ev) { c.ChooseGain(c.P, cheaperThan(c, c.Src), discard) },
	})
	reg("Cartographer", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		xs := c.FromTop(c.P, 4, "LOOK_AT")
		c.Discard(c.P, pickOf(c, c.P, engine.ToDiscard, xs, 0, len(xs)))
		topdeckAny(c, c.P, c.In(revealed, xs))
	}})
	reg("Cauldron", rules{Coins: 2,
		Play: func(c *ctx) {
			c.PlusBuys(1)
			w := c.AddWatch(engine.On(engine.EvGain), 0)
			if c.Unaffected(c.Other()) {
				w.N = 1
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P || !c.G.D(e.C).Action() {
				return false
			}
			n := 0
			for _, g := range c.T2().Gained {
				if c.G.D(g).Action() {
					n++
				}
			}
			if n == 3 && w.N == 0 {
				c.Gain(c.Other(), c.Card("Curse"), discard)
			}
			return n >= 3
		},
	})
	reg("Crossroads", rules{Play: func(c *ctx) {
		c.RevealHand(c.P)
		if n := c.Count(c.P, hand, victory); n > 0 {
			c.Draw(n)
		}
		if countPlayed(c, c.Src) == 1 {
			c.PlusActions(3)
		}
	}})
	reg("Develop", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		more, less := costExactly(c, x, 1), costExactly(c, x, -1)
		c.Trash(c.P, one(x))
		if c.Mode1(c.P, mode("the costlier one first", "GAIN"), mode("the cheaper one first", "GAIN")) == 1 {
			more, less = less, more
		}
		c.ChooseGain(c.P, more, deck)
		c.ChooseGain(c.P, less, deck)
	}})
	reg("Farmland", rules{VP: worth(2), OnGain: func(c *ctx, e *ev) {
		if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
			up := and(costExactly(c, x, 2), not(named("Farmland")))
			c.Trash(c.P, one(x))
			c.ChooseGain(c.P, up, discard)
		}
	}})
	reg("Fool's Gold", rules{
		Worth: func(g *engine.Game, p int) int {
			for _, x := range g.T2.Played {
				if g.Def(x).Name == "Fool's Gold" {
					return 4
				}
			}
			return 1
		},
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return e.P != c.P && c.G.D(e.C).Name == "Province" },
		React: func(c *ctx, e *ev) bool { // "trashes a Fool's Gold.", "gains a Gold."
			if len(c.Trash(c.P, one(c.Me))) == 1 {
				c.Gain(c.P, c.Card("Gold"), deck)
			}
			return true
		},
	})
	reg("Guard Dog", rules{
		Play: func(c *ctx) {
			c.Draw(2)
			if len(c.Hand(c.P)) <= 5 {
				c.Draw(2)
			}
		},
		Reacts:   engine.On(engine.EvAttack),
		CanReact: attacked,
		React: func(c *ctx, e *ev) bool {
			c.Log("REACTS_PLAYING", engine.Seat(c.P), []engine.CardID{c.Src})
			c.PlayNow(c.Me)
			return true
		},
	})
	reg("Haggler", rules{
		Play: func(c *ctx) { c.PlusCoins(2); c.AddWatch(engine.On(engine.EvGain), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // this turn, for each bought card gained
			if e.P == w.P && e.Bought {
				c.ChooseGain(w.P, and(not(victory), cheaperThan(c, e.C)), discard)
			}
			return false
		},
	})
	reg("Highway", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.T2().Reduce++
		},
	})
	reg("Inn", rules{
		Play: func(c *ctx) {
			c.Draw(2)
			c.PlusActions(2)
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, min(2, len(c.Hand(c.P))), 2))
		},
		OnGain: func(c *ctx, e *ev) { // "looks at an Estate and an Inn.", "shuffles their deck.", "shuffles an Inn into their deck."
			c.LookAt(c.P, c.Zone(c.P, discard))
			if xs := pick(c, c.P, engine.ToPick, discard, action, 0, len(c.Zone(c.P, discard))); len(xs) > 0 {
				c.ShuffleLine(c.P)
				c.ShuffleInto(c.P, xs)
			}
		},
	})
	reg("Jack of All Trades", rules{Play: func(c *ctx) {
		c.Gain(c.P, c.Card("Silver"), discard)
		if xs := c.FromTop(c.P, 1, "LOOK_AT"); len(xs) == 1 {
			if c.Mode1(c.P, mode("discard it", "DISCARD"), mode("put it back", "")) == 0 {
				c.Discard(c.P, xs)
			} else {
				c.Topdeck(c.P, xs)
			}
		}
		drawTo(c, 5)
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, not(treasure), 0, 1))
	}})
	reg("Margrave", rules{Play: func(c *ctx) {
		c.Draw(3)
		c.PlusBuys(1)
		c.Attack(func(v int) {
			c.DrawFor(v, 1)
			if n := len(c.Hand(v)) - 3; n > 0 {
				c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, n, n))
			}
		})
	}})
	// nomadsCoins: "gets +$2." under its gain or trash; at the start of a turn
	// (an Importer's gain) it names itself, "gets +$2. (Nomads)".
	nomadsCoins := func(c *ctx) {
		if c.T2().Phase == engine.StartPhase {
			c.Away().PlusCoins(2)
		} else {
			c.PlusCoins(2)
		}
	}
	reg("Nomads", rules{
		Play: func(c *ctx) {
			c.PlusBuys(1)
			c.PlusCoins(2)
		},
		OnGain:  func(c *ctx, e *ev) { nomadsCoins(c) },
		OnTrash: func(c *ctx, e *ev) { nomadsCoins(c) },
	})
	reg("Oasis", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusCoins(1)
		c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 1, 1))
	}})
	reg("Scheme", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.AddWatch(engine.On(engine.EvCleanup, engine.EvDiscard), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			switch {
			case e.Kind == engine.EvCleanup && e.P == w.P && w.N == 0:
				var xs []inst // each Scheme picks a different card
				for _, x := range cards(c.Options(w.P, inPlay, action)) {
					taken := false
					for _, o := range c.G.Watches() {
						taken = taken || o.C == w.C && o.N == int(x)+1
					}
					if !taken {
						xs = append(xs, x)
					}
				}
				if got := pickOf(c, w.P, engine.ToPick, xs, 0, 1); len(got) == 1 {
					w.N = int(got[0]) + 1
				}
				return w.N == 0
			case e.Kind == engine.EvDiscard && e.Cleanup && w.N == int(e.X)+1:
				if c.G.Where(e.X) == (engine.Loc{Zone: discard, Owner: int16(w.P)}) { // a Treasury may have topdecked itself
					c.Topdeck(w.P, one(e.X))
				}
				return true
			}
			return false
		},
	})
	reg("Souk", rules{
		Play: func(c *ctx) {
			c.PlusBuys(1)
			c.PlusCoins(7)
			c.LoseCoins(len(c.Hand(c.P)))
		},
		OnGain: func(c *ctx, e *ev) { c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 2)) },
	})
	reg("Spice Merchant", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToTrash, hand, treasure, true); x >= 0 {
			c.Trash(c.P, one(x))
			if c.Mode1(c.P, mode("+2 Cards +1 Action", "DRAW"), mode("+1 Buy +$2", "GETS_BUY")) == 0 {
				c.Draw(2)
				c.PlusActions(1)
			} else {
				c.PlusBuys(1)
				c.PlusCoins(2)
			}
		}
	}})
	reg("Stables", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToDiscard, hand, treasure, true); x >= 0 {
			c.Discard(c.P, one(x))
			c.Draw(3)
			c.PlusActions(1)
		}
	}})
	reg("Trader", rules{
		Play: func(c *ctx) {
			if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
				n := c.CostOf(c.G.CardOf(x)).Coins
				c.Trash(c.P, one(x))
				c.GainN(c.P, c.Card("Silver"), n, discard, "GAIN")
			}
		},
		Reacts: engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool {
			return e.P == c.P && c.G.D(e.C).Name != "Silver" && stillGained(c, e) && c.G.PileOf(e.C) >= 0
		},
		React: func(c *ctx, e *ev) bool {
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			if c.ReturnTo(c.P, e.X) {
				e.Done = true
				c.Gain(c.P, c.Card("Silver"), discard)
			}
			return true
		},
	})
	reg("Trail", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
		},
		OnGain:    func(c *ctx, e *ev) { playTrail(c, e) },
		OnTrash:   func(c *ctx, e *ev) { playTrail(c, e) },
		OnDiscard: func(c *ctx, e *ev) { playTrail(c, e) },
	})
	reg("Tunnel", rules{VP: worth(2), OnDiscard: func(c *ctx, e *ev) {
		if !e.Cleanup && c.Yes(c.P, "reveal it to gain a Gold", "REACTS_WITH") {
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			c.Reveal(c.P, one(c.Me))
			c.Gain(c.P, c.Card("Gold"), discard)
		}
	}})
	reg("Weaver", rules{
		Play: func(c *ctx) {
			if c.Mode1(c.P, engine.Mode{Label: "gain two Silvers", Line: "GAIN", Card: c.Card("Silver")}, mode("gain a card costing up to $4", "GAIN")) == 0 { // "gains a Sauna." is the second
				c.GainN(c.P, c.Card("Silver"), 2, discard, "GAIN")
				return
			}
			c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		},
		OnDiscard: func(c *ctx, e *ev) {
			if !e.Cleanup && c.G.Where(c.Me).Zone == discard && c.Yes(c.P, "play it", "PLAY") {
				c.PlayNow(c.Me)
			}
		},
	})
	reg("Wheelwright", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		if x := pick1(c, c.P, engine.ToDiscard, hand, nil, true); x >= 0 {
			up := c.CostsAtMost(c.CostOf(c.G.CardOf(x)))
			c.Discard(c.P, one(x))
			c.ChooseGain(c.P, and(action, up), discard)
		}
	}})
	reg("Witch's Hut", rules{Play: func(c *ctx) {
		c.Draw(4)
		xs := c.DiscardAs(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, min(2, len(c.Hand(c.P))), 2), "DISCARD_REVEALED")
		if len(xs) == 2 && c.G.Def(xs[0]).Action() && c.G.Def(xs[1]).Action() {
			c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
		}
	}})
}

// cheaperThan tests for a card costing less than another card.
func cheaperThan(c *ctx, card engine.CardID) func(*def) bool {
	cost := c.CostOf(card)
	return func(d *def) bool { return c.CostOf(d.ID).Less(cost) }
}

// countPlayed counts the copies of a card played this turn, this one included.
func countPlayed(c *ctx, card engine.CardID) int {
	n := 0
	for _, x := range c.T2().Played {
		if c.G.CardOf(x) == card {
			n++
		}
	}
	return n
}

// nobleBrigand is Noble Brigand's attack on v, when played or bought.
func nobleBrigand(c *ctx, v int) {
	xs := c.FromTop(v, 2, "REVEAL")
	var sg []inst
	treasures := false
	for _, x := range xs {
		d := c.G.Def(x)
		treasures = treasures || d.Treasure()
		if silverOrGold(d) {
			sg = append(sg, x)
		}
	}
	t := pickOf(c, c.P, engine.ToTrash, sg, 1, 1)
	c.Trash(v, t)
	c.Discard(v, c.In(revealed, xs))
	if !treasures {
		c.Gain(v, c.Card("Copper"), discard)
	}
	for _, x := range t {
		if c.G.Where(x).Zone == engine.Trash {
			c.GainInst(c.P, x, discard, "GAIN")
		}
	}
}

// playTrail plays Trail when it is gained, trashed or discarded other than in Clean-up.
func playTrail(c *ctx, e *ev) {
	if e.Cleanup || e.Done || !c.Yes(c.P, "play it", "REACTS_PLAYING") {
		return
	}
	e.Done = true
	c.PlayAs(c.Me, "REACTS_PLAYING")
}
