package cards

import (
	"slices"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Rising Sun: Omens' +1 Sun, the Prophecies, Shadow cards and the Events.
// Simplifications: Shadow cards are played from the deck only in the Action
// phase, not by Throne Rooms; Flourishing Trade's Action plays become Buys
// only once the Buys run out; Enlightenment's Treasures are Actions only for
// playing them in the Action phase.

// sun removes a Sun token from the Prophecy: "+1 Sun". The last one makes it
// come true, "Great Leader comes true.", and Kind Emperor acts then too.
func sun(c *ctx) {
	g := c.G
	if g.Prophecy == 0 || g.Sun == 0 {
		return
	}
	if g.Sun--; g.Sun > 0 {
		return
	}
	c.Log("COMES_TRUE", engine.Name(g.Prophecy))
	if r := g.D(g.Prophecy).Rules; r != nil && r.Play != nil {
		d := *c
		d.Src, d.Me = g.Prophecy, -1
		r.Play(&d)
	}
}

// cameTrue: the Prophecy acts once its last Sun is gone.
func cameTrue(c *ctx) bool { return c.G.Prophecy != 0 && c.G.Sun == 0 }

// noDuplicates: no two cards of a hand share a name (Snake Witch).
func noDuplicates(c *ctx, xs []inst) bool {
	seen := map[engine.CardID]bool{}
	for _, x := range xs {
		if seen[c.G.CardOf(x)] {
			return false
		}
		seen[c.G.CardOf(x)] = true
	}
	return true
}

// hasActionInPlay: an Action card in play (Amass).
func hasActionInPlay(c *ctx, p int) bool { return c.Count(p, inPlay, action) > 0 }

func init() {
	reg("Fishmonger", rules{Play: func(c *ctx) { c.PlusBuys(1); c.PlusCoins(1) }})
	reg("Snake Witch", rules{Play: func(c *ctx) { // "reveals their hand: ...", "returns a Snake Witch to the Snake Witch pile.", the other "gains a Curse."
		c.Draw(1)
		c.PlusActions(1)
		if h := c.Hand(c.P); len(h) > 0 && noDuplicates(c, h) && c.Yes(c.P, "reveal your hand and return this", "REVEAL_HAND") {
			c.RevealHand(c.P)
			if c.ReturnTo(c.P, c.Me) {
				c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
			}
		}
	}})
	reg("Aristocrat", rules{Play: func(c *ctx) {
		switch n := c.Count(c.P, inPlay, named("Aristocrat")); {
		case n > 8:
		case n%4 == 1:
			c.PlusActions(3)
		case n%4 == 2:
			c.Draw(3)
		case n%4 == 3:
			c.PlusCoins(3)
		default:
			c.PlusBuys(3)
		}
	}})
	reg("Craftsman", rules{Play: func(c *ctx) {
		c.TakeDebt(2)
		c.ChooseGain(c.P, c.CostsUpTo(5), discard)
	}})
	reg("Riverboat", rules{
		Play: func(c *ctx) { c.Stay() },
		Next: func(c *ctx, _ *engine.Dur) bool { // "plays a Scholar. (|)", the set-aside card where it is
			if i := c.G.PileOf(c.G.Riverboat); i >= 0 && c.G.Top(i) == c.G.Riverboat {
				s := c.G.Piles[i].Cards
				c.PlayHereBlank(s[len(s)-1])
			}
			return true
		},
		Setup: func(g *engine.Game) { // an unused non-Duration Action costing $5, set aside as a pile of one
			us := g.Unused(func(d *def) bool {
				return d.Action() && !duration(d) && d.Rules != nil && d.Cost == engine.Upto(5)
			})
			if len(us) == 0 {
				return
			}
			g.Riverboat = us[g.Rand(len(us))]
			g.AddPile(g.Riverboat, 1, false)
			if r := g.D(g.Riverboat).Rules; r.Setup != nil {
				r.Setup(g)
			}
		},
	})
	reg("Root Cellar", rules{Play: func(c *ctx) { c.Draw(3); c.PlusActions(1); c.TakeDebt(3) }})
	reg("Alley", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.Discard(c.P, one(pick1(c, c.P, engine.ToDiscard, hand, nil, false)))
	}})
	reg("Change", rules{Play: func(c *ctx) { // "trashes a Silver.", "gains a Gold.", "takes 3 debt."
		if pl := c.Pl(c.P); pl.Debt > 0 { // the debt is paid first if it can be: "loses 1 Coin.", then the trash
			had := pl.Coins
			if pl.Debt > had {
				c.OfferCoffers() // or with Coffers, silently: "uses 1 Coffers. (+$1)", "trashes a Student."
			}
			n := min(pl.Debt, pl.Coins)
			pl.Debt -= n
			pl.Coins -= n
			if n := min(n, had); n == 1 {
				c.Log("LOSES_COIN", engine.Seat(c.P), n)
			} else if n > 1 {
				c.Log("LOSES_COINS", engine.Seat(c.P), n)
			}
		}
		if c.Pl(c.P).Debt > 0 {
			c.PlusCoins(3)
			return
		}
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		was := c.CostOf(c.G.CardOf(x)).Coins
		c.Trash(c.P, one(x))
		if y := c.ChooseGain(c.P, func(d *def) bool { return c.CostOf(d.ID).Coins > was }, discard); y >= 0 {
			c.TakeDebt(c.CostOf(c.G.CardOf(y)).Coins - was)
		}
	}})
	reg("Ninja", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.Attack(func(v int) { downTo(c, v, 3) })
	}})
	reg("Poet", rules{Play: func(c *ctx) { // "reveals a Copper.", "puts a Copper into their hand." or "topdecks a Gold."
		sun(c)
		c.Draw(1)
		c.PlusActions(1)
		if x := c.TopCard(c.P); x >= 0 {
			c.Reveal(c.P, one(x))
			if c.CostsUpTo(3)(c.G.Def(x)) {
				c.ToHand(c.P, one(x), "PUT_IN_HAND")
			} else {
				c.Topdeck(c.P, one(x))
			}
		}
	}})
	reg("River Shrine", rules{
		Play: func(c *ctx) {
			sun(c)
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 2))
			c.AddWatch(engine.On(engine.EvCleanup), 0)
		},
		Watch: func(c *ctx, e *ev, _ *engine.Watch) bool { // at the start of Clean-up, with no gains in the Buy phase
			if e.P == c.P && len(c.T2().BuyGained) == 0 {
				phaseLine(c) // "begins their Cleanup phase.", "gains a Night Watchman."
				c.Here().ChooseGain(c.P, c.CostsUpTo(4), discard)
			}
			return true
		},
	})
	reg("Rustic Village", rules{Play: func(c *ctx) { // "discards a Copper and an Estate.", "draws a card."
		sun(c)
		c.Draw(1)
		c.PlusActions(2)
		if xs := pick(c, c.P, engine.ToDiscard, hand, nil, 0, 2); len(xs) == 2 {
			c.Discard(c.P, xs)
			c.Draw(1)
		}
	}})
	reg("Gold Mine", rules{Play: func(c *ctx) { // "gains a Gold.", "takes 4 debt."
		c.Draw(1)
		c.PlusActions(1)
		c.PlusBuys(1)
		if c.Yes(c.P, "gain a Gold and take 4 debt", "GAIN") {
			c.Gain(c.P, c.Card("Gold"), discard)
			c.TakeDebt(4)
		}
	}})
	reg("Imperial Envoy", rules{Play: func(c *ctx) { c.Draw(5); c.PlusBuys(1); c.TakeDebt(2) }})
	reg("Kitsune", rules{Play: func(c *ctx) {
		sun(c)
		curse := engine.Mode{Label: "each other player gains a Curse", Line: "GAIN", Card: c.Card("Curse")}
		silver := engine.Mode{Label: "gain a Silver", Line: "GAIN", Card: c.Card("Silver")}
		for _, m := range c.Modes(c.P, 2, 2, mode("+$2", "GETS_COINS"), mode("+2 Actions", "GETS_ACTIONS"), curse, silver) {
			switch m {
			case 0:
				c.PlusCoins(2)
			case 1:
				c.PlusActions(2)
			case 2:
				c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
			case 3:
				c.Gain(c.P, c.Card("Silver"), discard)
			}
		}
	}})
	reg("Litter", rules{Play: func(c *ctx) { c.Draw(2); c.PlusActions(2); c.TakeDebt(1) }})
	reg("Rice Broker", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		d := c.G.Def(x)
		c.Trash(c.P, one(x))
		if d.Treasure() {
			c.Draw(2)
		}
		if d.Action() {
			c.Draw(5)
		}
	}})
	reg("Ronin", rules{Play: func(c *ctx) {
		drawTo(c, 7)
	}})
	reg("Tanuki", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		c.Trash(c.P, one(x))
		c.ChooseGain(c.P, costUp(c, x, 2), discard)
	}})
	reg("Tea House", rules{Play: func(c *ctx) { sun(c); c.Draw(1); c.PlusActions(1); c.PlusCoins(2) }})
	reg("Samurai", rules{
		Play: func(c *ctx) {
			c.Attack(func(v int) { downTo(c, v, 3) })
			c.Stay()
		},
		Next: func(c *ctx, _ *engine.Dur) bool { // "gets +$1. (Samurai)" each turn; it stays in play
			c.PlusCoins(1)
			return false
		},
	})
	reg("Rice", rules{
		Worth: func(g *engine.Game, p int) int { // one per type among the cards in play, this one's Treasure too
			var types engine.Types
			for _, x := range g.P[p].Z[engine.InPlay] {
				types |= g.Def(x).T
			}
			n := 0
			for ; types != 0; types &= types - 1 {
				n++
			}
			return n
		},
		Play: func(c *ctx) { c.PlusBuys(1) },
	})
	reg("Mountain Shrine", rules{Play: func(c *ctx) { // "gets +$2.", "trashes a Copper.", "draws 2 cards."
		sun(c)
		c.PlusCoins(2)
		c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, true)))
		if len(inTrash(c, action)) > 0 {
			c.Draw(2)
		}
	}})
	reg("Daimyo", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.AddWatch(engine.On(engine.EvPlay), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // after the next non-Command Action: "plays a Treasurer. (Daimyo)"
			d := c.G.Def(e.X)
			if w.N > 0 || e.P != c.P || e.X == c.Me || !d.Action() || d.Is(engine.TCommand) || e.By == c.Src {
				return false // w.N: the replay's own play is done, not the next one; another Daimyo's replay isn't it either
			}
			w.N = 1
			a := c.At(e.Level)
			a.Me = -1              // it doesn't hold a Duration it replays
			a.PlayCardWith(e.X, 1) // a replay where the card is: a Ratcatcher on the Tavern mat doesn't come back
			return true
		},
	})
	reg("Artist", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		n := map[engine.CardID]int{}
		for _, x := range c.Zone(c.P, inPlay) {
			n[c.G.CardOf(x)]++
		}
		k := 0
		for _, m := range n {
			if m == 1 {
				k++
			}
		}
		c.Draw(k)
	}})

	// Prophecies: each acts only once it came true.
	reg("Approaching Army", rules{
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) { // "gets +$1." after an Attack, unnamed
			if cameTrue(c) && c.G.D(e.C).Attack() {
				c.As(e.P).Here().PlusCoins(1)
			}
		},
		Setup: func(g *engine.Game) { // an Attack kingdom pile more; a replayed game's kingdom has it already
			banes := 0
			for _, pile := range g.Piles {
				if pile.Bane && pile.Supply {
					banes++
				}
			}
			if len(g.Kingdom) > 10+banes {
				return
			}
			if us := g.Unused(func(d *def) bool { return d.Attack() && d.Kind == "kingdom" }); len(us) > 0 {
				g.AddKingdom(us[g.Rand(len(us))])
			}
		},
	})
	reg("Biding Time", rules{
		Hears: engine.On(engine.EvCleanup),
		Hear: func(c *ctx, e *ev) { // "sets a Silver and 2 Coppers aside with Biding Time.", "sets nothing aside ..."; next turn into the hand, unlogged
			if !cameTrue(c) {
				return
			}
			xs := append([]inst{}, c.Hand(e.P)...)
			ids := make([]engine.CardID, len(xs))
			for i, x := range xs {
				ids[i] = c.G.CardOf(x)
			}
			c.As(e.P).Log("SET_ASIDE_WITH", engine.Seat(e.P), ids, engine.Name(c.Src))
			c.As(e.P).MoveTo(e.P, xs, aside, "")
			if len(xs) > 0 {
				d := c.As(e.P).Later()
				d.Aside = xs
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.MoveTo(c.P, c.In(aside, d.Aside), hand, "")
			return true
		},
	})
	reg("Bureaucracy", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "gains a Merchant Guild.", then "gains a Copper." under it
			if cameTrue(c) && c.CostOf(e.C) != (engine.Cost{}) {
				c.Gain(e.P, c.Card("Copper"), discard)
			}
		},
	})
	reg("Good Harvest", rules{
		Hears: engine.On(engine.EvPlaying),
		Hear: func(c *ctx, e *ev) { // the first play of each Treasure each turn: first +1 Buy and +$1
			if !cameTrue(c) || !c.G.D(e.C).Treasure() || countPlayed(c, e.C) > 1 {
				return
			}
			a := c.As(e.P).Here() // "gets +1 Buy.", "gets +$1.", unnamed
			a.PlusBuys(1)
			a.PlusCoins(1)
		},
	})
	reg("Great Leader", rules{
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) { // after each Action played, +1 Action, unlogged
			if cameTrue(c) && c.G.D(e.C).Action() {
				c.Pl(e.P).Actions++
			}
		},
	})
	reg("Growth", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // gaining a Treasure gains a cheaper card
			if cameTrue(c) && c.G.D(e.C).Treasure() {
				c.As(e.P).ChooseGain(e.P, cheaperThan(c, e.C), discard)
			}
		},
	})
	reg("Harsh Winter", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // on your own turn: the pile's debt, "takes 2 debt."; else 2 debt onto it, unlogged
			i := c.G.PileOf(e.C)
			if !cameTrue(c) || e.P != c.G.Cur || i < 0 {
				return
			}
			if pile := &c.G.Piles[i]; pile.Debt > 0 {
				n := pile.Debt
				pile.Debt = 0
				c.As(e.P).TakeDebt(n)
			} else {
				pile.Debt = 2
			}
		},
	})
	reg("Flourishing Trade", rules{Reduce: func(g *engine.Game, _ inst, _ engine.CardID) int {
		if g.CameTrue("Flourishing Trade") {
			return 1
		}
		return 0
	}})
	reg("Divine Wind", rules{Play: func(c *ctx) { // the Supply's kingdom piles go, and 10 new ones come, unlogged
		g := c.G
		for i := range g.Piles {
			if p := &g.Piles[i]; p.Supply && slices.Contains(g.Kingdom, p.Card) {
				p.Supply = false
			}
		}
		var opts []engine.Option
		for _, k := range g.Unused(func(d *def) bool { return d.Kind == "kingdom" }) {
			opts = append(opts, engine.Option{Card: k, Inst: -1})
		}
		n := min(10, len(opts))
		g.Kingdom = nil
		for _, o := range c.Choose(c.P, engine.ToPick, opts, n, n) {
			g.AddKingdom(o.Card)
		}
	}})
	reg("Enlightenment", rules{}) // the engine plays Treasures as Actions once it came true
	reg("Kind Emperor", rules{
		Play:  func(c *ctx) { c.ChooseGain(c.P, action, hand) }, // also when its last Sun goes: "comes true.", "gains a Nobles."
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) { // "gains a Golem. (Kind Emperor)" into the hand
			if cameTrue(c) {
				phaseLine(c)
				c.As(e.P).Away().ChooseGain(e.P, action, hand)
			}
		},
	})
	reg("Panic", rules{
		Hears: engine.On(engine.EvPlay, engine.EvDiscard),
		Hear: func(c *ctx, e *ev) {
			if !cameTrue(c) || !c.G.D(e.C).Treasure() {
				return
			}
			switch {
			case e.Kind == engine.EvPlay:
				c.As(e.P).Away().PlusBuys(2)
			case e.From == inPlay && !e.Done:
				if c.As(e.P).ReturnTo(e.P, e.X) {
					e.Done = true
				}
			}
		},
	})
	reg("Progress", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "topdecks a Province." under its gain
			if cameTrue(c) && !e.Done && e.To != deck {
				c.As(e.P).Topdeck(e.P, one(e.X))
				e.Done, e.To = true, deck
			}
		},
	})
	reg("Rapid Expansion", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "sets a Wealthy Village aside."; next turn "plays a Wealthy Village. (Rapid Expansion)"
			if d := c.G.D(e.C); !cameTrue(c) || e.Done || !d.Action() && !d.Treasure() {
				return
			}
			if xs := c.As(e.P).SetAside(e.P, one(e.X)); len(xs) == 1 {
				e.Done = true
				c.As(e.P).Later().Aside = xs
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			for _, x := range c.In(aside, d.Aside) {
				c.PlayWith(x)
			}
			return true
		},
	})
	reg("Sickness", rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) { // "gains a Curse. (Sickness)" onto the deck, or "discards 3 cards."
			if !cameTrue(c) {
				return
			}
			phaseLine(c)
			a := c.As(e.P)
			if a.Mode1(e.P, engine.Mode{Label: "gain a Curse onto your deck", Line: "GAIN_WITH"}, mode("discard 3 cards", "DISCARD")) == 0 {
				a.Away().Gain(e.P, c.Card("Curse"), deck)
				return
			}
			n := min(3, len(c.Hand(e.P)))
			a.Discard(e.P, pick(a, e.P, engine.ToDiscard, hand, nil, n, n))
		},
	})

	// Events
	reg("Amass", rules{Buy: func(c *ctx) {
		if !hasActionInPlay(c, c.P) {
			c.ChooseGain(c.P, and(action, c.CostsUpTo(5)), discard)
		}
	}})
	reg("Asceticism", rules{Buy: func(c *ctx) { // "loses 1 Coin.", "trashes an Estate."
		n := c.Amount(c.P, 0, min(c.Pl(c.P).Coins, len(c.Hand(c.P))), "LOSES_COIN", "LOSES_COINS")
		if n > 0 {
			c.LoseCoins(n)
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, n, n))
		}
	}})
	reg("Credit", rules{Buy: func(c *ctx) { // "gains a Quartermaster.", "takes 5 debt."
		if x := c.ChooseGain(c.P, and(func(d *def) bool { return d.Action() || d.Treasure() }, c.CostsUpTo(8)), discard); x >= 0 {
			if n := c.CostOf(c.G.CardOf(x)).Coins; n > 0 {
				c.TakeDebt(n)
			}
		}
	}})
	reg("Foresight", rules{
		Buy: func(c *ctx) { // "reveals an Island.", "sets an Island aside."; after the Clean-up draw "puts an Island into their hand."
			all, found := c.RevealUntil(c.P, action, 1)
			if xs := c.SetAside(c.P, found); len(xs) > 0 {
				c.AddWatch(engine.On(engine.EvTurnEnd), int(xs[0])).Long = true
			}
			c.Discard(c.P, c.In(revealed, all))
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P == c.P {
				c.At(0).ToHand(c.P, c.In(aside, one(inst(w.N))), "PUT_IN_HAND") // after the Clean-up draw, at the top level
				return true
			}
			return false
		},
	})
	reg("Kintsugi", rules{
		Buy: func(c *ctx) { // gaining a Gold this game is kept on the player's Mat
			x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
			if x < 0 {
				return
			}
			c.Trash(c.P, one(x))
			if c.Pl(c.P).Mat[c.Src] > 0 {
				c.ChooseGain(c.P, costUp(c, x, 2), discard)
			}
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if c.G.D(e.C).Name == "Gold" {
				c.Pl(e.P).Mat[c.Src]++
			}
		},
	})
	reg("Practice", rules{Buy: func(c *ctx) { // "plays a Siren.", "plays a Siren again."
		if x := pick1(c, c.P, engine.ToPlay, hand, action, true); x >= 0 {
			c.PlayCard(x, 0)
			c.PlayCard(x, 1)
		}
	}})
	reg("Sea Trade", rules{Buy: func(c *ctx) {
		n := c.Count(c.P, inPlay, action)
		c.Draw(n)
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, n))
	}})
	reg("Receive Tribute", rules{Buy: func(c *ctx) {
		if len(c.T2().Gained) < 3 {
			return
		}
		got := map[engine.CardID]bool{}
		for range 3 {
			o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(func(d *def) bool {
				return d.Action() && !got[d.ID] && c.Count(c.P, inPlay, named(d.Name)) == 0
			}), true)
			if o == nil {
				return
			}
			got[o.Card] = true
			c.Gain(c.P, o.Card, discard)
		}
	}})
	reg("Gather", rules{Buy: func(c *ctx) { // "gains a Silver.", "gains an Envoy.", "gains a Duchy."
		for _, n := range []int{3, 4, 5} {
			c.ChooseGain(c.P, func(d *def) bool { return c.CostOf(d.ID) == engine.Upto(n) }, discard)
		}
	}})
	reg("Continue", rules{ // once per turn
		CanBuy: func(g *engine.Game, p int) bool { return g.T2.Used[g.T.MustCard("Continue")] == 0 },
		Buy: func(c *ctx) { // "gains a Poet.", "plays a Poet.", "gets +1 Action.", "gets +1 Buy."
			c.T2().Used[c.Src]++
			if x := c.ChooseGain(c.P, and(action, not(attack), c.CostsUpTo(4)), discard); x >= 0 {
				c.T2().Back = true
				if len(c.In(discard, one(x))) == 1 { // not one Progress topdecked or Rapid Expansion set aside
					c.PlayNow(x)
				}
			}
			c.PlusActions(1)
			c.PlusBuys(1)
		},
	})
}
