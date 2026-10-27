package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Cornucopia & Guilds, second edition: Rewards, Coffers and overpaying.

var rewards = []string{"Coronet", "Courser", "Demesne", "Housecarl", "Huge Turnip", "Renown"}

// needAll adds a non-Supply pile of n of each card.
func needAll(names []string, n int) func(*engine.Game) {
	return func(g *engine.Game) {
		for _, name := range names {
			g.Need(name, n)
		}
	}
}

// pileOptions are the cards of the named piles that are left.
func pileOptions(c *ctx, names []string) []engine.Option {
	var out []engine.Option
	for _, n := range names {
		if id := c.Card(n); c.G.Left(id) > 0 {
			out = append(out, engine.Option{Card: id, Inst: -1})
		}
	}
	return out
}

// distinct counts the differently named cards.
func distinct(c *ctx, xs []inst) int {
	seen := map[engine.CardID]bool{}
	for _, x := range xs {
		seen[c.G.CardOf(x)] = true
	}
	return len(seen)
}

// bane: a card of Young Witch's Bane pile.
func bane(c *ctx) func(*def) bool {
	return func(d *def) bool {
		i := c.G.PileOf(d.ID)
		return i >= 0 && c.G.Piles[i].Bane && c.G.Piles[i].Supply
	}
}

func costsCoins(ns ...int) func(*def) bool {
	return func(d *def) bool {
		for _, n := range ns {
			if d.Cost == engine.Upto(n) {
				return true
			}
		}
		return false
	}
}

func usesCoffers(c *ctx, n int) {
	c.Pl(c.P).Coffers -= n
	if n == 1 {
		c.Log("USES_COFFER", engine.Seat(c.P), n)
	} else {
		c.Log("USES_COFFERS", engine.Seat(c.P), n)
	}
}

// revealAndTake reveals until n cards pass a test, takes them into hand and discards the rest.
func revealAndTake(c *ctx, ok func(*def) bool, n int) {
	all, found := c.RevealUntil(c.P, ok, n)
	c.ToHand(c.P, found, "PUT_IN_HAND")
	c.Discard(c.P, c.In(revealed, all))
}

func init() {
	reg("Carnival", rules{Play: func(c *ctx) {
		xs := c.FromTop(c.P, 4, "REVEAL")
		seen := map[engine.CardID]bool{}
		var keep []inst
		for _, x := range xs {
			if id := c.G.CardOf(x); !seen[id] {
				seen[id] = true
				keep = append(keep, x)
			}
		}
		c.ToHand(c.P, keep, "PUT_IN_HAND")
		c.Discard(c.P, c.In(revealed, xs))
	}})
	reg("Farmhands", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(2)
		},
		OnGain: func(c *ctx, e *ev) {
			if x := pick1(c, c.P, engine.ToSetAside, hand, func(d *def) bool { return d.Action() || d.Treasure() }, true); x >= 0 {
				c.SetAside(c.P, one(x))
				c.Later().Aside = one(x)
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			for _, x := range c.In(aside, d.Aside) {
				c.PlayWith(x)
			}
			return true
		},
	})
	reg("Ferryman", rules{
		Setup: func(g *engine.Game) {
			if us := g.Unused(costsCoins(3, 4)); len(us) > 0 {
				g.AddBane(us[g.Rand(len(us))], false)
			}
		},
		Play: func(c *ctx) {
			c.Draw(2)
			c.PlusActions(1)
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, min(1, len(c.Hand(c.P))), 1))
		},
		OnGain: func(c *ctx, e *ev) {
			for i := range c.G.Piles {
				if pile := &c.G.Piles[i]; pile.Bane && !pile.Supply && c.G.Top(i) != 0 {
					c.Gain(c.P, c.G.Top(i), discard)
				}
			}
		},
	})
	reg("Hamlet", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		if len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 0, 1))) > 0 {
			c.PlusActions(1)
		}
		if len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 0, 1))) > 0 {
			c.PlusBuys(1)
		}
	}})
	reg("Horn of Plenty", rules{Alone: true, Play: func(c *ctx) {
		x := c.ChooseGain(c.P, c.CostsUpTo(distinct(c, c.Zone(c.P, inPlay))), discard)
		if x >= 0 && c.G.Def(x).Victory() && c.G.Where(c.Me).Zone == inPlay {
			c.Trash(c.P, one(c.Me))
		}
	}})
	reg("Hunting Party", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.RevealHand(c.P)
		inHand := map[engine.CardID]bool{}
		for _, x := range c.Hand(c.P) {
			inHand[c.G.CardOf(x)] = true
		}
		revealAndTake(c, func(d *def) bool { return !inHand[d.ID] }, 1)
	}})
	reg("Jester", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		c.Attack(func(v int) {
			x := c.DiscardTop(v)
			if x < 0 {
				return
			}
			d := c.G.Def(x)
			switch {
			case d.Victory():
				c.Gain(v, c.Card("Curse"), discard)
			case c.G.Left(d.ID) == 0:
			case c.Mode1(c.P, mode("they gain a copy", "GAIN"), mode("you gain a copy", "GAIN")) == 0:
				c.Gain(v, d.ID, discard)
			default:
				c.Gain(c.P, d.ID, discard)
			}
		})
	}})
	reg("Joust", rules{
		Setup: needAll(rewards, 1), // one of each with 2 players
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.PlusCoins(1)
			x := pick1(c, c.P, engine.ToSetAside, hand, named("Province"), true)
			if x < 0 {
				return
			}
			c.SetAside(c.P, one(x))
			c.AddWatch(engine.On(engine.EvCleanup), int(x)+1)
			if o := c.ChooseOne(c.P, engine.ToGain, pileOptions(c, rewards), false); o != nil {
				c.GainAs(c.P, o.Card, hand, "GAIN")
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.Kind != engine.EvCleanup || e.P != w.P {
				return false
			}
			c.Discard(w.P, c.In(aside, one(inst(w.N-1)))) // the Province set aside
			return true
		},
	})
	reg("Menagerie", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		c.RevealHand(c.P)
		if h := c.Hand(c.P); distinct(c, h) == len(h) {
			c.Draw(3)
		} else {
			c.Draw(1)
		}
	}})
	reg("Remake", rules{Play: func(c *ctx) {
		for range 2 {
			x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
			if x < 0 {
				return
			}
			ok := costExactly(c, x, 1)
			c.Trash(c.P, one(x))
			c.ChooseGain(c.P, ok, discard)
		}
	}})
	reg("Shop", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusCoins(1)
		played := map[engine.CardID]bool{}
		for _, x := range c.Zone(c.P, inPlay) {
			played[c.G.CardOf(x)] = true
		}
		if x := pick1(c, c.P, engine.ToPlay, hand, func(d *def) bool { return d.Action() && !played[d.ID] }, true); x >= 0 {
			c.PlayNow(x)
		}
	}})
	reg("Young Witch", rules{
		Setup: func(g *engine.Game) {
			for _, pile := range g.Piles {
				if pile.Bane && pile.Supply {
					return // the replayed game's
				}
			}
			if us := g.Unused(costsCoins(2, 3)); len(us) > 0 {
				g.AddBane(us[g.Rand(len(us))], true)
			}
		},
		Play: func(c *ctx) {
			c.Draw(2)
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, min(2, len(c.Hand(c.P))), 2))
			c.Attack(func(v int) {
				if bs := c.Options(v, hand, bane(c)); len(bs) > 0 && c.Yes(v, "reveal a Bane", "REVEAL") {
					c.Reveal(v, one(bs[0].Inst))
					return
				}
				c.Gain(v, c.Card("Curse"), discard)
			})
		},
	})

	// Rewards
	reg("Coronet", rules{Alone: true, Play: func(c *ctx) {
		for _, kind := range []func(*def) bool{action, treasure} {
			if x := pick1(c, c.P, engine.ToPlay, hand, and(kind, not(engine.Of(engine.TReward))), true); x >= 0 {
				c.PlayCard(x, 0)
				c.PlayCard(x, 1)
			}
		}
	}})
	reg("Courser", rules{Play: func(c *ctx) {
		for _, m := range c.Modes(c.P, 2, 2, mode("+2 Cards", "DRAW"), mode("+2 Actions", "GETS_ACTIONS"), mode("+$2", "GETS_COINS"), mode("gain 4 Silvers", "GAIN")) {
			switch m {
			case 0:
				c.Draw(2)
			case 1:
				c.PlusActions(2)
			case 2:
				c.PlusCoins(2)
			case 3:
				c.GainN(c.P, c.Card("Silver"), 4, discard, "GAIN")
			}
		}
	}})
	reg("Demesne", rules{
		Play: func(c *ctx) {
			c.PlusActions(2)
			c.PlusBuys(2)
			c.Gain(c.P, c.Card("Gold"), discard)
		},
		VP: func(g *engine.Game, p int) int { return g.Count(p, named("Gold")) },
	})
	reg("Housecarl", rules{Play: func(c *ctx) {
		var acts []inst
		for _, x := range c.Zone(c.P, inPlay) {
			if c.G.Def(x).Action() {
				acts = append(acts, x)
			}
		}
		if n := distinct(c, acts); n > 0 {
			c.Draw(n)
		}
	}})
	reg("Huge Turnip", rules{Alone: true, Play: func(c *ctx) {
		c.PlusCoffers(2)
		c.CoinsFrom(c.Pl(c.P).Coffers)
	}})
	reg("Renown", rules{Play: func(c *ctx) {
		c.PlusBuys(1)
		c.T2().Reduce += 2
	}})

	// Guilds
	reg("Advisor", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		xs := c.FromTop(c.P, 3, "REVEAL")
		if len(xs) == 0 {
			return
		}
		c.Discard(c.P, pickOf(c, c.Other(), engine.ToDiscard, xs, 1, 1))
		c.ToHand(c.P, c.In(revealed, xs), "PUT_IN_HAND")
	}})
	reg("Baker", rules{
		Start: func(g *engine.Game, me engine.CardID) {
			for p := range g.P {
				g.P[p].Coffers++
				g.CtxFor(p, me, 0).Log("GETS_COFFER", engine.Seat(p), 1) // "gets +1 Coffers.", before the starting decks
			}
		},
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.PlusCoffers(1)
		},
	})
	reg("Butcher", rules{Play: func(c *ctx) {
		c.PlusCoffers(2)
		x := pick1(c, c.P, engine.ToTrash, hand, nil, true)
		if x < 0 {
			return
		}
		cost := c.CostOf(c.G.CardOf(x))
		c.Trash(c.P, one(x))
		n := c.Amount(c.P, 0, c.Pl(c.P).Coffers, "USES_COFFER", "USES_COFFERS")
		if n > 0 {
			usesCoffers(c, n)
		}
		c.ChooseGain(c.P, c.CostsAtMost(cost.Plus(n)), discard)
	}})
	reg("Candlestick Maker", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		c.PlusBuys(1)
		c.PlusCoffers(1)
	}})
	reg("Farrier", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.PlusBuys(1)
		},
		// +1 Card per $1 at the end of this turn: dominion.games draws them after
		// the next hand, "draws 7 cards." in Clean-up unless a shuffle comes between
		Overpay: func(c *ctx, n int) { c.T2().LateDraw += n },
	})
	reg("Footpad", rules{
		Setup: func(g *engine.Game) { g.Global = append(g.Global, g.T.MustCard("Footpad")) },
		Play: func(c *ctx) {
			c.PlusCoffers(2)
			c.Attack(func(v int) {
				if n := len(c.Hand(v)) - 3; n > 0 {
					c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, n, n))
				}
			})
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if c.Me < 0 && c.T2().Phase == engine.ActionPhase { // any player's gain in an Action phase
				c.As(e.P).Away().Draw(1)
			}
		},
	})
	reg("Herald", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			xs := c.FromTop(c.P, 1, "REVEAL")
			switch {
			case len(xs) == 0:
			case c.G.Def(xs[0]).Action():
				c.PlayNow(xs[0])
			default:
				c.MoveTo(c.P, xs, deck, "")
			}
		},
		Overpay: func(c *ctx, n int) {
			ds := c.Zone(c.P, discard)
			for _, x := range pick(c, c.P, engine.ToTopdeck, discard, nil, min(n, len(ds)), min(n, len(ds))) {
				c.Topdeck(c.P, one(x))
			}
		},
	})
	reg("Infirmary", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 1))
		},
		Overpay: func(c *ctx, n int) {
			if c.G.Where(c.Me) != (engine.Loc{Zone: discard, Owner: int16(c.P)}) {
				return // it went elsewhere when gained
			}
			c.PlayNow(c.Me)
			for k := 1; k < n; k++ {
				c.PlayCard(c.Me, k)
			}
		},
	})
	reg("Journeyman", rules{Play: func(c *ctx) {
		name := c.NameCard(c.P)
		c.Log("NAMES", engine.Seat(c.P), engine.Name(name))
		revealAndTake(c, func(d *def) bool { return d.ID != name }, 3)
	}})
	reg("Merchant Guild", rules{
		Play: func(c *ctx) {
			c.PlusBuys(1)
			c.PlusCoins(1)
			c.AddWatch(engine.On(engine.EvBuyStart, engine.EvBuyEnd), len(c.T2().Gained))
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			switch {
			case e.P != w.P:
			case e.Kind == engine.EvBuyStart:
				w.N = len(c.T2().Gained)
			case e.Kind == engine.EvBuyEnd:
				if n := len(c.T2().Gained) - w.N; n > 0 { // +1 Coffers per card gained in the Buy phase
					c.PlusCoffers(n)
				}
				return true
			}
			return false
		},
	})
	reg("Plaza", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(2)
		if len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, treasure, 0, 1))) > 0 {
			c.PlusCoffers(1)
		}
	}})
	reg("Soothsayer", rules{Play: func(c *ctx) {
		c.Gain(c.P, c.Card("Gold"), discard)
		c.Attack(func(v int) {
			if c.Gain(v, c.Card("Curse"), discard) >= 0 {
				c.DrawFor(v, 1)
			}
		})
	}})
	reg("Stonemason", rules{
		Play: func(c *ctx) {
			x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
			if x < 0 {
				return
			}
			cost := c.CostOf(c.G.CardOf(x))
			c.Trash(c.P, one(x))
			for range 2 {
				c.ChooseGain(c.P, func(d *def) bool { return c.CostOf(d.ID).Less(cost) }, discard)
			}
		},
		Overpay: func(c *ctx, n int) {
			for range 2 {
				c.ChooseGain(c.P, func(d *def) bool { return d.Action() && c.CostOf(d.ID) == engine.Upto(n) }, discard)
			}
		},
	})

	reg("Fairgrounds", rules{VP: func(g *engine.Game, p int) int {
		seen := map[engine.CardID]bool{}
		for _, x := range g.Owned(p) {
			seen[g.CardOf(x)] = true
		}
		return 2 * (len(seen) / 5)
	}})
}
