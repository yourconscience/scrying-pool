package cards

import (
	"math/bits"
	"slices"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Menagerie: Exile, Horses, the Events and the Ways. Simplifications: a Way
// is offered only for the Actions played in the Action phase; Way of the
// Chameleon writes no "would draw" lines and leaves an Attack out; Snowy
// Village ignores later +Actions without a line; Sleigh only takes a card
// gained to the discard pile; Wayfarer only follows the gains of the player
// whose turn it is; Seize the Day's extra turn is logged as Mission's.

const exile = engine.Exile

// exileFrom moves the top card of a Supply pile to a player's Exile mat: "exiles a Gold"; -1 if it can't.
func exileFrom(c *ctx, p int, card engine.CardID) inst {
	i := c.G.PileOf(card)
	if i < 0 || c.G.Top(i) != card {
		return -1
	}
	s := c.G.Piles[i].Cards
	x := s[len(s)-1]
	c.G.Move(x, engine.Loc{Zone: exile, Owner: int16(p)})
	c.Log("EXILE", engine.Seat(p), []engine.CardID{card})
	return x
}

// exileCards moves a player's cards to their Exile mat: "exiles an Estate".
func exileCards(c *ctx, p int, xs []inst) []inst { return c.MoveTo(p, xs, exile, "EXILE") }

// exiled: player p has a copy of the card in Exile.
func exiled(c *ctx, p int, card engine.CardID) bool {
	return slices.ContainsFunc(c.Zone(p, exile), func(x inst) bool { return c.G.CardOf(x) == card })
}

func horses(g *engine.Game) { g.Need("Horse", 30) }

// nowOrLater is a Duration that does its effect either now or at the start of the next turn.
func nowOrLater(effect func(c *ctx), line string) rules {
	return rules{
		Play: func(c *ctx) {
			if c.Mode1(c.P, mode("now", line), mode("at the start of your next turn", "")) == 0 {
				effect(c)
			} else {
				c.Stay()
			}
		},
		Next: func(c *ctx, _ *engine.Dur) bool { effect(c); return true },
	}
}

// playAside plays the cards an Event or Way set aside for this turn: "plays a Gold. (Reap)".
func playAside(c *ctx, d *engine.Dur) bool {
	for _, x := range c.In(aside, d.Aside) {
		c.PlayWith(x)
	}
	return true
}

// backToPile returns the card a Way played to its pile, or logs why it can't:
// "returns a Port to the Port pile", "has no pile to return to".
func backToPile(c *ctx, way string) bool {
	card := []engine.CardID{c.G.CardOf(c.Me)}
	switch {
	case c.G.PileOf(card[0]) < 0:
		c.Log(way+"_FAILS_NO_PILE", card)
	case c.InPlace || c.G.Where(c.Me).Zone != inPlay:
		c.Log(way+"_FAILS_NOT_IN_PLAY", card)
	default:
		return c.ReturnTo(c.P, c.Me)
	}
	return false
}

// twoTypes: a card with two or more types, e.g. an Action - Attack (Falconer).
func twoTypes(d *def) bool { return bits.OnesCount64(uint64(d.T&(engine.TEvent-1))) >= 2 }

// reactPlaying is a Reaction that plays itself: "reacts with a Black Cat.", "plays a Black Cat."
func reactPlaying(c *ctx, _ *ev) bool {
	c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
	if c.G.Cur == c.P && c.T2().Phase == engine.StartPhase {
		c.PlayBlank(c.Me) // "plays a Stowaway. (|)" under a gain at the start of their own turn
		return true
	}
	c.PlayNow(c.Me)
	return true
}

func init() {
	horse := func(c *ctx) engine.CardID { return c.Card("Horse") }

	reg("Horse", rules{Play: func(c *ctx) { c.Draw(2); c.PlusActions(1); c.ReturnTo(c.P, c.Me) }})

	// Actions and Treasures
	reg("Animal Fair", rules{
		Play: func(c *ctx) {
			c.PlusCoins(4)
			if n := c.G.EmptyPiles(); n > 0 {
				c.PlusBuys(n)
			}
		},
		PayWith: action,
	})
	reg("Barge", nowOrLater(func(c *ctx) { c.Draw(3); c.PlusBuys(1) }, "DRAW"))
	reg("Black Cat", rules{
		Play: func(c *ctx) {
			c.Draw(2)
			if c.G.Cur != c.P {
				c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
			}
		},
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return e.P != c.P && c.G.D(e.C).Victory() },
		React:    reactPlaying,
	})
	reg("Bounty Hunter", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		x := pick1(c, c.P, engine.ToExile, hand, nil, false)
		if x < 0 {
			return
		}
		had := exiled(c, c.P, c.G.CardOf(x))
		exileCards(c, c.P, one(x))
		if !had {
			c.PlusCoins(3)
		}
	}})
	reg("Camel Train", rules{
		Play: func(c *ctx) {
			if o := c.ChooseOne(c.P, engine.ToExile, c.SupplyOptions(not(victory)), false); o != nil {
				exileFrom(c, c.P, o.Card)
			}
		},
		OnGain: func(c *ctx, _ *ev) { exileFrom(c, c.P, c.Card("Gold")) },
	})
	reg("Cardinal", rules{Play: func(c *ctx) { // the other player "reveals a Silver and an Estate.", "exiles a Silver.", "discards an Estate."
		c.PlusCoins(2)
		c.Attack(func(v int) {
			top := c.FromTop(v, 2, "REVEAL")
			var ok []inst
			for _, x := range top {
				if costs3to6(c)(c.G.Def(x)) {
					ok = append(ok, x)
				}
			}
			if len(ok) > 0 {
				exileCards(c, v, pickOf(c, v, engine.ToExile, ok, 1, 1))
			}
			c.Discard(v, c.In(revealed, top))
		})
	}})
	reg("Cavalry", rules{
		Play: func(c *ctx) { c.GainN(c.P, horse(c), 2, discard, "GAIN") },
		OnGain: func(c *ctx, _ *ev) { // "draws 2 cards.", "gets +1 Buy.", then back to the Action phase
			c.Draw(2)
			c.PlusBuys(1)
			if c.G.Cur == c.P && c.T2().Phase == engine.BuyPhase {
				c.T2().Back = true
			}
		},
		Setup: horses,
	})
	reg("Coven", rules{Play: func(c *ctx) { // the other player "exiles a Curse.", or with none left "discards 2 Curses from Exile."
		c.PlusActions(1)
		c.PlusCoins(2)
		c.Attack(func(v int) {
			if exileFrom(c, v, c.Card("Curse")) < 0 {
				var curses []inst
				for _, x := range c.Zone(v, exile) {
					if c.G.Def(x).Name == "Curse" {
						curses = append(curses, x)
					}
				}
				c.MoveTo(v, curses, discard, "DISCARD_FROM_EXILE")
			}
		})
	}})
	reg("Destrier", rules{
		Play: func(c *ctx) { c.Draw(2); c.PlusActions(1) },
		SelfCost: func(g *engine.Game, _ int, cost *engine.Cost) {
			cost.Coins = max(0, cost.Coins-len(g.T2.Gained))
		},
	})
	reg("Displace", rules{Play: func(c *ctx) { // "exiles an Estate.", "gains a Hostelry."
		x := pick1(c, c.P, engine.ToExile, hand, nil, false)
		if x < 0 {
			return
		}
		ok := and(costUp(c, x, 2), not(named(c.G.Def(x).Name)))
		exileCards(c, c.P, one(x))
		c.ChooseGain(c.P, ok, discard)
	}})
	reg("Falconer", rules{
		Play: func(c *ctx) { // gains to hand, logged as "gains a Silver."
			c.ChooseGain(c.P, func(d *def) bool { return c.CostOf(d.ID).Less(c.CostOf(c.Src)) }, hand)
		},
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return twoTypes(c.G.D(e.C)) },
		React:    reactPlaying,
	})
	reg("Fisherman", rules{
		Play: func(c *ctx) { c.Draw(1); c.PlusActions(1); c.PlusCoins(1) },
		SelfCost: func(g *engine.Game, p int, cost *engine.Cost) {
			if len(g.P[p].Z[discard]) == 0 {
				cost.Coins = max(0, cost.Coins-3)
			}
		},
	})
	reg("Gatekeeper", rules{ // d.N is 1 while its Attack lasts
		Play: func(c *ctx) {
			d := c.Stay()
			c.Attack(func(int) { d.N = 1 })
		},
		Next:  func(c *ctx, d *engine.Dur) bool { d.N = 0; c.PlusCoins(3); return true },
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // the other player "gains a Silver.", "exiles a Silver."
			if e.P == c.P || !c.G.D(e.C).Is(engine.TAction|engine.TTreasure) || !stillGained(c, e) || exiled(c, e.P, e.C) {
				return
			}
			for _, d := range c.Durs() {
				if d.N == 1 {
					e.Done = true
					exileCards(c.Here(), e.P, one(e.X))
					return
				}
			}
		},
	})
	reg("Goatherd", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, true)))
		if n := c.G.PrevTrashed(c.Other()); n > 0 {
			c.Draw(n)
		}
	}})
	reg("Groom", rules{Play: func(c *ctx) { // "gains a Farmers' Market.", "gains a Horse."
		x := c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		if x < 0 {
			return
		}
		d := c.G.Def(x)
		if c.G.IsAction(x) { // an inheriting Estate too: "gains an Estate.", "gains a Horse.", "draws a card."
			c.Gain(c.P, horse(c), discard)
		}
		if d.Treasure() {
			c.Gain(c.P, c.Card("Silver"), discard)
		}
		if d.Victory() {
			c.Draw(1)
			c.PlusActions(1)
		}
	}, Setup: horses})
	reg("Hostelry", rules{
		Play: func(c *ctx) { c.Draw(1); c.PlusActions(2) },
		OnGain: func(c *ctx, _ *ev) { // "discards and reveals 2 Coppers.", "gains 2 Horses."
			xs := c.DiscardAs(c.P, pick(c, c.P, engine.ToDiscard, hand, treasure, 0, len(c.Hand(c.P))), "DISCARD_REVEALED")
			c.GainN(c.P, horse(c), len(xs), discard, "GAIN")
		},
		Setup: horses,
	})
	reg("Hunting Lodge", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(2)
		line := "DISCARD"
		if len(c.Hand(c.P)) == 0 {
			line = "DRAW"
		}
		if c.Yes(c.P, "discard your hand for +5 Cards", line) {
			c.Discard(c.P, append([]inst{}, c.Hand(c.P)...))
			c.Draw(5)
		}
	}})
	reg("Kiln", rules{
		Play: func(c *ctx) { c.PlusCoins(2); c.AddWatch(engine.On(engine.EvPlaying), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "plays a Sacred Grove.", "gains a Sacred Grove."
			if e.P != w.P {
				return false
			}
			if c.Yes(w.P, "gain a copy of it", "GAIN") {
				c.Gain(w.P, e.C, discard)
			}
			return true
		},
	})
	reg("Livery", rules{
		Play: func(c *ctx) { c.PlusCoins(3); c.AddWatch(engine.On(engine.EvGain), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "gains a Duchy.", "gains a Horse."
			if e.P == w.P && c.CostOf(e.C).Coins >= 4 {
				c.Gain(w.P, horse(c), discard)
			}
			return false
		},
		Setup: horses,
	})
	reg("Mastermind", rules{
		Play: func(c *ctx) { c.Stay() },
		Next: func(c *ctx, _ *engine.Dur) bool { // "plays a Mining Village. (Mastermind)", three times
			if x := pick1(c, c.P, engine.ToPlay, hand, action, true); x >= 0 {
				for k := range 3 {
					c.PlayCardWith(x, k)
				}
			}
			return true
		},
	})
	reg("Paddock", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		c.GainN(c.P, horse(c), 2, discard, "GAIN")
		if n := c.G.EmptyPiles(); n > 0 {
			c.PlusActions(n)
		}
	}, Setup: horses})
	reg("Sanctuary", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusBuys(1)
		exileCards(c, c.P, one(pick1(c, c.P, engine.ToExile, hand, nil, true)))
	}})
	reg("Scrap", rules{Play: func(c *ctx) { // "trashes an Estate.", "draws a Copper.", "gets +1 Buy."
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		n := min(6, c.CostOf(c.G.CardOf(x)).Coins)
		c.Trash(c.P, one(x))
		ks := c.Modes(c.P, n, n, mode("+1 Card", "DRAW"), mode("+1 Action", "GETS_ACTION"), mode("+1 Buy", "GETS_BUY"),
			mode("+$1", "GETS_COIN"), mode("gain a Silver", "GAIN"), mode("gain a Horse", "GAIN"))
		slices.Sort(ks)
		for _, k := range ks {
			switch k {
			case 0:
				c.Draw(1)
			case 1:
				c.PlusActions(1)
			case 2:
				c.PlusBuys(1)
			case 3:
				c.PlusCoins(1)
			case 4:
				c.Gain(c.P, c.Card("Silver"), discard)
			case 5:
				c.Gain(c.P, horse(c), discard)
			}
		}
	}, Setup: horses})
	reg("Sheepdog", rules{
		Play:     func(c *ctx) { c.Draw(2) },
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return e.P == c.P },
		React:    func(c *ctx, _ *ev) bool { c.PlayAs(c.Me, "REACTS_PLAYING"); return true }, // "reacts with a Sheepdog, playing it."
	})
	reg("Sleigh", rules{
		Play:     func(c *ctx) { c.GainN(c.P, horse(c), 2, discard, "GAIN") },
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return e.P == c.P && e.To == discard && stillGained(c, e) },
		React: func(c *ctx, e *ev) bool { // "reacts with a Sleigh.", "discards a Sleigh.", "looks at" the discard pile, "puts a Gold into their hand."
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			c.Discard(c.P, one(c.Me))
			c.LookAt(c.P, c.Zone(c.P, discard))
			e.Done = true
			if c.Mode1(c.P, mode("put it into your hand", "PUT_IN_HAND"), mode("put it onto your deck", "TOPDECK")) == 0 {
				c.ToHand(c.P, one(e.X), "PUT_IN_HAND")
			} else {
				c.Topdeck(c.P, one(e.X))
			}
			return true
		},
		Setup: horses,
	})
	reg("Snowy Village", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(4)
		c.PlusBuys(1)
		c.T2().NoActions = true
	}})
	reg("Stockpile", rules{Coins: 3, Alone: true, Play: func(c *ctx) { c.PlusBuys(1); exileCards(c, c.P, one(c.Me)) }})
	reg("Supplies", rules{Coins: 1, Alone: true, Play: func(c *ctx) { c.Gain(c.P, horse(c), deck) }, Setup: horses})
	vg := nowOrLater(func(c *ctx) { c.Draw(1); c.PlusActions(2) }, "DRAW")
	vg.OnDiscard = func(c *ctx, e *ev) { // "discards a Village Green.", "plays a Village Green."
		if !e.Cleanup && c.G.Where(c.Me).Zone == discard && c.Yes(c.P, "play it", "PLAY") {
			c.PlayNow(c.Me)
		}
	}
	reg("Village Green", vg)
	reg("Wayfarer", rules{
		Play: func(c *ctx) {
			c.Draw(3)
			if c.Yes(c.P, "gain a Silver", "GAIN") {
				c.Gain(c.P, c.Card("Silver"), discard)
			}
		},
		SelfCost: func(g *engine.Game, _ int, cost *engine.Cost) {
			for i := len(g.T2.Gained) - 1; i >= 0; i-- {
				if d := g.D(g.T2.Gained[i]); d.Name != "Wayfarer" {
					*cost = d.Cost
					if d.Rules != nil && d.Rules.SelfCost != nil { // as it costs now: a Fisherman's $2 with an empty discard pile
						d.Rules.SelfCost(g, g.Cur, cost)
					}
					return
				}
			}
		},
	})

	// Ways: the Way's text instead of the card's, with the card as "this"
	way := func(name string, play func(c *ctx)) { reg("Way of the "+name, rules{Play: play}) }
	way("Butterfly", func(c *ctx) { // "returns a Minion to the Minion pile.", "gains a Border Village."
		ok := costExactly(c, c.Me, 1)
		if c.G.PileOf(c.G.CardOf(c.Me)) >= 0 && !c.InPlace && c.G.Where(c.Me).Zone == inPlay && !c.Yes(c.P, "return it to its pile", "RETURN_TO") {
			c.Log("WAY_OF_THE_BUTTERFLY_FAILS_INTENTIONAL", engine.Seat(c.P), []engine.CardID{c.G.CardOf(c.Me)})
			return
		}
		if backToPile(c, "WAY_OF_THE_BUTTERFLY") {
			c.ChooseGain(c.P, ok, discard)
		}
	})
	way("Camel", func(c *ctx) { exileFrom(c, c.P, c.Card("Gold")) })
	way("Chameleon", func(c *ctx) {
		if d := c.G.Def(c.Me); d.Rules.Play != nil {
			s := c.Swapped()
			s.Src = d.ID
			d.Rules.Play(s)
		}
	})
	reg("Way of the Frog", rules{
		Play: func(c *ctx) { c.PlusActions(1); c.AddWatch(engine.On(engine.EvDiscard), int(c.Me)) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "begins their Cleanup phase.", "topdecks an Ironworks."
			if e.X != inst(w.N) || e.From != inPlay {
				return false
			}
			if c.G.Where(e.X) == (engine.Loc{Zone: discard, Owner: int16(e.P)}) { // a Scheme may have topdecked it
				c.Topdeck(e.P, one(e.X))
			}
			return true
		},
	})
	way("Goat", func(c *ctx) { c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false))) })
	way("Horse", func(c *ctx) { c.Draw(2); c.PlusActions(1); backToPile(c, "WAY_OF_THE_HORSE") })
	way("Mole", func(c *ctx) {
		c.PlusActions(1)
		c.Discard(c.P, append([]inst{}, c.Hand(c.P)...))
		c.Draw(3)
	})
	way("Monkey", func(c *ctx) { c.PlusBuys(1); c.PlusCoins(1) })
	reg("Way of the Mouse", rules{
		Play: func(c *ctx) { // "plays a Ratcatcher using Way of the Mouse.", "plays an Herb Gatherer."
			if i := c.G.PileOf(c.G.Mouse); i >= 0 && c.G.Top(i) == c.G.Mouse {
				s := c.G.Piles[i].Cards
				c.PlayHere(s[len(s)-1])
			}
		},
		Setup: func(g *engine.Game) { // an unused non-Duration Action costing $2 or $3, set aside as a pile of one
			us := g.Unused(func(d *def) bool {
				return d.Action() && !duration(d) && !strings.Contains(d.Name, " / ") && d.Rules != nil && (d.Cost == engine.Upto(2) || d.Cost == engine.Upto(3)) // not a split pile such as the Augurs
			})
			if len(us) == 0 {
				return
			}
			g.Mouse = us[g.Rand(len(us))]
			g.AddPile(g.Mouse, 1, false)
			if r := g.D(g.Mouse).Rules; r.Setup != nil {
				r.Setup(g)
			}
		},
	})
	way("Mule", func(c *ctx) { c.PlusActions(1); c.PlusCoins(1) })
	way("Otter", func(c *ctx) { c.Draw(2) })
	way("Owl", func(c *ctx) { drawTo(c, 6) })
	way("Ox", func(c *ctx) { c.PlusActions(2) })
	way("Pig", func(c *ctx) { c.Draw(1); c.PlusActions(1) })
	way("Rat", func(c *ctx) { // "discards a Copper.", "gains an Overlord."
		if x := pick1(c, c.P, engine.ToDiscard, hand, treasure, true); x >= 0 && len(c.Discard(c.P, one(x))) == 1 {
			c.Gain(c.P, c.G.CardOf(c.Me), discard)
		}
	})
	reg("Way of the Seal", rules{
		Play: func(c *ctx) { c.PlusCoins(1); c.AddWatch(engine.On(engine.EvGain), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "buys and gains an Estate.", "topdecks an Estate with Way of the Seal."
			if e.P == w.P && e.To != deck && stillGained(c, e) && c.Yes(w.P, "put it onto your deck", "TOPDECK_WITH") {
				e.Done = true
				c.MoveTo(w.P, one(e.X), deck, "TOPDECK_WITH", engine.Name(c.Src))
			}
			return false
		},
	})
	way("Sheep", func(c *ctx) { c.PlusCoins(2) })
	way("Squirrel", func(c *ctx) { c.T2().HandSize += 2 }) // silent: "draws 7 cards." in Clean-up
	reg("Way of the Turtle", rules{
		Play: func(c *ctx) { // "sets a Crown aside.", and at the start of the next turn "plays a Crown. (Way of the Turtle)"
			if c.InPlace || c.G.Where(c.Me).Zone != inPlay || len(c.SetAside(c.P, one(c.Me))) == 0 {
				c.Log("WAY_OF_THE_TURTLE_FAILS_TO_SET_ASIDE", []engine.CardID{c.G.CardOf(c.Me)})
				return
			}
			c.Later().Aside = one(c.Me)
		},
		Next: playAside,
	})
	way("Worm", func(c *ctx) { exileFrom(c, c.P, c.Card("Estate")) })

	// Events
	reg("Alliance", rules{Buy: func(c *ctx) {
		for _, n := range []string{"Province", "Duchy", "Estate", "Gold", "Silver", "Copper"} {
			c.Gain(c.P, c.Card(n), discard)
		}
	}})
	reg("Banish", rules{Buy: func(c *ctx) { // "exiles 4 Coppers."
		x := pick1(c, c.P, engine.ToExile, hand, nil, true)
		if x < 0 {
			return
		}
		var same []inst
		for _, y := range c.Hand(c.P) {
			if y != x && c.G.CardOf(y) == c.G.CardOf(x) {
				same = append(same, y)
			}
		}
		exileCards(c, c.P, append(one(x), pickOf(c, c.P, engine.ToExile, same, 0, len(same))...))
	}})
	reg("Bargain", rules{Buy: func(c *ctx) {
		c.ChooseGain(c.P, and(not(victory), c.CostsUpTo(5)), discard)
		c.Gain(c.Other(), horse(c), discard)
	}, Setup: horses})
	reg("Commerce", rules{Buy: func(c *ctx) { // "gains 2 Golds."
		seen := map[engine.CardID]bool{}
		for _, id := range c.T2().Gained {
			seen[id] = true
		}
		if len(seen) == 0 {
			c.Log("COMMERCE_FAIL", engine.Seat(c.P))
			return
		}
		c.GainN(c.P, c.Card("Gold"), len(seen), discard, "GAIN")
	}})
	reg("Delay", rules{
		Buy: func(c *ctx) { // "sets a Develop aside.", and at the start of the next turn "plays a Develop. (Delay)"
			if x := pick1(c, c.P, engine.ToSetAside, hand, action, true); x >= 0 {
				c.Later().Aside = c.SetAside(c.P, one(x))
			}
		},
		Next: playAside,
	})
	reg("Demand", rules{Buy: func(c *ctx) { // "gains a Horse.", "gains an Urchin.", both onto the deck
		c.Gain(c.P, horse(c), deck)
		c.ChooseGain(c.P, c.CostsUpTo(4), deck)
	}, Setup: horses})
	reg("Desperation", rules{
		CanBuy: once("Desperation"),
		Buy: func(c *ctx) {
			c.T2().Used[c.Src]++
			if c.Gain(c.P, c.Card("Curse"), discard) >= 0 {
				c.PlusBuys(1)
				c.PlusCoins(2)
			}
		},
	})
	reg("Enclave", rules{Buy: func(c *ctx) {
		c.Gain(c.P, c.Card("Gold"), discard)
		exileFrom(c, c.P, c.Card("Duchy"))
	}})
	reg("Enhance", rules{Buy: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToTrash, hand, not(victory), true); x >= 0 {
			ok := costUp(c, x, 2)
			c.Trash(c.P, one(x))
			c.ChooseGain(c.P, ok, discard)
		}
	}})
	reg("Gamble", rules{Buy: func(c *ctx) { // "discards a Relic.", "plays a Relic. (+$2)"
		c.PlusBuys(1)
		x := c.DiscardTop(c.P)
		if x < 0 {
			return
		}
		d := c.G.Def(x)
		line := "PLAY"
		if !d.Action() {
			line = "PLAY_TREASURES_FOR"
		}
		if (d.Action() || d.Treasure()) && c.Yes(c.P, "play it", line) {
			c.PlayNow(x)
		}
	}})
	reg("Invest", rules{
		Buy: func(c *ctx) { // "exiles an Upgrade.", "draws 2 cards when an opponent gains or invests in Upgrade."
			o := c.ChooseOne(c.P, engine.ToExile, c.SupplyOptions(action), false)
			if o == nil {
				return
			}
			x := exileFrom(c, c.P, o.Card)
			c.Log("IS_INVESTED", engine.Seat(c.P), engine.Name(o.Card), 2)
			for _, w := range c.G.Watches() { // the other player's Invests in it: "draws 2 cards. (Invest)"
				if w.C == c.Src && w.P != c.P && c.G.CardOf(inst(w.N)) == o.Card && c.G.Where(inst(w.N)) == (engine.Loc{Zone: exile, Owner: int16(w.P)}) {
					c.Deeper().Away().DrawFor(w.P, 2)
				}
			}
			c.AddWatch(engine.On(engine.EvGain), int(x)).Long = true
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // the other player "buys and gains an Upgrade.", "draws 2 cards. (Invest)"
			x := inst(w.N)
			if c.G.Where(x) != (engine.Loc{Zone: exile, Owner: int16(w.P)}) {
				return true
			}
			if e.P != w.P && e.C == c.G.CardOf(x) {
				c.Away().DrawFor(w.P, 2)
			}
			return false
		},
	})
	reg("March", rules{Buy: func(c *ctx) { // "looks at 3 Coppers, 3 Estates and a Peasant.", "plays a Peasant."
		c.LookAt(c.P, c.Zone(c.P, discard))
		if x := pick1(c, c.P, engine.ToPlay, discard, action, true); x >= 0 {
			c.PlayNow(x)
		}
	}})
	reg("Populate", rules{Buy: func(c *ctx) {
		for i := range c.G.Piles {
			if top := c.G.Top(i); c.G.Piles[i].Supply && top != 0 && c.G.D(top).Action() {
				c.Gain(c.P, top, discard)
			}
		}
	}})
	reg("Pursue", rules{Buy: func(c *ctx) { // "names Copper.", "reveals 2 Coppers, a Tunnel and a Flagship.", "topdecks 2 Coppers.", "discards a Tunnel and a Flagship."
		c.PlusBuys(1)
		name := c.NameCard(c.P)
		c.Log("NAMES", engine.Seat(c.P), engine.Name(name))
		top := c.FromTop(c.P, 4, "REVEAL")
		var keep []inst
		for _, x := range top {
			if c.G.CardOf(x) == name {
				keep = append(keep, x)
			}
		}
		c.Topdeck(c.P, keep)
		c.Discard(c.P, c.In(revealed, top))
	}})
	reg("Reap", rules{
		Buy: func(c *ctx) { // "gains a Gold setting it aside.", and at the start of the next turn "plays a Gold. (Reap)"
			if x := c.GainAs(c.P, c.Card("Gold"), aside, "GAIN_SETTING_ASIDE"); x >= 0 {
				c.Later().Aside = one(x)
			}
		},
		Next: playAside,
	})
	reg("Ride", rules{Buy: func(c *ctx) { c.Gain(c.P, horse(c), discard) }, Setup: horses})
	reg("Seize the Day", rules{ // once per game, kept on the player's Mat
		CanBuy: func(g *engine.Game, p int) bool { return g.P[p].Mat[g.T.MustCard("Seize the Day")] == 0 },
		Buy: func(c *ctx) {
			c.Pl(c.P).Mat[c.Src]++
			c.Log("MISSION_SUCCESS", engine.Seat(c.P))
			c.ExtraTurn()
		},
	})
	reg("Stampede", rules{Buy: func(c *ctx) { // "gains 5 Horses." onto the deck
		if n := len(c.Zone(c.P, inPlay)); n > 5 {
			c.Log("STAMPEDE_FAIL", engine.Seat(c.P), n)
			return
		}
		c.GainN(c.P, horse(c), 5, deck, "GAIN")
	}, Setup: horses})
	reg("Toil", rules{Buy: func(c *ctx) {
		c.PlusBuys(1)
		if x := pick1(c, c.P, engine.ToPlay, hand, action, true); x >= 0 {
			c.PlayNow(x)
		}
	}})
	reg("Transport", rules{Buy: func(c *ctx) { // "exiles a Library.", or "topdecks a Library."
		if c.Count(c.P, exile, action) > 0 && c.Mode1(c.P, mode("exile an Action card from the Supply", "EXILE"), mode("put an Action card from Exile onto your deck", "TOPDECK")) == 1 {
			c.Topdeck(c.P, one(pick1(c, c.P, engine.ToTopdeck, exile, action, false)))
			return
		}
		if o := c.ChooseOne(c.P, engine.ToExile, c.SupplyOptions(action), false); o != nil {
			exileFrom(c, c.P, o.Card)
		}
	}})
}
