package cards

import (
	"slices"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Allies: the kingdom cards, the split piles and the Allies. Simplifications:
// Elder's extra option works only for the choices of this set's cards; Hunter
// takes the Action, then the Treasure, then the Victory card, so a card of two
// of those types fills the first; Skirmisher's discarding ignores a Moat;
// Voyage and Island Folk don't see an Outpost turn already waiting.

// ally is an Ally: each player starts with a Favor, "gets +1 Favor. (Desert Guides)".
func ally(r rules) rules {
	r.Start = func(g *engine.Game, me engine.CardID) {
		for p := range g.P {
			g.CtxFor(p, me, 0).PlusFavors(1)
		}
	}
	return r
}

// phaseLine writes "starts their turn." or "begins their buy phase.": with an
// Ally acting then in the game, dominion.games logs it every turn, even when the Ally does nothing.
func phaseLine(c *ctx) { c.Emit() }

func favors(c *ctx, p int) int { return c.G.P[p].Favors }

// spend asks whether to spend n Favors, and spends them: "spends 2 Favors."
func spend(c *ctx, n int, label string) bool {
	if favors(c, c.P) < n || !c.Yes(c.P, label, plural(n, "SPENDS_FAVOR", "SPENDS_FAVORS")) {
		return false
	}
	c.SpendFavors(n)
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// choose asks for n of a card's modes, or one more when Elder played it; they
// happen in the card's order.
func choose(c *ctx, n int, ms ...engine.Mode) []int {
	max := n
	if slices.Contains(c.T2().Elder, c.Me) && n < len(ms) {
		max++
	}
	got := c.Modes(c.P, n, max, ms...)
	slices.Sort(got)
	return got
}

// rotates: rotating the pile changes its top card.
func rotates(g *engine.Game, i int) bool {
	s := g.Piles[i].Cards
	return len(s) > 0 && g.CardOf(s[0]) != g.CardOf(s[len(s)-1])
}

// mayRotate lets the player rotate a split pile: "rotates 2 Town Criers."
func mayRotate(c *ctx, pile string) {
	if i := c.G.PileOf(c.Card(pile)); i >= 0 && rotates(c.G, i) && c.Yes(c.P, "rotate the "+pile, "ROTATES") {
		c.Rotate(i)
	}
}

// stack adds one to this card's watch for the turn, or makes it, so two Gallerias give "+2 Buys. (Galleria)" for a gain.
func stack(c *ctx) {
	ws := c.G.Watches()
	for i := range ws {
		if ws[i].C == c.Src && ws[i].P == c.P && !ws[i].Long {
			ws[i].N++
			return
		}
	}
	c.AddWatch(engine.On(engine.EvGain), 1)
}

// gainTop gains the top card of a split pile.
func gainTop(c *ctx, pile string) { c.GainN(c.P, c.Card(pile), 1, discard, "GAIN") }

// topdeckFromPlay is a card that may go onto the deck when discarded from play (Merchant Camp, Tent).
func topdeckFromPlay(c *ctx, e *ev) {
	if e.From == inPlay && c.G.Where(c.Me) == (engine.Loc{Zone: discard, Owner: int16(c.P)}) && c.Yes(c.P, "put it onto your deck", "TOPDECK") {
		c.Topdeck(c.P, one(c.Me))
	}
}

// drawTo draws until the hand has n cards: what the deck has, then the rest after the shuffle, on lines of their own.
func drawTo(c *ctx, n int) {
	need := n - len(c.Hand(c.P))
	if k := len(c.Zone(c.P, deck)); k > 0 && k < need {
		c.Draw(k)
		need -= k
	}
	if need > 0 {
		c.Draw(need)
	}
}

// againLine is the line playing a card again writes: "plays a Gold again. (+$3)".
func againLine(d *def) string {
	if playLine(d) == "PLAY_TREASURES_FOR" {
		return "PLAY_TREASURES_AGAIN_FOR"
	}
	return "PLAY_AGAIN"
}

func liaison(c *ctx, e *ev) bool { return c.G.D(e.C).Is(engine.TLiaison) }

func init() {
	curse := func(c *ctx) engine.CardID { return c.Card("Curse") }

	// Kingdom cards
	reg("Town", rules{Play: func(c *ctx) {
		for _, m := range choose(c, 1, mode("+1 Card and +2 Actions", "DRAW"), mode("+1 Buy and +$2", "GETS_BUY")) {
			if m == 0 {
				c.Draw(1)
				c.PlusActions(2)
			} else {
				c.PlusBuys(1)
				c.PlusCoins(2)
			}
		}
	}})
	reg("Modify", rules{Play: func(c *ctx) {
		t := c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
		for _, m := range choose(c, 1, mode("+1 Card and +1 Action", "DRAW"), mode("gain a card costing up to $2 more", "GAIN")) {
			switch {
			case m == 0:
				c.Draw(1)
				c.PlusActions(1)
			case len(t) == 1:
				c.ChooseGain(c.P, costUp(c, t[0], 2), discard)
			}
		}
	}})
	reg("Merchant Camp", rules{
		Play:      func(c *ctx) { c.PlusActions(2); c.PlusCoins(1) },
		OnDiscard: topdeckFromPlay,
	})
	reg("Swap", rules{Play: func(c *ctx) { // "returns a Flag Bearer to the Flag Bearer pile.", "gains a Swap."
		c.Draw(1)
		c.PlusActions(1)
		if x := pick1(c, c.P, engine.ToReturn, hand, action, true); x >= 0 && c.ReturnTo(c.P, x) {
			card := c.G.CardOf(x)
			c.ChooseGain(c.P, and(action, c.CostsUpTo(5), func(d *def) bool { return d.ID != card }), hand)
		}
	}})
	reg("Skirmisher", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.PlusCoins(1)
			c.AddWatch(engine.On(engine.EvGain), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "gains a Skirmisher.", the other player "discards 2 Golds and a Kitsune."
			if e.P == w.P && c.G.D(e.C).Attack() {
				downTo(c, 1-w.P, 3)
			}
			return false
		},
	})
	reg("Highwayman", rules{
		Play: func(c *ctx) {
			d := c.Stay()
			c.Attack(func(int) { d.N = 1 })
		},
		Next: func(c *ctx, _ *engine.Dur) bool { // "discards a Highwayman. (Highwayman)", "draws 3 cards. (Highwayman)"
			c.Discard(c.P, inPlayOnly(c, c.Me))
			c.Draw(3)
			return true
		},
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) {
			if e.P != c.Other() {
				return
			}
			for _, d := range c.Durs() {
				if d.N == 1 {
					c.T2().Nullify = true
				}
			}
		},
	})
	reg("Barbarian", rules{Play: func(c *ctx) { // the other player "trashes an Estate.", "gains a Curse."
		c.PlusCoins(2)
		c.Attack(func(v int) {
			x := c.TopCard(v)
			if x < 0 {
				return
			}
			c.Trash(v, one(x))
			d := c.G.Def(x)
			if c.CostOf(d.ID).Coins < 3 {
				c.Gain(v, curse(c), discard)
				return
			}
			c.ChooseGain(v, and(cheaperThan(c, d.ID), func(e *def) bool { return e.T&d.T&(engine.TEvent-1) != 0 }), discard)
		})
	}})
	reg("Innkeeper", rules{Play: func(c *ctx) { // "draws 3 cards.", "discards 2 other cards and a Copper."
		c.PlusActions(1)
		for _, m := range choose(c, 1, engine.Mode{Label: "+1 Card", Line: "DRAW", Count: 1},
			engine.Mode{Label: "+3 Cards, discard 3", Line: "DRAW", Count: 3}, engine.Mode{Label: "+5 Cards, discard 6", Line: "DRAW", Count: 5}) {
			c.Draw([]int{1, 3, 5}[m])
			if n := min([]int{0, 3, 6}[m], len(c.Hand(c.P))); n > 0 {
				c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, n, n))
			}
		}
	}})
	reg("Capital City", rules{Play: func(c *ctx) { // "discards a card and a Copper.", "gets +$2."; "loses 2 Coins.", "draws 2 cards."
		c.Draw(1)
		c.PlusActions(2)
		if len(c.Hand(c.P)) >= 2 && c.Yes(c.P, "discard 2 cards for +$2", "DISCARD") {
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 2, 2))
			c.PlusCoins(2)
		}
		if c.Pl(c.P).Coins >= 2 && c.Yes(c.P, "pay $2 for +2 Cards", "LOSES_COINS") {
			c.LoseCoins(2)
			c.Draw(2)
		}
	}})
	reg("Specialist", rules{Play: func(c *ctx) { // "plays a Captain.", then "plays a Captain again." or "gains a Captain."
		x := pick1(c, c.P, engine.ToPlay, hand, func(d *def) bool { return d.Action() || d.Treasure() }, true)
		if x < 0 {
			return
		}
		c.PlayNow(x) // it stays in play with a Duration only when it plays it again
		for _, m := range choose(c, 1, mode("play it again", againLine(c.G.Def(x))), mode("gain a copy of it", "GAIN")) {
			if m == 0 {
				c.PlayCard(x, 1)
			} else {
				c.Gain(c.P, c.G.CardOf(x), discard)
			}
		}
	}})
	reg("Carpenter", rules{Play: func(c *ctx) {
		if c.G.EmptyPiles() == 0 {
			c.PlusActions(1)
			c.ChooseGain(c.P, c.CostsUpTo(4), discard)
			return
		}
		if t := c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false))); len(t) == 1 {
			c.ChooseGain(c.P, costUp(c, t[0], 2), discard)
		}
	}})
	reg("Royal Galley", rules{
		Play: func(c *ctx) { // "draws a card.", "plays a Coven.", which it sets aside unlogged; next turn "plays a Coven. (Royal Galley)"
			c.Draw(1)
			x := pick1(c, c.P, engine.ToPlay, hand, and(action, not(duration)), true)
			if x < 0 {
				return
			}
			c.PlayNow(x)
			if xs := c.MoveTo(c.P, c.In(inPlay, one(x)), aside, ""); len(xs) > 0 {
				c.Stay().Aside = xs
			}
		},
		Next: playAside,
	})
	reg("Marquis", rules{Play: func(c *ctx) {
		c.PlusBuys(1)
		c.Draw(len(c.Hand(c.P)))
		downTo(c, c.P, 10)
	}})
	reg("Sentinel", rules{Play: func(c *ctx) { // "looks at ...", "trashes 2 Coppers.", "topdecks a Necropolis, a Gladiator, and a Lackeys."
		xs := c.FromTop(c.P, 5, "LOOK_AT")
		c.Trash(c.P, pickOf(c, c.P, engine.ToTrash, xs, 0, 2))
		topdeckAny(c, c.P, c.In(revealed, xs))
	}})
	reg("Courier", rules{Play: func(c *ctx) { // "discards an Estate.", "looks at 6 other cards and a Silver.", "plays an Upgrade."
		c.PlusCoins(1)
		c.DiscardTop(c.P)
		c.LookAt(c.P, c.Zone(c.P, discard))
		if x := pick1(c, c.P, engine.ToPlay, discard, func(d *def) bool { return d.Action() || d.Treasure() }, true); x >= 0 {
			c.PlayNow(x)
		}
	}})
	reg("Galleria", rules{
		Play: func(c *ctx) { c.PlusCoins(3); stack(c) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P == w.P && costs3or4(c)(c.G.D(e.C)) {
				c.Away().PlusBuys(w.N)
			}
			return false
		},
	})
	reg("Hunter", rules{Play: func(c *ctx) { // "reveals 3 cards", "puts a Plunder and a Hunter into their hand.", "discards a Sacrifice."
		c.PlusActions(1)
		xs := c.FromTop(c.P, 3, "REVEAL")
		var got []inst
		for _, ok := range []func(*def) bool{action, treasure, victory} {
			if ys := only(c, others(xs, got), ok); len(ys) > 0 {
				got = append(got, pickOf(c, c.P, engine.ToPick, ys, 1, 1)...)
			}
		}
		c.ToHand(c.P, got, "PUT_IN_HAND")
		c.Discard(c.P, c.In(revealed, xs))
	}})
	reg("Sycophant", rules{
		Play: func(c *ctx) {
			c.PlusActions(1)
			n := min(3, len(c.Hand(c.P)))
			if len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, n, n))) > 0 {
				c.PlusCoins(3)
			}
		},
		OnGain:  func(c *ctx, _ *ev) { c.PlusFavors(2) },
		OnTrash: func(c *ctx, _ *ev) { c.PlusFavors(2) },
	})
	reg("Underling", rules{Play: func(c *ctx) { c.Draw(1); c.PlusActions(1); c.PlusFavors(1) }})
	reg("Importer", rules{
		Play: func(c *ctx) { c.Stay() },
		Next: func(c *ctx, _ *engine.Dur) bool { c.ChooseGain(c.P, c.CostsUpTo(5), discard); return true }, // "gains a Lost City. (Importer)"
		Start: func(g *engine.Game, me engine.CardID) {
			for p := range g.P {
				g.CtxFor(p, me, 0).PlusFavors(4)
			}
		},
	})
	reg("Contract", rules{Alone: true,
		Play: func(c *ctx) { // "gets +$2.", "gets +1 Favor. (Contract)", "sets a Patrol aside with Contract."; next turn "plays a Patrol. (Contract)"
			c.PlusCoins(2)
			c.PlusFavors(1)
			if xs := c.SetAsideWith(c.P, pick(c, c.P, engine.ToSetAside, hand, action, 0, 1)...); len(xs) > 0 {
				c.Stay().Aside = xs
			}
		},
		Next: playAside,
	})
	reg("Emissary", rules{Play: func(c *ctx) {
		n := c.Pl(c.P).Shuffles
		c.Draw(3)
		if c.Pl(c.P).Shuffles > n {
			c.PlusActions(1)
			c.PlusFavors(2)
		}
	}})
	reg("Bauble", rules{Alone: true,
		Play: func(c *ctx) { // "gets +1 Buy.", "gets +1 Favor. (Bauble)"; a gain "topdecks a Specialist with Bauble."
			for _, m := range choose(c, 2, mode("+1 Buy", "GETS_BUY"), mode("+$1", "GETS_COIN"), mode("+1 Favor", "GETS_FAVOR_FROM"), mode("put gains onto your deck", "")) {
				switch m {
				case 0:
					c.PlusBuys(1)
				case 1:
					c.PlusCoins(1)
				case 2:
					c.PlusFavors(1)
				case 3:
					c.AddWatch(engine.On(engine.EvGain), 0)
				}
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P == w.P && stillGained(c, e) && c.Yes(w.P, "put it onto your deck", "TOPDECK_WITH") {
				e.Done = true
				c.MoveTo(w.P, one(e.X), deck, "TOPDECK_WITH", engine.Name(c.Src))
			}
			return false
		},
	})
	reg("Broker", rules{Play: func(c *ctx) { // "trashes an Estate.", "gets +2 Favors. (Broker)"
		t := c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
		if len(t) == 0 {
			return
		}
		n := c.CostOf(c.G.CardOf(t[0])).Coins
		if n == 0 {
			return
		}
		for _, m := range choose(c, 1, engine.Mode{Label: "+1 Card per $1", Line: "DRAW", Count: n},
			mode("+1 Action per $1", plural(n, "GETS_ACTION", "GETS_ACTIONS")), mode("+$1 per $1", plural(n, "GETS_COIN", "GETS_COINS")),
			mode("+1 Favor per $1", plural(n, "GETS_FAVOR_FROM", "GETS_FAVORS_FROM"))) {
			switch m {
			case 0:
				c.Draw(n)
			case 1:
				c.PlusActions(n)
			case 2:
				c.PlusCoins(n)
			case 3:
				c.PlusFavors(n)
			}
		}
	}})
	reg("Guildmaster", rules{
		Play: func(c *ctx) { c.PlusCoins(3); stack(c) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "buys and gains a Falconer.", "gets +1 Favor. (Guildmaster)"
			if e.P == w.P {
				c.PlusFavors(w.N)
			}
			return false
		},
	})

	// Augurs
	reg("Herb Gatherer", rules{Play: func(c *ctx) { // "moves their deck to the discard.", "looks at 7 other cards and a Swashbuckler.", "plays a Silver. (+$2)"
		c.PlusBuys(1)
		if len(c.Zone(c.P, deck)) > 0 {
			c.MoveTo(c.P, append([]inst{}, c.Zone(c.P, deck)...), discard, "")
			c.Log("DECK_TO_DISCARD", engine.Seat(c.P))
		}
		c.LookAt(c.P, c.Zone(c.P, discard))
		if x := pick1(c, c.P, engine.ToPlay, discard, treasure, true); x >= 0 {
			c.PlayNow(x)
		}
		mayRotate(c, "Augurs")
	}})
	reg("Acolyte", rules{Play: func(c *ctx) {
		if len(c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, func(d *def) bool { return d.Action() || d.Victory() }, true)))) == 1 {
			c.Gain(c.P, c.Card("Gold"), discard)
		}
		if c.G.Where(c.Me).Zone == inPlay && !c.InPlace && c.Yes(c.P, "trash this to gain an Augur", "TRASH") && len(c.Trash(c.P, one(c.Me))) == 1 {
			gainTop(c, "Augurs")
		}
	}})
	reg("Sorceress", rules{Play: func(c *ctx) { // "guesses Gold.", "reveals a Gold.", "puts a Gold into their hand."
		c.PlusActions(1)
		name := c.NameCard(c.P)
		c.Log("SORCERESS_GUESS", engine.Seat(c.P), engine.Name(name))
		x := c.TopCard(c.P)
		if x < 0 {
			return
		}
		c.Reveal(c.P, one(x))
		c.ToHand(c.P, one(x), "PUT_IN_HAND")
		if c.G.CardOf(x) == name {
			c.Attack(func(v int) { c.Gain(v, curse(c), discard) })
		}
	}})
	reg("Sibyl", rules{Play: func(c *ctx) {
		c.Draw(4)
		c.PlusActions(1)
		c.Topdeck(c.P, one(pick1(c, c.P, engine.ToTopdeck, hand, nil, false)))
		c.Bottomdeck(c.P, one(pick1(c, c.P, engine.ToPick, hand, nil, false)))
	}})

	// Clashes
	reg("Battle Plan", rules{Play: func(c *ctx) { // "reveals a Warlord.", "draws a Copper.", "rotates 4 Blacksmiths."
		c.Draw(1)
		c.PlusActions(1)
		if x := pick1(c, c.P, engine.ToReveal, hand, attack, true); x >= 0 {
			c.Reveal(c.P, one(x))
			c.Draw(1)
		}
		var opts []engine.Option
		for i, p := range c.G.Piles {
			if p.Supply && rotates(c.G, i) {
				opts = append(opts, engine.Option{Card: c.G.Top(i), Inst: -1})
			}
		}
		if o := c.ChooseOne(c.P, engine.ToPick, opts, true); o != nil {
			c.Rotate(c.G.PileOf(o.Card))
		}
	}})
	reg("Archer", rules{Play: func(c *ctx) { // the other player "reveals 3 Coppers and a Riverboat.", "discards a Riverboat."
		c.PlusCoins(2)
		c.Attack(func(v int) {
			n := len(c.Hand(v))
			if n < 5 {
				return
			}
			xs := pick(c, v, engine.ToReveal, hand, nil, n-1, n-1)
			c.Reveal(v, xs)
			c.Discard(v, cards(c.Choose(c.P, engine.ToDiscard, c.OptionsOf(xs), 1, 1)))
		})
	}})
	reg("Warlord", rules{
		Play: func(c *ctx) {
			c.PlusActions(1)
			d := c.Stay()
			c.Attack(func(int) { d.N = 1 })
		},
		Next: func(c *ctx, _ *engine.Dur) bool { c.Draw(2); return true }, // "draws 2 cards. (Warlord)"
		Forbid: func(g *engine.Game, me inst, p int, x inst) bool {
			if !g.Def(x).Action() {
				return false
			}
			for _, d := range g.Durs() {
				if d.X == me && d.N == 1 && !d.Done {
					n := 0
					for _, y := range g.P[p].Z[inPlay] {
						if g.CardOf(y) == g.CardOf(x) {
							n++
						}
					}
					return n >= 2
				}
			}
			return false
		},
	})
	reg("Territory", rules{
		VP: func(g *engine.Game, p int) int { return len(names(g, p, victory)) },
		OnGain: func(c *ctx, _ *ev) {
			if n := c.G.EmptyPiles(); n > 0 {
				c.GainN(c.P, c.Card("Gold"), n, discard, "GAIN")
			}
		},
	})

	// Forts
	reg("Tent", rules{
		Play:      func(c *ctx) { c.PlusCoins(2); mayRotate(c, "Forts") },
		OnDiscard: topdeckFromPlay,
	})
	reg("Garrison", rules{
		Play: func(c *ctx) { c.PlusCoins(2); c.AddWatch(engine.On(engine.EvGain), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "adds a token to Garrison, now at 2."
			if e.P != w.P || c.G.Where(c.Me).Zone != inPlay {
				return false
			}
			ds := c.Durs()
			if len(ds) == 0 {
				ds = append(ds, c.Stay())
			}
			ds[0].N++
			c.Log("GARRISON_ADDS_TOKEN", engine.Seat(w.P), ds[0].N)
			return false
		},
		Next: func(c *ctx, d *engine.Dur) bool { c.Draw(d.N); return true },
	})
	reg("Hill Fort", rules{Play: func(c *ctx) { // "gains a Farmhands.", "draws a card.", "gets +1 Action."
		x := c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		for _, m := range choose(c, 1, mode("put it into your hand", "PUT_IN_HAND"), mode("+1 Card and +1 Action", "DRAW")) {
			if m == 0 {
				if x >= 0 {
					c.ToHand(c.P, c.In(discard, one(x)), "PUT_IN_HAND") // not one City-state played
				}
			} else {
				c.Draw(1)
				c.PlusActions(1)
			}
		}
	}})
	reg("Stronghold", rules{VP: worth(2),
		Play: func(c *ctx) {
			for _, m := range choose(c, 1, mode("+$3", "GETS_COINS"), mode("+3 Cards next turn", "")) {
				if m == 0 {
					c.PlusCoins(3)
				} else {
					c.Stay()
				}
			}
		},
		Next: func(c *ctx, _ *engine.Dur) bool { c.Draw(3); return true },
	})

	// Odysseys
	reg("Old Map", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.Discard(c.P, one(pick1(c, c.P, engine.ToDiscard, hand, nil, false)))
		c.Draw(1)
		mayRotate(c, "Odysseys")
	}})
	reg("Voyage", rules{
		Play: func(c *ctx) { // "takes an extra Voyage turn after this one."; "Turn 9 - L [Voyage]"
			c.PlusActions(1)
			if c.T2().Extra != 0 || countPlayed(c, c.Src) > 1 {
				return
			}
			c.Log("VOYAGE_SUCCESS", engine.Seat(c.P))
			c.ExtraTurn()
			c.Stay()
		},
		Next: func(c *ctx, _ *engine.Dur) bool {
			if c.T2().Extra == c.Src {
				c.T2().HandLimit = 3
			}
			return true
		},
	})
	reg("Sunken Treasure", rules{Alone: true, Play: func(c *ctx) { // "gains a Galleria."
		c.ChooseGain(c.P, func(d *def) bool { return d.Action() && c.Count(c.P, inPlay, named(d.Name)) == 0 }, discard)
	}})
	reg("Distant Shore", rules{VP: worth(2), Play: func(c *ctx) {
		c.Draw(2)
		c.PlusActions(1)
		c.Gain(c.P, c.Card("Estate"), discard)
	}})

	// Townsfolk
	reg("Town Crier", rules{Play: func(c *ctx) { // "gains a Silver.", "rotates 2 Town Criers."
		for _, m := range choose(c, 1, mode("+$2", "GETS_COINS"), mode("gain a Silver", "GAIN"), mode("+1 Card and +1 Action", "DRAW")) {
			switch m {
			case 0:
				c.PlusCoins(2)
			case 1:
				c.Gain(c.P, c.Card("Silver"), discard)
			case 2:
				c.Draw(1)
				c.PlusActions(1)
			}
		}
		mayRotate(c, "Townsfolk")
	}})
	reg("Blacksmith", rules{Play: func(c *ctx) { // "draws a Copper, an Estate, and an Elder.", the shuffle, "draws a Copper and a Gold."
		// drawing to 6 shows as +2 Cards or less: a replay takes it for those
		need, k := 6-len(c.Hand(c.P)), len(c.Zone(c.P, deck))
		until := mode("draw until you have 6 cards", "")
		if need > 2 {
			until = engine.Mode{Label: until.Label, Line: "DRAW", Count: need}
			if k > 0 && k < need {
				until.Count = k
			}
		}
		for _, m := range choose(c, 1, until, engine.Mode{Label: "+2 Cards", Line: "DRAW", Count: 2}, engine.Mode{Label: "+1 Card and +1 Action", Line: "DRAW", Count: 1}) {
			switch m {
			case 0:
				drawTo(c, 6)
			case 1:
				c.Draw(2)
			case 2:
				c.Draw(1)
				c.PlusActions(1)
			}
		}
	}})
	reg("Miller", rules{Play: func(c *ctx) { // "looks at 4 cards.", "puts a card into their hand.", "discards 2 other cards and an Estate."
		c.PlusActions(1)
		xs := c.FromTop(c.P, 4, "LOOK_AT")
		c.ToHand(c.P, pickOf(c, c.P, engine.ToPick, xs, 1, 1), "PUT_IN_HAND")
		c.Discard(c.P, c.In(revealed, xs))
	}})
	reg("Elder", rules{Play: func(c *ctx) { // "gets +$2.", "plays a Town.", whose two options follow
		c.PlusCoins(2)
		if x := pick1(c, c.P, engine.ToPlay, hand, action, true); x >= 0 {
			c.T2().Elder = append(c.T2().Elder, x)
			c.PlayNow(x)
		}
	}})

	// Wizards
	reg("Student", rules{Play: func(c *ctx) { // "rotates 3 Liches.", "trashes a Copper.", "gets +1 Favor. (Student)", "topdecks a Student."
		c.PlusActions(1)
		mayRotate(c, "Wizards")
		t := c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
		if len(t) == 1 && c.G.Def(t[0]).Treasure() {
			c.PlusFavors(1)
			c.Topdeck(c.P, inPlayOnly(c, c.Me))
		}
	}})
	reg("Conjurer", rules{
		Play: func(c *ctx) { c.ChooseGain(c.P, c.CostsUpTo(4), discard); c.Stay() },
		Next: func(c *ctx, _ *engine.Dur) bool { // "puts a Conjurer in hand (Conjurer)."
			c.ToHand(c.P, inPlayOnly(c, c.Me), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			return true
		},
	})
	reg("Sorcerer", rules{Play: func(c *ctx) { // the other player "guesses Gold.", "reveals a Copper.", "gains a Curse."
		c.Draw(1)
		c.PlusActions(1)
		c.Attack(func(v int) {
			name := c.NameCard(v)
			c.Log("SORCERER_GUESS", engine.Seat(v), engine.Name(name))
			x := c.TopCard(v)
			if x < 0 {
				return
			}
			c.Reveal(v, one(x))
			c.MoveTo(v, one(x), deck, "")
			if c.G.CardOf(x) != name {
				c.Gain(v, curse(c), discard)
			}
		})
	}})
	reg("Lich", rules{
		Play: func(c *ctx) { // "will skip a Turn."
			c.Draw(6)
			c.PlusActions(2)
			pl := c.Pl(c.P)
			if pl.Skip++; pl.Skip == 1 {
				c.Log("WILL_SKIP_A_TURN", engine.Seat(c.P))
			} else {
				c.Log("WILL_SKIP_A_TURN_COUNT", engine.Seat(c.P), pl.Skip)
			}
		},
		OnTrash: func(c *ctx, _ *ev) { // "discards a Lich. (Lich)", "gains a Sorcerer."
			if c.G.Where(c.Me).Zone != engine.Trash {
				return
			}
			c.G.Move(c.Me, engine.Loc{Zone: discard, Owner: int16(c.P)})
			c.Log("DISCARD_WITH", engine.Seat(c.P), []engine.CardID{c.Src}, engine.Name(c.Src))
			if o := c.ChooseOne(c.P, engine.ToGain, c.OptionsOf(inTrash(c, cheaperThan(c, c.Src))), false); o != nil {
				c.GainInst(c.P, o.Inst, discard, "GAIN")
			}
		},
	})

	// Allies
	reg("Architects' Guild", ally(rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "spends 2 Favors.", "gains a Silver."
			if spend(c, 2, "spend 2 Favors to gain a cheaper non-Victory card") {
				c.ChooseGain(c.P, and(not(victory), cheaperThan(c, e.C)), discard)
			}
		},
	}))
	reg("Order of Astrologers", ally(rules{
		Hears: engine.On(engine.EvShuffling),
		Hear: func(c *ctx, e *ev) { // "spends 1 Favor.", "topdecks a Mountain Shrine.", then the shuffle; not at the start
			xs := c.Zone(c.P, discard)
			if c.G.P[0].Turns == 0 || favors(c, c.P) == 0 || len(xs) == 0 {
				return
			}
			d := c.Deeper()
			if n := d.Amount(c.P, 0, min(favors(c, c.P), len(xs)), "SPENDS_FAVOR", "SPENDS_FAVORS"); n > 0 {
				d.SpendFavors(n)
				d.Topdeck(c.P, pickOf(d, c.P, engine.ToTopdeck, xs, n, n))
			}
		},
	}))
	reg("League of Bankers", ally(rules{
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, _ *ev) {
			phaseLine(c)
			if n := favors(c, c.P) / 4; n > 0 {
				c.Away().PlusCoins(n)
			}
		},
	}))
	reg("Mountain Folk", ally(rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, _ *ev) { // "spends 5 Favors.", "draws 3 cards. (Mountain Folk)"
			phaseLine(c)
			if spend(c, 5, "spend 5 Favors for +3 Cards") {
				c.Away().Draw(3)
			}
		},
	}))
	reg("Woodworkers' Guild", ally(rules{
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, _ *ev) { // "trashes a Village.", "spends 1 Favor.", "gains a Smithy."
			phaseLine(c)
			if favors(c, c.P) == 0 {
				return
			}
			if len(c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, action, true)))) == 1 {
				c.SpendFavors(1)
				c.ChooseGain(c.P, action, discard)
			}
		},
	}))
	reg("Peaceful Cult", ally(rules{
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, _ *ev) { // "spends 2 Favors.", "trashes 2 Coppers."
			phaseLine(c)
			if n := c.Amount(c.P, 0, min(favors(c, c.P), len(c.Hand(c.P))), "SPENDS_FAVOR", "SPENDS_FAVORS"); n > 0 {
				c.SpendFavors(n)
				c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, n, n))
			}
		},
	}))
	reg("Crafters' Guild", ally(rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, _ *ev) { // "gains a Silver. (Crafters' Guild)", "spends 2 Favors."
			phaseLine(c)
			if favors(c, c.P) < 2 {
				return
			}
			if o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(c.CostsUpTo(4)), true); o != nil {
				c.Away().Gain(c.P, o.Card, deck)
				c.SpendFavors(2)
			}
		},
	}))
	reg("Cave Dwellers", ally(rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, _ *ev) { // "discards an Estate. (Cave Dwellers)", "spends 1 Favor.", "draws a card. (Cave Dwellers)"
			phaseLine(c)
			for favors(c, c.P) > 0 {
				x := pick1(c, c.P, engine.ToDiscard, hand, nil, true)
				if x < 0 {
					return
				}
				c.Away().Discard(c.P, one(x))
				c.SpendFavors(1)
				c.Away().Draw(1)
			}
		},
	}))
	reg("Plateau Shepherds", ally(rules{
		VP: func(g *engine.Game, p int) int {
			n := g.Count(p, func(d *def) bool { return g.CostOf(d.ID) == engine.Upto(2) })
			return 2 * min(n, g.P[p].Favors)
		},
	}))
	reg("Desert Guides", ally(rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, _ *ev) { // "spends 1 Favor.", "discards 3 Coppers and 2 Estates. (Desert Guides)", "draws 5 cards. (Desert Guides)"
			phaseLine(c)
			for spend(c, 1, "spend a Favor to discard your hand and draw 5 cards") {
				c.Away().Discard(c.P, slices.Clone(c.Hand(c.P)))
				c.Away().Draw(5)
			}
		},
	}))
	reg("Family of Inventors", ally(rules{
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, _ *ev) { // "places a Favor token on the Smithy pile. (Family of Inventors)"
			phaseLine(c)
			if favors(c, c.P) == 0 {
				return
			}
			var opts []engine.Option
			for i, p := range c.G.Piles {
				if top := c.G.Top(i); p.Supply && top != 0 && !c.G.D(top).Victory() {
					opts = append(opts, engine.Option{Card: p.Card, Inst: -1})
				}
			}
			if o := c.ChooseOne(c.P, engine.ToPick, opts, true); o != nil {
				c.G.P[c.P].Favors--
				c.G.Piles[c.G.PileOf(o.Card)].Favors++
				c.Log("PLAYER_ADDS_FAVOR_TOKEN_TO", engine.Seat(c.P), engine.PileName(o.Card))
			}
		},
		Reduce: func(g *engine.Game, _ inst, c engine.CardID) int {
			if i := g.PileOf(c); i >= 0 {
				return g.Piles[i].Favors
			}
			return 0
		},
	}))
	reg("Island Folk", ally(rules{
		Hears: engine.On(engine.EvTurnEnd),
		Hear: func(c *ctx, _ *ev) { // after the next hand "spends 5 Favors.", "takes an extra turn after this one."
			if c.T2().Extra == 0 && spend(c, 5, "spend 5 Favors for an extra turn") {
				c.Log("ISLAND_FOLK", engine.Seat(c.P))
				c.ExtraTurn()
			}
		},
	}))
	reg("Order of Masons", ally(rules{
		Hears: engine.On(engine.EvShuffling),
		Hear: func(c *ctx, e *ev) { // "spends 1 Favor.", "puts 2 Estates into their discard pile.", then the shuffle
			xs := c.Zone(c.P, discard)
			if favors(c, c.P) == 0 || len(xs) == 0 {
				return
			}
			d := c.Deeper()
			if n := d.Amount(c.P, 0, min(favors(c, c.P), (len(xs)+1)/2), "SPENDS_FAVOR", "SPENDS_FAVORS"); n > 0 {
				d.SpendFavors(n)
				keep := pickOf(d, c.P, engine.ToPick, xs, 0, 2*n)
				if len(keep) > 0 {
					d.Log("PUT_IN_DISCARD", engine.Seat(c.P), kindsOf(c, keep))
				}
				e.Keep = append(e.Keep, keep...)
			}
		},
	}))
	reg("Band of Nomads", ally(rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "spends 1 Favor.", "gets +1 Buy."
			if c.CostOf(e.C).Coins < 3 || !spend(c, 1, "spend a Favor for +1 Card, Action or Buy") {
				return
			}
			switch c.Mode1(c.P, mode("+1 Card", "DRAW"), mode("+1 Action", "GETS_ACTION"), mode("+1 Buy", "GETS_BUY")) {
			case 0:
				c.Draw(1)
			case 1:
				c.PlusActions(1)
			case 2:
				c.PlusBuys(1)
			}
		},
	}))
	reg("Gang of Pickpockets", ally(rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, _ *ev) {
			phaseLine(c)
			if len(c.Hand(c.P)) > 4 && !spend(c, 1, "spend a Favor not to discard down to 4") {
				downTo(c, c.P, 4)
			}
		},
	}))
	reg("Market Towns", ally(rules{
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, _ *ev) { // "spends 1 Favor.", "plays a Village."
			phaseLine(c)
			for c.Count(c.P, hand, action) > 0 && spend(c, 1, "spend a Favor to play an Action") {
				c.PlayNow(pick1(c, c.P, engine.ToPlay, hand, action, false))
			}
		},
	}))
	reg("Forest Dwellers", ally(rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, _ *ev) { // "spends 1 Favor.", "looks at ...", "discards an Estate.", "topdecks 2 cards with Forest Dwellers."
			phaseLine(c)
			if !spend(c, 1, "spend a Favor to look at the top 3 cards") {
				return
			}
			xs := c.FromTop(c.P, 3, "LOOK_AT")
			c.Discard(c.P, pickOf(c, c.P, engine.ToDiscard, xs, 0, len(xs)))
			rest := c.In(revealed, xs)
			if len(rest) > 1 {
				rest = pickOf(c, c.P, engine.ToOrder, rest, len(rest), len(rest))
				slices.Reverse(rest) // the first picked goes on top, so it moves last
			}
			c.Away().TopdeckNamed(c.P, rest)
		},
	}))
	reg("Fellowship of Scribes", ally(rules{
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) { // "spends 1 Favor.", "draws a card."
			if c.G.D(e.C).Action() && len(c.Hand(c.P)) <= 4 && spend(c, 1, "spend a Favor for +1 Card") {
				c.Draw(1)
			}
		},
	}))
	reg("League of Shopkeepers", ally(rules{
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) { // "gets +$1. (League of Shopkeepers)"
			if !liaison(c, e) {
				return
			}
			n, a := favors(c, c.P), c.Away()
			if n >= 5 {
				a.PlusCoins(1)
			}
			if n >= 10 {
				a.PlusActions(1)
				a.PlusBuys(1)
			}
		},
	}))
	reg("Coastal Haven", ally(rules{
		Hears: engine.On(engine.EvCleanup),
		Hear: func(c *ctx, _ *ev) { // without "begins their Cleanup phase.": "spends 2 Favors.", "keeps 2 Estates in hand."
			c.Quietly(func() {
				if n := c.Amount(c.P, 0, min(favors(c, c.P), len(c.Hand(c.P))), "SPENDS_FAVOR", "SPENDS_FAVORS"); n > 0 {
					c.SpendFavors(n)
					xs := pick(c, c.P, engine.ToPick, hand, nil, n, n)
					c.Log("KEEPS_IN_HAND", engine.Seat(c.P), kindsOf(c, xs))
					c.T2().KeepHand = append(c.T2().KeepHand, xs...)
				}
			})
		},
	}))
	reg("City-state", ally(rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "gains a Village.", "spends 2 Favors.", "plays a Village."
			if e.P == c.G.Cur && c.G.D(e.C).Action() && stillGained(c, e) && spend(c, 2, "spend 2 Favors to play it") {
				e.Done = true
				c.PlayNow(e.X)
			}
		},
	}))
	reg("Trappers' Lodge", ally(rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "spends 1 Favor.", "topdecks a Guildmaster with Trappers' Lodge."
			if stillGained(c, e) && spend(c, 1, "spend a Favor to put it onto your deck") {
				e.Done = true
				c.MoveTo(c.P, one(e.X), deck, "TOPDECK_WITH", engine.Name(c.Src))
			}
		},
	}))
	reg("Circle of Witches", ally(rules{
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) { // "spends 3 Favors.", the other player "gains a Curse."
			if liaison(c, e) && spend(c, 3, "spend 3 Favors to give a Curse") {
				c.Gain(c.Other(), curse(c), discard)
			}
		},
	}))
}
