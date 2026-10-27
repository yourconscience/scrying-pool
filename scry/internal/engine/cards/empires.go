package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Empires: debt, VP tokens on piles and Landmarks, split piles, the Castles,
// the Events and the Landmarks. Simplifications, from what dominion.games
// logs: Capital's debt is not paid off in Clean-up, and a Mountain Pass bid of
// 0 is a pass.

// costsMore: a costs more than b, in $, Potions and debt.
func costsMore(a, b engine.Cost) bool { return b.LE(a) && a != b }

// downTo makes a player discard down to n cards in hand.
func downTo(c *ctx, v, n int) {
	if k := len(c.Hand(v)) - n; k > 0 {
		c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, k, k))
	}
}

// fromDiscard lets a player reveal a card from their discard pile and put it into their hand (Settlers).
func fromDiscard(c *ctx, name string) { // "looks at 3 Coppers and an Estate.", "puts a Copper into their hand."
	c.LookAt(c.P, c.Zone(c.P, discard))
	if x := pick1(c, c.P, engine.ToPick, discard, named(name), true); x >= 0 {
		c.ToHand(c.P, one(x), "PUT_IN_HAND")
	}
}

// castles counts a player's Castles.
func castles(g *engine.Game, p int) int {
	return g.Count(p, func(d *def) bool { return d.Is(engine.TCastle) })
}

// gainCastle gains the top Castle.
func gainCastle(c *ctx) {
	if i := c.G.PileOf(c.Card("Castles")); i >= 0 {
		if top := c.G.Top(i); top != 0 {
			c.Gain(c.P, top, discard)
		}
	}
}

// landVP is a Landmark with 6 VP per player that pays 2 at a time when ok allows.
func landVP(name string, on engine.EvMask, ok func(c *ctx, e *ev) bool) {
	reg(name, rules{
		Setup: func(g *engine.Game) { g.LandVP[g.T.MustCard(name)] = 12 },
		Hears: on,
		Hear: func(c *ctx, e *ev) {
			if ok(c, e) {
				c.TakeLandVP(2)
			}
		},
	})
}

// names counts each differently named card a player has.
func names(g *engine.Game, p int, ok func(*def) bool) map[engine.CardID]int {
	n := map[engine.CardID]int{}
	for _, x := range g.Owned(p) {
		if d := g.Def(x); ok == nil || ok(d) {
			n[d.ID]++
		}
	}
	return n
}

func init() {
	gold := func(c *ctx) engine.CardID { return c.Card("Gold") }
	silver := func(c *ctx) engine.CardID { return c.Card("Silver") }
	inPlayHere := func(c *ctx) bool { return !c.InPlace && c.G.Where(c.Me).Zone == inPlay }

	reg("Archive", rules{
		Play: func(c *ctx) { // "looks at a Copper, a Treasure Hunter and a Riverboat.", "puts a Treasure Hunter into their hand.", "sets a Copper and a Riverboat aside."
			c.PlusActions(1)
			top := c.FromTop(c.P, 3, "LOOK_AT")
			if len(top) == 0 {
				return
			}
			c.ToHand(c.P, pickOf(c, c.P, engine.ToPick, top, 1, 1), "PUT_IN_HAND")
			if rest := c.SetAside(c.P, c.In(revealed, top)); len(rest) > 0 {
				c.Stay().Aside = rest
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool { // "puts a Copper in hand (Archive)."
			left := c.In(aside, d.Aside)
			if len(left) > 0 {
				got := pickOf(c, c.P, engine.ToPick, left, 1, 1)
				c.ToHand(c.P, got, "PUT_IN_HAND_FROM", engine.Name(c.Src))
				for i, x := range d.Aside { // it no longer has that card, even if a Delay sets it aside again
					if len(got) == 1 && x == got[0] {
						d.Aside = append(d.Aside[:i:i], d.Aside[i+1:]...)
						break
					}
				}
			}
			return len(c.In(aside, d.Aside)) == 0
		},
	})
	reg("Capital", rules{
		Coins: 6,
		Play:  func(c *ctx) { c.PlusBuys(1) },
		OnDiscard: func(c *ctx, e *ev) { // "takes 6 debt." with no Clean-up line before it
			if e.From == inPlay {
				c.Quietly(func() { c.TakeDebt(6) })
			}
		},
	})
	reg("Catapult", rules{Play: func(c *ctx) { // "trashes a Copper.", the other player "discards a card and a Copper."
		c.PlusCoins(1)
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		d := c.G.Def(x)
		cost := c.CostOf(d.ID)
		c.Trash(c.P, one(x))
		c.Attack(func(v int) {
			if cost.Coins >= 3 {
				c.Gain(v, c.Card("Curse"), discard)
			}
			if d.Treasure() {
				downTo(c, v, 3)
			}
		})
	}})
	reg("Rocks", rules{
		Coins:   1,
		OnGain:  func(c *ctx, _ *ev) { rocks(c) },
		OnTrash: func(c *ctx, _ *ev) { rocks(c) },
	})
	reg("Chariot Race", rules{Play: func(c *ctx) { // "draws a card.", "reveals a Copper.", the other player "reveals a Scepter."
		c.PlusActions(1)
		mine := c.Draw(1)
		c.Reveal(c.P, mine)
		theirs := c.FromTop(c.Other(), 1, "REVEAL")
		c.MoveTo(c.Other(), theirs, deck, "")
		if len(mine) == 1 && (len(theirs) == 0 || costsMore(c.CostOf(c.G.CardOf(mine[0])), c.CostOf(c.G.CardOf(theirs[0])))) {
			c.PlusCoins(1)
			c.PlusVP(1)
		}
	}})
	reg("Charm", rules{
		Alone: true,
		Play: func(c *ctx) {
			if c.Mode1(c.P, mode("+1 Buy and +$2", "GETS_BUY"), mode("gain another card with the same cost the next time you gain a card", "")) == 0 {
				c.PlusBuys(1)
				c.PlusCoins(2)
			} else {
				c.AddWatch(engine.On(engine.EvGain), 0)
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P || w.N > 0 {
				return false
			}
			w.N = 1
			cost := c.CostOf(e.C)
			ok := func(d *def) bool { return d.ID != e.C && c.CostOf(d.ID) == cost }
			if o := c.ChooseOne(w.P, engine.ToGain, c.SupplyOptions(ok), true); o != nil {
				c.Gain(w.P, o.Card, discard)
			}
			return true
		},
	})
	reg("City Quarter", rules{Play: func(c *ctx) { // "reveals their hand: ...", "draws 2 cards."
		c.PlusActions(2)
		c.RevealHand(c.P)
		c.Draw(c.Count(c.P, hand, action))
	}})
	reg("Crown", rules{Play: func(c *ctx) { // "plays a Sailor.", "plays a Sailor again."
		ok := action
		if c.T2().Phase == engine.BuyPhase {
			ok = treasure
		}
		if x := pick1(c, c.P, engine.ToPlay, hand, ok, true); x >= 0 {
			c.PlayCard(x, 0)
			c.PlayCard(x, 1)
		}
	}})
	reg("Encampment", rules{
		Play: func(c *ctx) { // "sets an Encampment aside."
			c.Draw(2)
			c.PlusActions(2)
			if x := pick1(c, c.P, engine.ToReveal, hand, func(d *def) bool { return d.Name == "Gold" || d.Name == "Plunder" }, true); x >= 0 {
				c.Reveal(c.P, one(x))
				return
			}
			if inPlayHere(c) {
				c.SetAside(c.P, one(c.Me))
				c.AddWatch(engine.On(engine.EvCleanup), int(c.Me))
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "begins their Cleanup phase.", "returns an Encampment to the Encampment pile."
			if e.P != w.P {
				return false
			}
			if x := inst(w.N); c.G.Where(x).Zone == aside {
				c.ReturnTo(w.P, x)
			}
			return true
		},
	})
	reg("Plunder", rules{Coins: 2, Play: func(c *ctx) { c.PlusVP(1) }})
	reg("Enchantress", rules{
		Play: func(c *ctx) {
			d := c.Stay()
			c.Attack(func(int) { d.N = 1 })
		},
		Next:  func(c *ctx, d *engine.Dur) bool { d.N = 0; c.Draw(2); return true },
		Hears: engine.On(engine.EvPlaying),
		Hear: func(c *ctx, e *ev) { // "plays a Souk.", "Souk is enchanted by Enchantress", "draws a card.", "gets +1 Action."
			if e.P == c.P || e.Skip || c.G.Cur != e.P || c.T2().ActionsIn != 1 || !c.G.D(e.C).Action() {
				return
			}
			for _, d := range c.Durs() {
				if d.N == 1 {
					e.Skip = true
					c.Here().At(e.Level+1).Log("PLAY_ENCHANTED", engine.Name(e.C), engine.Name(c.Src))
					v := c.Here().As(e.P).At(e.Level + 2)
					v.Draw(1)
					v.PlusActions(1)
					return
				}
			}
		},
	})
	reg("Engineer", rules{Play: func(c *ctx) { // "gains a Hostelry.", "trashes an Engineer.", "gains a Blockade."
		c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		if inPlayHere(c) && c.Yes(c.P, "trash it to gain another card", "TRASH") {
			c.Trash(c.P, one(c.Me))
			c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		}
	}})
	reg("Farmers' Market", rules{Play: func(c *ctx) { // "adds 1 VP to the Farmers' Market pile.", "gets +$1."; "takes 4 VP from the Farmers' Market pile."
		c.PlusBuys(1)
		i := c.G.PileOf(c.Src)
		if c.G.Piles[i].VP >= 4 {
			c.TakeVPFrom(i)
			if inPlayHere(c) {
				c.Trash(c.P, one(c.Me))
			}
			return
		}
		c.AddVPTo(i, 1)
		c.PlusCoins(c.G.Piles[i].VP)
	}})
	reg("Forum", rules{
		Play: func(c *ctx) {
			c.Draw(3)
			c.PlusActions(1)
			n := min(2, len(c.Hand(c.P)))
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, n, n))
		},
		OnGain: func(c *ctx, _ *ev) { c.PlusBuys(1) },
	})
	reg("Gladiator", rules{Play: func(c *ctx) { // "reveals a Lackeys.", "gets +$1.", "trashes a Gladiator."
		c.PlusCoins(2)
		x := pick1(c, c.P, engine.ToReveal, hand, nil, false)
		if x >= 0 {
			c.Reveal(c.P, one(x))
			if y := pick1(c, c.Other(), engine.ToReveal, hand, named(c.G.Def(x).Name), true); y >= 0 {
				c.Reveal(c.Other(), one(y))
				return
			}
		}
		c.PlusCoins(1)
		c.TrashFromSupply(c.P, c.Card("Gladiator"))
	}})
	reg("Fortune", rules{
		Alone: true,
		Play: func(c *ctx) { // "gets +1 Buy."; the $ doubles silently
			c.PlusBuys(1)
			if c.T2().Used[c.Src] == 0 {
				c.T2().Used[c.Src]++
				c.Pl(c.P).Coins *= 2
			}
		},
		OnGain: func(c *ctx, _ *ev) { c.GainN(c.P, gold(c), c.Count(c.P, inPlay, named("Gladiator")), discard, "GAIN") },
	})
	reg("Groundskeeper", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			if c.T2().Used[c.Src]++; c.T2().Used[c.Src] == 1 { // one watch counts this turn's plays
				c.AddWatch(engine.On(engine.EvGain), 0)
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "buys and gains an Estate.", "gets 2 VP from Groundskeeper." for two plays
			if e.P == w.P && c.G.D(e.C).Victory() {
				n := c.T2().Used[c.Src]
				c.Pl(w.P).VP += n
				c.Log("PLAYER_GETS_VP_FROM", engine.Seat(w.P), n, engine.Name(c.Src))
			}
			return false
		},
	})
	reg("Legionary", rules{Play: func(c *ctx) { // "reveals a Gold.", the other player "discards 3 cards.", "draws a Copper."
		c.PlusCoins(3)
		x := pick1(c, c.P, engine.ToReveal, hand, named("Gold"), true)
		if x < 0 {
			return
		}
		c.Reveal(c.P, one(x))
		c.Attack(func(v int) {
			downTo(c, v, 2)
			c.DrawFor(v, 1)
		})
	}})
	reg("Overlord", rules{Play: func(c *ctx) { // "plays an Overlord.", "plays an Upgrade."
		ok := and(action, not(func(d *def) bool { return d.Is(engine.TCommand) || d.Is(engine.TDuration) }), c.CostsUpTo(5))
		o := c.ChooseOne(c.P, engine.ToPlay, c.SupplyOptions(ok), false)
		if o == nil {
			return
		}
		s := c.G.Piles[c.G.PileOf(o.Card)].Cards
		c.PlayHere(s[len(s)-1])
	}})
	reg("Patrician", rules{Play: func(c *ctx) { // "reveals a Cursed Gold.", "topdecks a Cursed Gold."
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 1, "REVEAL")
		if len(top) == 0 {
			return
		}
		if c.CostOf(c.G.CardOf(top[0])).Coins >= 5 {
			c.ToHand(c.P, top, "PUT_IN_HAND")
		} else {
			c.MoveTo(c.P, top, deck, "TOPDECK")
		}
	}})
	reg("Emporium", rules{
		Play: func(c *ctx) { c.Draw(1); c.PlusActions(1); c.PlusCoins(1) },
		OnGain: func(c *ctx, _ *ev) {
			if c.Count(c.P, inPlay, action) >= 5 {
				c.PlusVP(2)
			}
		},
	})
	reg("Royal Blacksmith", rules{Play: func(c *ctx) { // "reveals their hand: ...", "discards nothing."
		c.Draw(5)
		c.RevealHand(c.P)
		var cu []inst
		for _, x := range c.Hand(c.P) {
			if c.G.Def(x).Name == "Copper" {
				cu = append(cu, x)
			}
		}
		if len(cu) == 0 {
			c.Log("DISCARD", engine.Seat(c.P), []engine.CardID{})
			return
		}
		c.Discard(c.P, cu)
	}})
	reg("Sacrifice", rules{Play: func(c *ctx) { // "trashes an Estate.", "gets 2 VP."
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		d := c.G.Def(x)
		c.Trash(c.P, one(x))
		if d.Action() {
			c.Draw(2)
			c.PlusActions(2)
		}
		if d.Treasure() {
			c.PlusCoins(2)
		}
		if d.Victory() {
			c.PlusVP(2)
		}
	}})
	reg("Settlers", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		fromDiscard(c, "Copper")
	}})
	reg("Bustling Village", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(3)
		fromDiscard(c, "Settlers")
	}})
	reg("Temple", rules{
		Play: func(c *ctx) { // "gets 1 VP.", "trashes a Copper and an Estate."; the VP it adds to its pile is silent
			c.PlusVP(1)
			var firsts []inst
			seen := map[engine.CardID]bool{}
			for _, x := range c.Hand(c.P) {
				if id := c.G.CardOf(x); !seen[id] {
					seen[id] = true
					firsts = append(firsts, x)
				}
			}
			if len(firsts) > 0 {
				c.Trash(c.P, pickOf(c, c.P, engine.ToTrash, firsts, 1, min(3, len(firsts))))
			}
			c.G.Piles[c.G.PileOf(c.Src)].VP++
		},
		OnGain: func(c *ctx, _ *ev) { c.TakeVPFrom(c.G.PileOf(c.Src)) }, // "takes 2 VP from the Temple pile."
	})
	reg("Villa", rules{
		Play: func(c *ctx) { c.PlusActions(2); c.PlusBuys(1); c.PlusCoins(1) },
		OnGain: func(c *ctx, _ *ev) { // "buys and gains a Villa.", "puts a Villa into their hand.", "gets +1 Action.", then back to the Action phase
			c.ToHand(c.P, one(c.Me), "PUT_IN_HAND")
			c.PlusActions(1)
			if c.G.Cur == c.P && c.T2().Phase == engine.BuyPhase {
				c.T2().Back = true
			}
		},
	})
	reg("Wild Hunt", rules{Play: func(c *ctx) { // the VP it adds is silent; "gains an Estate.", "gets 3 VP."
		i := c.G.PileOf(c.Src)
		if c.Mode1(c.P, mode("+3 Cards and add 1 VP to the pile", "DRAW"), mode("gain an Estate and take the VP", "GAIN")) == 0 {
			c.Draw(3)
			c.G.Piles[i].VP++
		} else if c.Gain(c.P, c.Card("Estate"), discard) >= 0 {
			if n := c.G.Piles[i].VP; n > 0 {
				c.G.Piles[i].VP = 0
				c.PlusVP(n)
			}
		}
	}})

	// Castles, cheapest on top
	vpPer := func(n int) func(*engine.Game, int) int {
		return func(g *engine.Game, p int) int { return n * castles(g, p) }
	}
	reg("Humble Castle", rules{Coins: 1, VP: vpPer(1)})
	crumbling := func(c *ctx, _ *ev) { // "gets 1 VP.", "gains a Silver."
		c.PlusVP(1)
		c.Gain(c.P, silver(c), discard)
	}
	reg("Crumbling Castle", rules{VP: worth(1), OnGain: crumbling, OnTrash: crumbling})
	reg("Small Castle", rules{VP: worth(2), Play: func(c *ctx) { // "trashes a Humble Castle.", "gains a Sprawling Castle."
		opts := c.Options(c.P, hand, func(d *def) bool { return d.Is(engine.TCastle) })
		if inPlayHere(c) {
			opts = append(c.OptionsOf(one(c.Me)), opts...)
		}
		if o := c.ChooseOne(c.P, engine.ToTrash, opts, false); o != nil && len(c.Trash(c.P, one(o.Inst))) == 1 {
			gainCastle(c)
		}
	}})
	reg("Haunted Castle", rules{VP: worth(2), OnGain: func(c *ctx, _ *ev) { // "gains a Gold.", the other player "topdecks a Copper and a Town."
		if c.G.Cur != c.P {
			return
		}
		c.Gain(c.P, gold(c), discard)
		if v := c.Other(); len(c.Hand(v)) >= 5 {
			topdeckAny(c, v, pick(c, v, engine.ToTopdeck, hand, nil, 2, 2))
		}
	}})
	reg("Opulent Castle", rules{VP: worth(3), Play: func(c *ctx) { // "discards and reveals an Estate and a Duchy.", "gets +$4."
		xs := pick(c, c.P, engine.ToDiscard, hand, victory, 0, len(c.Hand(c.P)))
		if n := len(c.DiscardAs(c.P, xs, "DISCARD_REVEALED")); n > 0 {
			c.PlusCoins(2 * n)
		}
	}})
	reg("Sprawling Castle", rules{VP: worth(4), OnGain: func(c *ctx, _ *ev) {
		if c.Mode1(c.P, mode("gain a Duchy", "GAIN"), mode("gain 3 Estates", "GAIN")) == 0 {
			c.Gain(c.P, c.Card("Duchy"), discard)
		} else {
			c.GainN(c.P, c.Card("Estate"), 3, discard, "GAIN")
		}
	}})
	reg("Grand Castle", rules{VP: worth(5), OnGain: func(c *ctx, _ *ev) { // "reveals their hand: a Squire and a Silver.", "gets 0 VP from Grand Castle."
		c.RevealHand(c.P)
		n := c.Count(c.P, hand, victory) + c.Count(c.P, inPlay, victory)
		c.Pl(c.P).VP += n
		c.Log("PLAYER_GETS_VP_FROM", engine.Seat(c.P), n, engine.Name(c.Src))
	}})
	reg("King's Castle", rules{VP: vpPer(2)})

	// Events
	reg("Advance", rules{Buy: func(c *ctx) {
		if len(c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, action, true)))) == 1 {
			c.ChooseGain(c.P, and(action, c.CostsUpTo(6)), discard)
		}
	}})
	reg("Annex", rules{Buy: func(c *ctx) {
		pile := c.Zone(c.P, discard)
		keep := map[inst]bool{}
		for _, x := range pick(c, c.P, engine.ToPick, discard, nil, 0, min(5, len(pile))) {
			keep[x] = true
		}
		var rest []inst
		for _, x := range pile {
			if !keep[x] {
				rest = append(rest, x)
			}
		}
		c.ShuffleInto(c.P, rest)
		c.Gain(c.P, c.Card("Duchy"), discard)
	}})
	reg("Banquet", rules{Buy: func(c *ctx) {
		c.GainN(c.P, c.Card("Copper"), 2, discard, "GAIN")
		c.ChooseGain(c.P, and(not(victory), c.CostsUpTo(5)), discard)
	}})
	reg("Conquest", rules{Buy: func(c *ctx) {
		c.GainN(c.P, silver(c), 2, discard, "GAIN")
		n := 0
		for _, g := range c.T2().Gained {
			if g == silver(c) {
				n++
			}
		}
		if n > 0 {
			c.PlusVP(n)
		}
	}})
	reg("Delve", rules{Buy: func(c *ctx) { c.PlusBuys(1); c.Gain(c.P, silver(c), discard) }})
	reg("Dominate", rules{Buy: func(c *ctx) {
		if c.Gain(c.P, c.Card("Province"), discard) >= 0 {
			c.PlusVP(9)
		}
	}})
	reg("Donate", rules{
		Buy: func(c *ctx) {
			if c.T2().Used[c.Src] == 0 {
				c.T2().Used[c.Src]++
				c.AddWatch(engine.On(engine.EvTurnStart), 0).Long = true
			}
		},
		// dominion.games does it at the start of the player's next turn, without
		// "starts their turn": "puts 8 cards into their hand.", "trashes 4 Coppers
		// and 3 Estates.", "shuffles their deck.", "shuffles 6 cards into their deck.", "draws 5 cards."
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P {
				return false
			}
			p := w.P
			c.Quietly(func() {
				c.ToHand(p, append([]inst{}, c.Zone(p, discard)...), "PUT_IN_HAND") // the discard pile, then the deck on a line of its own
				c.ToHand(p, append([]inst{}, c.Zone(p, deck)...), "PUT_IN_HAND")
				c.Trash(p, pick(c, p, engine.ToTrash, hand, nil, 0, len(c.Hand(p))))
				if len(c.Hand(p)) > 0 {
					c.ShuffleLine(p)
				}
				c.ShuffleInto(p, append([]inst{}, c.Hand(p)...))
				c.DrawFor(p, 5)
			})
			return true
		},
	})
	reg("Ritual", rules{Buy: func(c *ctx) {
		if c.Gain(c.P, c.Card("Curse"), discard) < 0 {
			return
		}
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		n := c.CostOf(c.G.CardOf(x)).Coins
		c.Trash(c.P, one(x))
		if n > 0 {
			c.PlusVP(n)
		}
	}})
	reg("Salt the Earth", rules{Buy: func(c *ctx) {
		c.PlusVP(1)
		if o := c.ChooseOne(c.P, engine.ToTrash, c.SupplyOptions(victory), false); o != nil {
			c.TrashFromSupply(c.P, o.Card)
		}
	}})
	reg("Tax", rules{
		Setup: func(g *engine.Game) {
			for i := range g.Piles {
				if g.Piles[i].Supply {
					g.Piles[i].Debt++
				}
			}
		},
		Buy: func(c *ctx) { // "adds 2 Debt to the Harbinger pile."
			var opts []engine.Option
			for _, p := range c.G.Piles {
				if p.Supply {
					opts = append(opts, engine.Option{Card: p.Card, Inst: -1})
				}
			}
			if o := c.ChooseOne(c.P, engine.ToPick, opts, false); o != nil {
				c.G.Piles[c.G.PileOf(o.Card)].Debt += 2
				c.Log("TAX_ADD_DEBT", engine.Seat(c.P), "2 Debt", engine.PileName(o.Card))
			}
		},
		Hears: engine.On(engine.EvBought),
		Hear: func(c *ctx, e *ev) { // "buys and gains a Courtier.", what the gain causes, "takes 1 Debt from the Courtier pile."
			i := c.G.PileOf(e.C)
			if i < 0 || c.G.Piles[i].Debt == 0 {
				return
			}
			n := c.G.Piles[i].Debt
			c.G.Piles[i].Debt = 0
			c.Pl(e.P).Debt += n
			c.Log("TAX_TAKE_DEBT", engine.Seat(e.P), itoa(n)+" Debt", engine.PileName(c.G.Piles[i].Card))
		},
	})
	reg("Triumph", rules{Buy: func(c *ctx) {
		if c.Gain(c.P, c.Card("Estate"), discard) >= 0 {
			c.PlusVP(len(c.T2().Gained))
		}
	}})
	reg("Wedding", rules{Buy: func(c *ctx) { c.Gain(c.P, gold(c), discard); c.PlusVP(1) }}) // the log has the Gold first
	reg("Windfall", rules{Buy: func(c *ctx) {
		if len(c.Zone(c.P, deck)) == 0 && len(c.Zone(c.P, discard)) == 0 {
			c.GainN(c.P, gold(c), 3, discard, "GAIN")
		}
	}})

	// Landmarks
	reg("Aqueduct", rules{
		Setup: func(g *engine.Game) {
			for _, n := range []string{"Silver", "Gold"} {
				g.Piles[g.PileOf(g.T.MustCard(n))].VP = 8
			}
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "moves 1 VP from the Silver pile to Aqueduct."
			d := c.G.D(e.C)
			if i := c.G.PileOf(e.C); d.Treasure() && i >= 0 {
				c.MoveVPToLand(i)
			}
			if d.Victory() {
				c.TakeLandVP(c.G.LandVP[c.Src])
			}
		},
	})
	reg("Arena", rules{
		Setup: func(g *engine.Game) { g.LandVP[g.T.MustCard("Arena")] = 12 },
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, e *ev) { // "begins their buy phase.", "discards a Farmers' Market.", "takes 2 VP from Arena."
			if e.P != c.G.Cur || c.G.LandVP[c.Src] == 0 {
				return
			}
			if len(c.Discard(e.P, one(pick1(c, e.P, engine.ToDiscard, hand, action, true)))) == 1 {
				c.As(e.P).TakeLandVP(2)
			}
		},
	})
	reg("Bandit Fort", rules{VP: func(g *engine.Game, p int) int {
		return -2 * g.Count(p, func(d *def) bool { return d.Name == "Silver" || d.Name == "Gold" })
	}})
	landVP("Basilica", engine.On(engine.EvGain), func(c *ctx, e *ev) bool { // "buys and gains a Silver.", "takes 2 VP from Basilica."
		return inBuyPhase(c, e.P) && c.Pl(e.P).Coins >= 2
	})
	landVP("Baths", engine.On(engine.EvTurnEnd), func(c *ctx, e *ev) bool { // after the Clean-up draw
		return len(c.T2().Gained) == 0
	})
	landVP("Battlefield", engine.On(engine.EvGain), func(c *ctx, e *ev) bool { return c.G.D(e.C).Victory() })
	landVP("Colonnade", engine.On(engine.EvBought), func(c *ctx, e *ev) bool {
		return inBuyPhase(c, e.P) && c.G.D(e.C).Action() && c.Count(e.P, inPlay, named(c.G.D(e.C).Name)) > 0
	})
	landVP("Labyrinth", engine.On(engine.EvGain), func(c *ctx, e *ev) bool { return e.P == c.G.Cur && len(c.T2().Gained) == 2 && c.T2().Gained[1] == e.C }) // the second gain, not the first whose own gain came between
	reg("Defiled Shrine", rules{
		Setup: func(g *engine.Game) {
			for i, p := range g.Piles {
				if d := g.D(p.Card); p.Supply && d.Action() && !d.Is(engine.TGathering) {
					g.Piles[i].VP = 2
				}
			}
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "moves 1 VP from the Clerk pile to Defiled Shrine."
			switch d := c.G.D(e.C); {
			case d.Action():
				if i := c.G.PileOf(e.C); i >= 0 {
					c.MoveVPToLand(i)
				}
			case d.Name == "Curse" && inBuyPhase(c, e.P):
				c.TakeLandVP(c.G.LandVP[c.Src])
			}
		},
	})
	reg("Fountain", rules{VP: func(g *engine.Game, p int) int {
		if g.Count(p, named("Copper")) >= 10 {
			return 15
		}
		return 0
	}})
	reg("Keep", rules{VP: func(g *engine.Game, p int) int {
		mine, theirs := names(g, p, treasure), names(g, 1-p, treasure)
		vp := 0
		for c, n := range mine {
			if n >= theirs[c] {
				vp += 5
			}
		}
		return vp
	}})
	reg("Mountain Pass", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // LandVP is 1 once the bids are done
			if c.G.D(e.C).Name == "Province" && c.G.LandVP[c.Src] == 0 {
				c.G.LandVP[c.Src] = 1
				// right under the gain: "bids 15 debt", "bids 0 debt", "takes 15 debt.", "gets 8 VP from Mountain Pass."
				bid(c, e.P)
			}
		},
	})
	reg("Museum", rules{VP: func(g *engine.Game, p int) int { return 2 * len(names(g, p, nil)) }})
	reg("Obelisk", rules{
		Start: func(g *engine.Game, me engine.CardID) { // "Obelisk chooses the Capital City pile.", before the starting decks
			if g.Obelisk < 0 {
				var piles []int
				for i, p := range g.Piles {
					if p.Supply && g.D(p.Card).Action() {
						piles = append(piles, i)
					}
				}
				if len(piles) == 0 {
					return
				}
				g.Obelisk = piles[g.Rand(len(piles))]
			}
			g.CtxFor(0, me, 0).Log("OBELISK_CHOOSES_PILE", engine.PileName(g.Piles[g.Obelisk].Card))
		},
		VP: func(g *engine.Game, p int) int {
			if g.Obelisk < 0 {
				return 0
			}
			return 2 * g.Count(p, func(d *def) bool { return g.PileOf(d.ID) == g.Obelisk })
		},
	})
	reg("Orchard", rules{VP: func(g *engine.Game, p int) int {
		vp := 0
		for _, n := range names(g, p, action) {
			if n >= 3 {
				vp += 4
			}
		}
		return vp
	}})
	reg("Palace", rules{VP: func(g *engine.Game, p int) int {
		return 3 * min(g.Count(p, named("Copper")), g.Count(p, named("Silver")), g.Count(p, named("Gold")))
	}})
	reg("Tomb", rules{
		Hears: engine.On(engine.EvTrash),
		Hear: func(c *ctx, e *ev) { // "trashes an Estate.", "gets 1 VP from Tomb."
			if e.P == c.P {
				c.Pl(c.P).VP++
				c.Log("PLAYER_GETS_VP_FROM", engine.Seat(c.P), 1, engine.Name(c.Src))
			}
		},
	})
	reg("Tower", rules{VP: func(g *engine.Game, p int) int {
		return g.Count(p, func(d *def) bool {
			i := g.PileOf(d.ID)
			return !d.Victory() && i >= 0 && g.Piles[i].Supply && len(g.Piles[i].Cards) == 0
		})
	}})
	reg("Triumphal Arch", rules{VP: func(g *engine.Game, p int) int {
		first, second := 0, 0
		for _, n := range names(g, p, action) {
			switch {
			case n > first:
				first, second = n, first
			case n > second:
				second = n
			}
		}
		return 3 * second
	}})
	reg("Wall", rules{VP: func(g *engine.Game, p int) int { return -max(0, len(g.Owned(p))-15) }})
	reg("Wolf Den", rules{VP: func(g *engine.Game, p int) int {
		vp := 0
		for _, n := range names(g, p, nil) {
			if n == 1 {
				vp -= 3
			}
		}
		return vp
	}})
}

// inBuyPhase: it is player p's Buy phase.
func inBuyPhase(c *ctx, p int) bool { return c.G.Cur == p && c.T2().Phase == engine.BuyPhase }

// rocks gains a Silver: onto the deck in its owner's Buy phase, else into their hand.
func rocks(c *ctx) {
	if inBuyPhase(c, c.P) {
		c.GainAs(c.P, c.Card("Silver"), deck, "GAIN_ON_DRAWPILE")
	} else {
		c.Gain(c.P, c.Card("Silver"), hand)
	}
}

// bid is Mountain Pass's auction: each player bids once, ending with the one who gained the Province.
func bid(c *ctx, last int) {
	high, best := -1, 0
	for _, p := range [2]int{1 - last, last} {
		n := c.Number(p, 0, 40, "PLAYER_BIDS")
		c.Log("PLAYER_BIDS", engine.Seat(p), n)
		if n > best {
			high, best = p, n
		}
	}
	if high >= 0 {
		w := c.As(high)
		w.TakeDebt(best)
		w.Pl(high).VP += 8
		w.Log("PLAYER_GETS_VP_FROM", engine.Seat(high), 8, engine.Name(c.Src))
	}
}
