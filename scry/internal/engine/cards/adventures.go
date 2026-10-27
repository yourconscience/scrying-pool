package cards

import (
	"slices"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Adventures: Reserve cards on the Tavern mat, Travellers exchanged in
// Clean-up, the tokens, and the Events. Simplifications: Alms can't be bought
// with a Treasure in play, Mission only needs this turn not to be an extra one,
// and Inheritance can't take a Reserve card (its Estate on the Tavern mat
// couldn't be called).

const tavern = engine.Tavern

// reserve puts this card on its owner's Tavern mat: "puts a Ratcatcher on their Tavern mat".
func reserve(c *ctx) {
	if c.G.Where(c.Me).Zone == inPlay {
		c.PutOnMat(c.P, one(c.Me), tavern)
	}
}

// call asks whether to call this card from its owner's Tavern mat, and calls it.
func call(c *ctx) bool {
	if c.G.Where(c.Me) != (engine.Loc{Zone: tavern, Owner: int16(c.P)}) || len(pickOf(c, c.P, engine.ToCall, one(c.Me), 0, 1)) == 0 {
		return false
	}
	return c.Call(c.Me)
}

// startCall is a Reserve card called at the start of its owner's turn: "calls a Ratcatcher", "trashes a Copper. (Ratcatcher)".
func startCall(play, called func(c *ctx)) rules {
	return rules{
		Play:  func(c *ctx) { play(c); reserve(c) },
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) {
			if e.P != c.P || c.G.Where(c.Me) != (engine.Loc{Zone: tavern, Owner: int16(c.P)}) {
				return
			}
			phaseLine(c) // "starts their turn." whether or not it's called
			if call(c) {
				called(c)
			}
		},
	}
}

// afterAction is a Reserve card called directly after an Action resolves, on that Action's line:
// "plays a Treasurer.", "calls a Royal Carriage.", "plays a Treasurer again."
func afterAction(play func(c *ctx), ok func(c *ctx, e *ev) bool, called func(c *ctx, e *ev)) rules {
	return rules{
		Play:  func(c *ctx) { play(c); reserve(c) },
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) {
			if e.P != c.P || c.G.Cur != c.P || !c.G.D(e.C).Action() || !ok(c, e) {
				return
			}
			if c = c.At(e.Level - 1).Here(); call(c) {
				called(c, e)
			}
		},
	}
}

// actionPiles are the Action Supply piles a token can go to; free: none with this player's tokens (Teacher).
func actionPiles(c *ctx, free bool) []engine.Option {
	var out []engine.Option
	for i, pile := range c.G.Piles {
		if !pile.Supply || !c.G.D(pile.Card).Action() {
			continue
		}
		taken := false
		for k := engine.TokCard; k <= engine.TokTrash; k++ {
			taken = taken || c.Pl(c.P).Tokens[k] == i
		}
		if !free || !taken {
			out = append(out, engine.Option{Card: pile.Card, Inst: -1})
		}
	}
	return out
}

// tokenTo moves one of the player's tokens to an Action Supply pile they pick: "moves +Action token to the Torturer pile".
func tokenTo(c *ctx, k int, free bool) {
	if o := c.ChooseOne(c.P, engine.ToPick, actionPiles(c, free), false); o != nil {
		c.MoveToken(k, c.G.PileOf(o.Card))
	}
}

// traveller exchanges this card for the next one of its line when it is
// discarded from play in Clean-up: "returns a Peasant.", "receives a Soldier."
func traveller(next string, r rules) rules {
	r.OnDiscard = func(c *ctx, e *ev) {
		card := c.Card(next)
		if i := c.G.PileOf(card); !e.Cleanup || i < 0 || c.G.Top(i) != card {
			return
		}
		c.Emit() // "begins their Cleanup phase" comes first, exchanged or not
		if c.Yes(c.P, "exchange it for a "+next, "EXCHANGE_RETURN") {
			c.Exchange(c.Me, card)
		}
	}
	return r
}

func travellers(names ...string) func(g *engine.Game) {
	return func(g *engine.Game) {
		for _, n := range names {
			g.Need(n, 5)
		}
	}
}

// once is an Event that can be bought once per turn; buying it counts in T2.Used.
func once(name string) func(g *engine.Game, p int) bool {
	return func(g *engine.Game, p int) bool { return g.T2.Used[g.T.MustCard(name)] == 0 }
}

func inPlayOf(g *engine.Game, p int, ok func(*def) bool) bool {
	for _, x := range g.P[p].Z[inPlay] {
		if ok(g.Def(x)) {
			return true
		}
	}
	return false
}

func costs3or4(c *ctx) func(*def) bool {
	return func(d *def) bool { k := c.CostOf(d.ID); return k == engine.Upto(3) || k == engine.Upto(4) }
}

func init() {
	silver := func(c *ctx) engine.CardID { return c.Card("Silver") }

	// Reserve cards
	cotr := afterAction(func(*ctx) {}, func(*ctx, *ev) bool { return true }, func(c *ctx, _ *ev) { c.PlusActions(2) })
	cotr.Coins = 1
	reg("Coin of the Realm", cotr)
	reg("Royal Carriage", afterAction(func(c *ctx) { c.PlusActions(1) },
		func(c *ctx, e *ev) bool { return c.G.Where(e.X) == engine.Loc{Zone: inPlay, Owner: int16(c.P)} },
		func(c *ctx, e *ev) {
			c.PlayCard(e.X, 1)
			for _, d := range c.G.Durs() { // it stays as long as the Duration it replayed, also one only its first play keeps out (Gear)
				if d.X == e.X && !d.Done && !slices.Contains(d.Hold, c.Me) {
					d.Hold = append(d.Hold, c.Me)
				}
			}
		}))
	reg("Ratcatcher", startCall(func(c *ctx) { c.Draw(1); c.PlusActions(1) }, func(c *ctx) {
		c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
	}))
	reg("Guide", startCall(func(c *ctx) { c.Draw(1); c.PlusActions(1) }, func(c *ctx) {
		c.Discard(c.P, append([]inst{}, c.Hand(c.P)...))
		c.Draw(5)
	}))
	reg("Transmogrify", startCall(func(c *ctx) { c.PlusActions(1) }, func(c *ctx) {
		if x := c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false))); len(x) == 1 {
			c.ChooseGain(c.P, costUp(c, x[0], 1), hand)
		}
	}))
	reg("Teacher", startCall(func(*ctx) {}, func(c *ctx) {
		k := c.Mode1(c.P, mode("+1 Card", "PLAYER_MOVES_TOKEN_TO"), mode("+1 Action", "PLAYER_MOVES_TOKEN_TO"),
			mode("+1 Buy", "PLAYER_MOVES_TOKEN_TO"), mode("+$1", "PLAYER_MOVES_TOKEN_TO"))
		tokenTo(c, engine.TokCard+k, true)
	}))
	reg("Duplicate", rules{
		Play:  reserve,
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "gains a Gold.", "calls a Duplicate.", "gains a Gold."
			if i := c.G.PileOf(e.C); e.P != c.P || i < 0 || c.G.Top(i) != e.C || !c.CostsUpTo(6)(c.G.D(e.C)) {
				return
			}
			if c = c.Here(); call(c) {
				c.Gain(c.P, e.C, discard)
			}
		},
	})
	reg("Wine Merchant", rules{
		Play:  func(c *ctx) { c.PlusBuys(1); c.PlusCoins(4); reserve(c) },
		Hears: engine.On(engine.EvBuyEnd),
		Hear: func(c *ctx, e *ev) { // "ends their buy phase.", "discards 2 Wine Merchants."
			if e.P != c.P || c.Pl(c.P).Coins < 2 || c.T2().Used[c.Src] > 0 || c.G.Where(c.Me).Zone != tavern {
				return
			}
			c.T2().Used[c.Src]++
			mine := pick(c, c.P, engine.ToDiscard, tavern, named("Wine Merchant"), 0, len(c.Zone(c.P, tavern)))
			c.Here().Discard(c.P, mine)
		},
	})
	reg("Distant Lands", rules{Play: reserve, VPAt: func(g *engine.Game, x inst) int {
		if g.Where(x).Zone == tavern {
			return 4
		}
		return 0
	}})
	reg("Miser", rules{Play: func(c *ctx) {
		if c.Mode1(c.P, mode("put a Copper from your hand onto your Tavern mat", "PUT_ON_MAT"), mode("+$1 per Copper on your Tavern mat", "GETS_COINS")) == 0 {
			c.PutOnMat(c.P, one(pick1(c, c.P, engine.ToPick, hand, named("Copper"), false)), tavern)
		} else if n := c.Count(c.P, tavern, named("Copper")); n > 0 {
			c.PlusCoins(n)
		}
	}})

	// Durations
	amulet := func(c *ctx, away bool) {
		coin, trash, gain := "GETS_COIN", "TRASH", "GAIN"
		if away { // next turn: "trashes an Estate. (Amulet)"
			coin, trash, gain = "GETS_COIN_FROM", "TRASH_WITH", "GAIN_WITH"
		}
		switch c.Mode1(c.P, mode("+$1", coin), mode("trash a card from your hand", trash), mode("gain a Silver", gain)) {
		case 0:
			c.PlusCoins(1)
		case 1:
			c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
		case 2:
			c.Gain(c.P, silver(c), discard)
		}
	}
	reg("Amulet", rules{
		Play: func(c *ctx) { amulet(c, false); c.Stay() },
		Next: func(c *ctx, _ *engine.Dur) bool { amulet(c, true); return true },
	})
	dungeon := func(c *ctx) {
		c.Draw(2)
		n := min(2, len(c.Hand(c.P)))
		c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, n, n))
	}
	reg("Dungeon", rules{
		Play: func(c *ctx) { c.PlusActions(1); dungeon(c); c.Stay() },
		Next: func(c *ctx, _ *engine.Dur) bool { dungeon(c); return true },
	})
	reg("Gear", rules{
		Play: func(c *ctx) { // "sets 2 cards aside with Gear."; it doesn't stay without them
			c.Draw(2)
			if xs := c.SetAsideWith(c.P, pick(c, c.P, engine.ToSetAside, hand, nil, 0, 2)...); len(xs) > 0 {
				c.Stay().Aside = xs
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool { // "puts 2 cards in hand (Gear)."
			c.ToHand(c.P, c.In(aside, d.Aside), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			return true
		},
	})
	reg("Caravan Guard", rules{
		Play:     func(c *ctx) { c.Draw(1); c.PlusActions(1); c.Stay() },
		Next:     func(c *ctx, _ *engine.Dur) bool { c.PlusCoins(1); return true },
		Reacts:   engine.On(engine.EvAttack),
		CanReact: func(c *ctx, e *ev) bool { return e.Victim == c.P },
		React: func(c *ctx, e *ev) bool { // "reacts with a Caravan Guard.", "plays a Caravan Guard."
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			c.PlayNow(c.Me)
			return true
		},
	})
	// Haunted Woods and Swamp Hag write nothing when played; d.N is 1 while
	// their Attack lasts (the victim wasn't protected)
	until := func(effect func(c *ctx, e *ev), next func(c *ctx)) rules {
		return rules{
			Play: func(c *ctx) {
				d := c.Stay()
				c.Attack(func(int) { d.N = 1 })
			},
			Next:  func(c *ctx, d *engine.Dur) bool { d.N = 0; next(c); return true },
			Hears: engine.On(engine.EvBought),
			Hear: func(c *ctx, e *ev) { // when another player buys a card: after what its gain caused (Haunted Castle's Gold), before Colonnade
				if e.P == c.P {
					return
				}
				for _, d := range c.Durs() {
					if d.N == 1 { // once per play: a Throne Roomed Swamp Hag gives 2 Curses
						effect(c.Here(), e)
					}
				}
			},
		}
	}
	reg("Haunted Woods", until(func(c *ctx, e *ev) { topdeckAny(c, e.P, append([]inst{}, c.Hand(e.P)...)) }, func(c *ctx) { c.Draw(3) }))
	reg("Swamp Hag", until(func(c *ctx, e *ev) { c.Gain(e.P, c.Card("Curse"), discard) }, func(c *ctx) { c.PlusCoins(3) }))
	reg("Hireling", rules{
		Play: func(c *ctx) { c.Stay().Ever = true },
		Next: func(c *ctx, _ *engine.Dur) bool { c.Draw(1); return false },
	})
	reg("Bridge Troll", rules{
		Play: func(c *ctx) {
			c.Attack(func(v int) { c.Pl(v).MinusCoin = c.Src }) // silent
			c.PlusBuys(1)
			c.T2().Reduce++ // this turn and the next
			c.Stay()
		},
		Next: func(c *ctx, _ *engine.Dur) bool { c.PlusBuys(1); c.T2().Reduce++; return true },
	})

	// Actions and Treasures
	reg("Artificer", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusCoins(1)
		want := engine.Upto(len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 0, len(c.Hand(c.P))))))
		if o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(func(d *def) bool { return c.CostOf(d.ID) == want }), true); o != nil {
			c.Gain(c.P, o.Card, deck)
		}
	}})
	reg("Giant", rules{Play: func(c *ctx) {
		if !c.FlipJourney() {
			c.PlusCoins(1)
			return
		}
		c.PlusCoins(5)
		c.Attack(func(v int) {
			top := c.FromTop(v, 1, "REVEAL")
			if len(top) == 0 {
				return
			}
			if costs3to6(c)(c.G.Def(top[0])) {
				c.Trash(v, top)
			} else {
				c.Discard(v, top)
				c.Gain(v, c.Card("Curse"), discard)
			}
		})
	}})
	reg("Lost City", rules{
		Play:   func(c *ctx) { c.Draw(2); c.PlusActions(2) },
		OnGain: func(c *ctx, _ *ev) { c.DrawFor(c.Other(), 1) },
	})
	reg("Magpie", rules{Play: func(c *ctx) { // "reveals a Jack of All Trades.", "puts a Jack of All Trades back onto their deck.", "gains a Magpie."
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 1, "REVEAL")
		if len(top) == 0 {
			return
		}
		d := c.G.Def(top[0])
		if d.Treasure() {
			c.ToHand(c.P, top, "PUT_IN_HAND")
		} else {
			c.MoveTo(c.P, top, deck, "BACK_ON_DECK")
		}
		if d.Action() || d.Victory() {
			c.Gain(c.P, c.Src, discard)
		}
	}})
	reg("Messenger", rules{
		Play: func(c *ctx) {
			c.PlusBuys(1)
			c.PlusCoins(2)
			if len(c.Zone(c.P, deck)) > 0 && c.Yes(c.P, "put your deck into your discard pile", "DECK_TO_DISCARD") {
				c.MoveTo(c.P, append([]inst{}, c.Zone(c.P, deck)...), discard, "")
				c.Log("DECK_TO_DISCARD", engine.Seat(c.P))
			}
		},
		OnGain: func(c *ctx, _ *ev) { // the first gain of the Buy phase: "gains a Silver.", the other player "gains a Silver."
			if c.P != c.G.Cur || c.T2().Phase != engine.BuyPhase || len(c.T2().BuyGained) != 1 {
				return
			}
			if x := c.ChooseGain(c.P, c.CostsUpTo(4), discard); x >= 0 {
				c.Gain(c.Other(), c.G.CardOf(x), discard)
			}
		},
	})
	reg("Port", rules{
		Play: func(c *ctx) { c.Draw(1); c.PlusActions(2) },
		OnGain: func(c *ctx, e *ev) {
			if e.By != c.Src { // the Port it gains doesn't come with another
				c.Gain(c.P, c.Src, discard)
			}
		},
	})
	reg("Ranger", rules{Play: func(c *ctx) {
		c.PlusBuys(1)
		if c.FlipJourney() {
			c.Draw(5)
		}
	}})
	reg("Raze", rules{Play: func(c *ctx) { // "trashes a Silver.", "looks at 3 cards.", "puts a Gold into their hand.", "discards 2 cards."
		c.PlusActions(1)
		opts := c.Options(c.P, hand, nil)
		if c.G.Where(c.Me).Zone == inPlay {
			opts = append(c.OptionsOf(one(c.Me)), opts...)
		}
		o := c.ChooseOne(c.P, engine.ToTrash, opts, false)
		if o == nil {
			return
		}
		n := c.CostOf(o.Card).Coins
		c.Trash(c.P, one(o.Inst))
		top := c.FromTop(c.P, n, "LOOK_AT")
		if len(top) == 0 {
			return
		}
		c.ToHand(c.P, pickOf(c, c.P, engine.ToPick, top, 1, 1), "PUT_IN_HAND")
		c.Discard(c.P, c.In(revealed, top))
	}})
	reg("Relic", rules{Coins: 2, Play: func(c *ctx) { // its own line: "plays a Relic. (+$2)", the other player "moves -Card token to Draw Pile."
		c.Attack(func(v int) { c.MinusCardOn(v) })
	}})
	reg("Storyteller", rules{Play: func(c *ctx) { // "plays 2 Coppers. (+$2)", "draws a card.", "loses 2 Coins.", "draws 2 cards."
		c.PlusActions(1)
		c.PlayMerged(pick(c, c.P, engine.ToPlay, hand, treasure, 0, 3))
		c.Draw(1)
		n := c.Pl(c.P).Coins
		c.LoseCoins(n)
		c.Draw(n)
	}})
	reg("Treasure Trove", rules{Coins: 2, Play: func(c *ctx) {
		c.Gain(c.P, c.Card("Gold"), discard)
		c.Gain(c.P, c.Card("Copper"), discard)
	}})

	// Travellers: Page to Treasure Hunter, Warrior, Hero, Champion; Peasant to Soldier, Fugitive, Disciple, Teacher
	page := traveller("Treasure Hunter", rules{Play: func(c *ctx) { c.Draw(1); c.PlusActions(1) }})
	page.Setup = travellers("Treasure Hunter", "Warrior", "Hero", "Champion")
	reg("Page", page)
	reg("Treasure Hunter", traveller("Warrior", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		c.PlusCoins(1)
		c.GainN(c.P, silver(c), len(c.G.PrevGained(c.Other())), discard, "GAIN")
	}}))
	reg("Warrior", traveller("Hero", rules{Play: func(c *ctx) { // the other player "discards an Envoy.", "trashes an Envoy."
		c.Draw(2)
		n := c.Count(c.P, inPlay, func(d *def) bool { return d.Is(engine.TTraveller) })
		c.Attack(func(v int) {
			for range n {
				if x := c.DiscardTop(v); x >= 0 && costs3or4(c)(c.G.Def(x)) {
					c.Trash(v, one(x))
				}
			}
		})
	}}))
	reg("Hero", traveller("Champion", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		c.ChooseGain(c.P, treasure, discard)
	}}))
	reg("Champion", rules{
		Play:  func(c *ctx) { c.PlusActions(1); c.Stay().Ever = true },
		Hears: engine.On(engine.EvPlaying, engine.EvAttack),
		Hear: func(c *ctx, e *ev) {
			if e.Kind == engine.EvAttack && e.Victim == c.P {
				e.Unaffected = true // silent
			} else if e.Kind == engine.EvPlaying && e.P == c.P && e.X != c.Me && c.G.D(e.C).Action() {
				c.Here().PlusActions(1) // "plays a Village.", "gets +1 Action."
			}
		},
	})
	peasant := traveller("Soldier", rules{Play: func(c *ctx) { c.PlusBuys(1); c.PlusCoins(1) }})
	peasant.Setup = travellers("Soldier", "Fugitive", "Disciple", "Teacher")
	reg("Peasant", peasant)
	reg("Soldier", traveller("Fugitive", rules{Play: func(c *ctx) {
		n := 0
		for _, x := range c.Zone(c.P, inPlay) {
			if x != c.Me && c.G.Def(x).Attack() {
				n++
			}
		}
		c.PlusCoins(2 + n)
		c.Attack(func(v int) {
			if len(c.Hand(v)) >= 4 {
				c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, 1, 1))
			}
		})
	}}))
	reg("Fugitive", traveller("Disciple", rules{Play: func(c *ctx) {
		c.Draw(2)
		c.PlusActions(1)
		c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 1, 1))
	}}))
	reg("Disciple", traveller("Teacher", rules{Play: func(c *ctx) { // "plays a Soldier.", "plays a Soldier again.", "gains a Soldier."
		x := pick1(c, c.P, engine.ToPlay, hand, action, true)
		if x < 0 {
			return
		}
		c.PlayCard(x, 0)
		c.PlayCard(x, 1)
		c.Gain(c.P, c.G.CardOf(x), discard)
	}}))

	// Events
	reg("Alms", rules{
		CanBuy: func(g *engine.Game, p int) bool { return once("Alms")(g, p) && !inPlayOf(g, p, treasure) },
		Buy:    func(c *ctx) { c.T2().Used[c.Src]++; c.ChooseGain(c.P, c.CostsUpTo(4), discard) },
	})
	reg("Ball", rules{Buy: func(c *ctx) { // the -$1 token is taken silently
		c.Pl(c.P).MinusCoin = c.Src
		c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		c.ChooseGain(c.P, c.CostsUpTo(4), discard)
	}})
	reg("Bonfire", rules{Buy: func(c *ctx) { c.Trash(c.P, pick(c, c.P, engine.ToTrash, inPlay, named("Copper"), 0, 2)) }})
	reg("Borrow", rules{Buy: func(c *ctx) { // "gets +1 Buy.", "moves -Card token to Draw Pile.", "gets +$1."
		c.PlusBuys(1)
		if c.T2().Used[c.Src] == 0 && c.MinusCardOn(c.P) {
			c.T2().Used[c.Src]++
			c.PlusCoins(1)
		}
	}})
	reg("Expedition", rules{Buy: func(c *ctx) { c.T2().HandSize += 2 }})
	reg("Ferry", rules{Buy: func(c *ctx) { tokenTo(c, engine.TokCost, false) }})
	reg("Lost Arts", rules{Buy: func(c *ctx) { tokenTo(c, engine.TokAction, false) }})
	reg("Pathfinding", rules{Buy: func(c *ctx) { tokenTo(c, engine.TokCard, false) }})
	reg("Plan", rules{Buy: func(c *ctx) { tokenTo(c, engine.TokTrash, false) }})
	reg("Training", rules{Buy: func(c *ctx) { tokenTo(c, engine.TokCoin, false) }})
	reg("Seaway", rules{Buy: func(c *ctx) {
		if x := c.ChooseGain(c.P, and(action, c.CostsUpTo(4)), discard); x >= 0 {
			c.MoveToken(engine.TokBuy, c.G.PileOf(c.G.CardOf(x)))
		}
	}})
	reg("Inheritance", rules{ // "inherits an Alley."
		CanBuy: func(g *engine.Game, p int) bool { return g.P[p].Inherited < 0 },
		Buy: func(c *ctx) {
			ok := and(action, not(func(d *def) bool { return d.Is(engine.TCommand | engine.TReserve) }), c.CostsUpTo(4))
			o := c.ChooseOne(c.P, engine.ToPick, c.SupplyOptions(ok), false)
			if o == nil {
				return
			}
			s := c.G.Piles[c.G.PileOf(o.Card)].Cards
			x := s[len(s)-1]
			c.G.Move(x, engine.Loc{Zone: aside, Owner: int16(c.P)})
			c.Pl(c.P).Inherited = x
			c.Log("INHERITS", engine.Seat(c.P), []engine.CardID{o.Card})
		},
	})
	reg("Mission", rules{ // "takes an extra turn after this one.", then "Turn 5 - L [Mission]" without buying cards
		CanBuy: func(g *engine.Game, p int) bool { return once("Mission")(g, p) && g.T2.Extra == 0 },
		Buy: func(c *ctx) {
			c.T2().Used[c.Src]++
			c.Log("MISSION_SUCCESS", engine.Seat(c.P))
			c.ExtraTurn()
		},
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, _ *ev) {
			if c.T2().Extra == c.Src {
				c.T2().NoBuy = true
			}
		},
	})
	reg("Pilgrimage", rules{
		CanBuy: once("Pilgrimage"),
		Buy: func(c *ctx) {
			c.T2().Used[c.Src]++
			if !c.FlipJourney() {
				return
			}
			var opts []engine.Option
			seen := map[engine.CardID]bool{}
			for _, x := range c.Zone(c.P, inPlay) {
				if id := c.G.CardOf(x); !seen[id] {
					seen[id] = true
					opts = append(opts, engine.Option{Card: id, Inst: -1})
				}
			}
			for _, o := range c.Choose(c.P, engine.ToGain, opts, 0, 3) {
				c.Gain(c.P, o.Card, discard)
			}
		},
	})
	reg("Quest", rules{Buy: func(c *ctx) { // "discards a Militia.", "gains a Gold."
		ms := []engine.Mode{mode("no", "")}
		do := []func() int{nil}
		if c.Count(c.P, hand, func(d *def) bool { return d.Attack() }) > 0 {
			ms = append(ms, mode("discard an Attack", "DISCARD"))
			do = append(do, func() int {
				return len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, func(d *def) bool { return d.Attack() }, 1, 1)))
			})
		}
		if c.Count(c.P, hand, named("Curse")) >= 2 {
			ms = append(ms, mode("discard two Curses", "DISCARD"))
			do = append(do, func() int { return len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, named("Curse"), 2, 2))) / 2 })
		}
		if len(c.Hand(c.P)) >= 6 {
			ms = append(ms, mode("discard six cards", "DISCARD"))
			do = append(do, func() int { return len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 6, 6))) / 6 })
		}
		if k := c.Mode1(c.P, ms...); k > 0 && do[k]() > 0 {
			c.Gain(c.P, c.Card("Gold"), discard)
		}
	}})
	reg("Raid", rules{Buy: func(c *ctx) {
		c.GainN(c.P, silver(c), c.Count(c.P, inPlay, named("Silver")), discard, "GAIN")
		c.MinusCardOn(c.Other())
	}})
	reg("Save", rules{
		Buy: func(c *ctx) { // "sets a Gold aside with Save.", and after the Clean-up draw "puts a Gold into their hand."
			c.PlusBuys(1)
			if c.T2().Used[c.Src] > 0 {
				return
			}
			c.T2().Used[c.Src]++
			if x := pick1(c, c.P, engine.ToSetAside, hand, nil, false); x >= 0 {
				c.SetAsideWith(c.P, x)
				c.AddWatch(engine.On(engine.EvTurnEnd), int(x)).Long = true
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P {
				return false
			}
			c.At(0).ToHand(w.P, c.In(aside, one(inst(w.N))), "PUT_IN_HAND")
			return true
		},
	})
	reg("Scouting Party", rules{Buy: func(c *ctx) {
		c.PlusBuys(1)
		top := c.FromTop(c.P, 5, "LOOK_AT")
		n := min(3, len(top))
		c.Discard(c.P, pickOf(c, c.P, engine.ToDiscard, top, n, n))
		topdeckAny(c, c.P, c.In(revealed, top))
	}})
	reg("Trade", rules{Buy: func(c *ctx) {
		n := len(c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 2)))
		c.GainN(c.P, silver(c), n, discard, "GAIN")
	}})
	reg("Travelling Fair", rules{
		Buy: func(c *ctx) {
			c.PlusBuys(2)
			if c.T2().Used[c.Src] == 0 {
				c.T2().Used[c.Src]++
				c.AddWatch(engine.On(engine.EvGain), 0)
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "gains a Silver.", "topdecks a Silver with Travelling Fair."
			if e.P == w.P && stillGained(c, e) && c.Yes(w.P, "put it on your deck", "TOPDECK_WITH") {
				c.MoveTo(w.P, one(e.X), deck, "TOPDECK_WITH", engine.Name(c.Src))
			}
			return false
		},
	})
}
