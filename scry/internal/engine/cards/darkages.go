package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Dark Ages: Shelters, Ruins, Spoils, Madman, Mercenary, Knights. Hermit is
// exchanged for a Madman at the end of the Buy phase, as dominion.games logs it.

func costs3to6(c *ctx) func(*def) bool {
	return func(d *def) bool { k := c.CostOf(d.ID); return k.Coins >= 3 && k.LE(engine.Upto(6)) }
}

func gainRuins(c *ctx, p int) { c.GainN(p, c.Card("Ruins"), 1, discard, "GAIN") }

func inTrash(c *ctx, ok func(*def) bool) []inst {
	var out []inst
	for _, x := range c.G.Trashed {
		if ok(c.G.Def(x)) {
			out = append(out, x)
		}
	}
	return out
}

// sack is the Knights' and Rogue's attack: the victim reveals 2 cards, trashes
// one costing from $3 to $6 and discards the rest. It returns the trashed card, or -1.
func sack(c *ctx, v int) inst {
	top := c.FromTop(v, 2, "REVEAL")
	var ok []inst
	for _, x := range top {
		if costs3to6(c)(c.G.Def(x)) {
			ok = append(ok, x)
		}
	}
	t := c.Trash(v, pickOf(c, v, engine.ToTrash, ok, 1, 1))
	c.Discard(v, c.In(revealed, top))
	if len(t) == 1 {
		return t[0]
	}
	return -1
}

func knightAttack(c *ctx, first func(v int)) {
	c.Attack(func(v int) {
		if first != nil {
			first(v)
		}
		if x := sack(c, v); x >= 0 && c.G.Def(x).Is(engine.TKnight) && c.G.Where(c.Me).Zone == inPlay {
			c.Trash(c.P, one(c.Me))
		}
	})
}

func knight(effect func(c *ctx)) rules {
	return rules{Play: func(c *ctx) {
		if effect != nil {
			effect(c)
		}
		knightAttack(c, nil)
	}}
}

func discardDownTo(c *ctx, v, n int) {
	if k := len(c.Hand(v)) - n; k > 0 {
		c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, k, k))
	}
}

func init() {
	needSpoils := func(g *engine.Game) { g.Need("Spoils", 15) }

	// Shelters and Ruins
	reg("Hovel", rules{
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return e.P == c.P && c.G.D(e.C).Victory() },
		React: func(c *ctx, e *ev) bool { // "buys and gains an Estate.", "trashes a Hovel."
			c.Trash(c.P, one(c.Me))
			return true
		},
	})
	reg("Necropolis", rules{Play: func(c *ctx) { c.PlusActions(2) }})
	reg("Overgrown Estate", rules{VP: worth(0), OnTrash: func(c *ctx, e *ev) { c.Draw(1) }})
	reg("Abandoned Mine", rules{Play: func(c *ctx) { c.PlusCoins(1) }})
	reg("Ruined Library", rules{Play: func(c *ctx) { c.Draw(1) }})
	reg("Ruined Market", rules{Play: func(c *ctx) { c.PlusBuys(1) }})
	reg("Ruined Village", rules{Play: func(c *ctx) { c.PlusActions(1) }})
	reg("Survivors", rules{Play: func(c *ctx) {
		xs := c.FromTop(c.P, 2, "LOOK_AT")
		if len(xs) > 0 && c.Mode1(c.P, mode("discard them", "DISCARD"), mode("put them back", "TOPDECK")) == 0 {
			c.Discard(c.P, xs)
			return
		}
		topdeckAny(c, c.P, xs)
	}})

	// cards outside the Supply
	reg("Madman", rules{Play: func(c *ctx) {
		c.PlusActions(2)
		if c.ReturnTo(c.P, c.Me) {
			c.Draw(len(c.Hand(c.P)))
		}
	}})
	reg("Mercenary", rules{Play: func(c *ctx) {
		n := min(2, len(c.Hand(c.P)))
		if n == 0 || !c.Yes(c.P, "trash 2 cards", "TRASH") {
			return
		}
		if len(c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, n, n))) < 2 {
			return
		}
		c.Draw(2)
		c.PlusCoins(2)
		c.Attack(func(v int) { discardDownTo(c, v, 3) })
	}})
	reg("Spoils", rules{Coins: 3, Play: func(c *ctx) { c.ReturnTo(c.P, c.Me) }})

	// Knights
	reg("Dame Anna", knight(func(c *ctx) {
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 2))
	}))
	reg("Dame Josephine", rules{VP: worth(2), Play: func(c *ctx) { knightAttack(c, nil) }})
	reg("Dame Molly", knight(func(c *ctx) { c.PlusActions(2) }))
	reg("Dame Natalie", knight(func(c *ctx) {
		if o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(c.CostsUpTo(3)), true); o != nil {
			c.Gain(c.P, o.Card, discard)
		}
	}))
	reg("Dame Sylvia", knight(func(c *ctx) { c.PlusCoins(2) }))
	reg("Sir Bailey", knight(func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
	}))
	reg("Sir Destry", knight(func(c *ctx) { c.Draw(2) }))
	reg("Sir Martin", knight(func(c *ctx) { c.PlusBuys(2) }))
	reg("Sir Michael", rules{Play: func(c *ctx) {
		knightAttack(c, func(v int) { discardDownTo(c, v, 3) })
	}})
	reg("Sir Vander", rules{
		Play:    func(c *ctx) { knightAttack(c, nil) },
		OnTrash: func(c *ctx, e *ev) { c.Gain(c.P, c.Card("Gold"), discard) },
	})

	// the kingdom
	reg("Altar", rules{Play: func(c *ctx) {
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 1, 1))
		c.ChooseGain(c.P, c.CostsUpTo(5), discard)
	}})
	reg("Armory", rules{Play: func(c *ctx) { c.ChooseGain(c.P, c.CostsUpTo(4), deck) }})
	reg("Band of Misfits", rules{Play: func(c *ctx) {
		ok := and(action, not(func(d *def) bool { return d.Is(engine.TCommand) || d.Is(engine.TDuration) }), cheaperThan(c, c.Src))
		o := c.ChooseOne(c.P, engine.ToPlay, c.SupplyOptions(ok), false)
		if o == nil {
			return
		}
		s := c.G.Piles[c.G.PileOf(o.Card)].Cards
		c.PlayNow(s[len(s)-1]) // from the Supply, leaving it there
	}})
	reg("Bandit Camp", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(2)
			c.Gain(c.P, c.Card("Spoils"), discard)
		},
		Setup: needSpoils,
	})
	reg("Beggar", rules{
		Play:     func(c *ctx) { c.GainN(c.P, c.Card("Copper"), 3, hand, "GAIN") },
		Reacts:   engine.On(engine.EvAttack),
		CanReact: attacked,
		React: func(c *ctx, e *ev) bool { // as Market Square's: "discards a Beggar. (Beggar)"
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			a := c.Away()
			a.Discard(c.P, one(c.Me))
			a.GainAs(c.P, c.Card("Silver"), deck, "GAIN_ON_DRAWPILE")
			a.Gain(c.P, c.Card("Silver"), discard)
			return true
		},
	})
	reg("Catacombs", rules{
		Play: func(c *ctx) {
			xs := c.FromTop(c.P, 3, "LOOK_AT")
			if c.Mode1(c.P, mode("put them into your hand", "PUT_IN_HAND"), mode("discard them, +3 Cards", "DISCARD")) == 0 {
				c.ToHand(c.P, xs, "PUT_IN_HAND")
				return
			}
			c.Discard(c.P, xs)
			c.Draw(3)
		},
		OnTrash: func(c *ctx, e *ev) { c.ChooseGain(c.P, cheaperThan(c, e.C), discard) },
	})
	reg("Count", rules{Play: func(c *ctx) {
		switch c.Mode1(c.P, mode("discard 2 cards", "DISCARD"), mode("put a card from your hand onto your deck", "TOPDECK"), mode("gain a Copper", "GAIN")) {
		case 0:
			n := min(2, len(c.Hand(c.P)))
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, n, n))
		case 1:
			c.Topdeck(c.P, pick(c, c.P, engine.ToTopdeck, hand, nil, 1, 1))
		case 2:
			c.Gain(c.P, c.Card("Copper"), discard)
		}
		switch c.Mode1(c.P, mode("+$3", "GETS_COINS"), mode("trash your hand", "TRASH"), mode("gain a Duchy", "GAIN")) {
		case 0:
			c.PlusCoins(3)
		case 1:
			c.Trash(c.P, append([]inst{}, c.Hand(c.P)...))
		case 2:
			c.Gain(c.P, c.Card("Duchy"), discard)
		}
	}})
	reg("Counterfeit", rules{Coins: 1, Play: func(c *ctx) {
		c.PlusBuys(1)
		x := pick1(c, c.P, engine.ToPlay, hand, and(treasure, not(duration)), true)
		if x < 0 {
			return
		}
		c.PlayCard(x, 0)
		c.PlayCard(x, 1)
		if c.G.Where(x).Zone == inPlay {
			c.Trash(c.P, one(x))
		}
	}})
	reg("Cultist", rules{
		Play: func(c *ctx) {
			c.Draw(2)
			c.Attack(func(v int) { gainRuins(c, v) })
			if x := pick1(c, c.P, engine.ToPlay, hand, named("Cultist"), true); x >= 0 {
				c.PlayNow(x)
			}
		},
		OnTrash: func(c *ctx, e *ev) { c.Draw(3) },
	})
	reg("Death Cart", rules{
		Play: func(c *ctx) {
			var xs []inst
			if c.G.Where(c.Me).Zone == inPlay {
				xs = append(xs, c.Me)
			}
			xs = append(xs, cards(c.Options(c.P, hand, action))...)
			if len(c.Trash(c.P, pickOf(c, c.P, engine.ToTrash, xs, 0, 1))) == 1 {
				c.PlusCoins(5)
			}
		},
		OnGain: func(c *ctx, e *ev) { c.GainN(c.P, c.Card("Ruins"), 2, discard, "GAIN") },
	})
	reg("Feodum", rules{
		VP: func(g *engine.Game, p int) int {
			n := 0
			for _, x := range g.Owned(p) {
				if g.Def(x).Name == "Silver" {
					n++
				}
			}
			return n / 3
		},
		OnTrash: func(c *ctx, e *ev) { c.GainN(c.P, c.Card("Silver"), 3, discard, "GAIN") },
	})
	reg("Forager", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		c.PlusBuys(1)
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 1, 1))
		seen := map[engine.CardID]bool{}
		for _, x := range inTrash(c, treasure) {
			seen[c.G.CardOf(x)] = true
		}
		if len(seen) > 0 {
			c.PlusCoins(len(seen))
		}
	}})
	reg("Fortress", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(2)
		},
		OnTrash: func(c *ctx, e *ev) {
			if c.G.Where(c.Me).Zone == engine.Trash && !e.Done {
				c.G.Move(c.Me, engine.Loc{Zone: hand, Owner: int16(c.P)})
				c.Log("PUT_IN_HAND", engine.Seat(c.P), []engine.CardID{c.Src})
				e.Done = true
			}
		},
	})
	reg("Graverobber", rules{Play: func(c *ctx) {
		if c.Mode1(c.P, mode("gain a card from the trash", "GAIN_ON_DRAWPILE"), mode("trash an Action card from your hand", "TRASH")) == 0 {
			if o := c.ChooseOne(c.P, engine.ToGain, c.OptionsOf(inTrash(c, costs3to6(c))), false); o != nil {
				c.GainInst(c.P, o.Inst, deck, "GAIN_ON_DRAWPILE")
			}
			return
		}
		x := pick1(c, c.P, engine.ToTrash, hand, action, false)
		if x < 0 {
			return
		}
		c.Trash(c.P, one(x))
		c.ChooseGain(c.P, costUp(c, x, 3), discard)
	}})
	reg("Hermit", rules{
		Play: func(c *ctx) {
			c.LookAt(c.P, c.Zone(c.P, discard))
			var xs []inst
			for _, z := range []engine.Zone{discard, hand} {
				xs = append(xs, cards(c.Options(c.P, z, not(treasure)))...)
			}
			c.Trash(c.P, pickOf(c, c.P, engine.ToTrash, xs, 0, 1))
			c.ChooseGain(c.P, c.CostsUpTo(3), discard)
		},
		Hears: engine.On(engine.EvBuyEnd),
		Hear: func(c *ctx, e *ev) {
			if e.P != c.P || c.G.Where(c.Me).Zone != inPlay {
				return
			}
			for _, b := range c.T2().Bought {
				if !c.G.D(b).Landscape() {
					return
				}
			}
			c.Exchange(c.Me, c.Card("Madman"))
		},
		Setup: func(g *engine.Game) { g.Need("Madman", 10) },
	})
	reg("Hunting Grounds", rules{
		Play: func(c *ctx) { c.Draw(4) },
		OnTrash: func(c *ctx, e *ev) {
			if c.Mode1(c.P, mode("gain a Duchy", "GAIN"), mode("gain 3 Estates", "GAIN")) == 0 {
				c.Gain(c.P, c.Card("Duchy"), discard)
				return
			}
			c.GainN(c.P, c.Card("Estate"), 3, discard, "GAIN")
		},
	})
	reg("Ironmonger", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 1, "REVEAL")
		if len(top) == 0 {
			return
		}
		d := c.G.Def(top[0])
		if c.Yes(c.P, "discard it", "DISCARD") {
			c.Discard(c.P, top)
		} else {
			c.Topdeck(c.P, top)
		}
		if d.Action() {
			c.PlusActions(1)
		}
		if d.Treasure() {
			c.PlusCoins(1)
		}
		if d.Victory() {
			c.Draw(1)
		}
	}})
	reg("Junk Dealer", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusCoins(1)
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 1, 1))
	}})
	reg("Marauder", rules{
		Play: func(c *ctx) {
			c.Gain(c.P, c.Card("Spoils"), discard)
			c.Attack(func(v int) { gainRuins(c, v) })
		},
		Setup: needSpoils,
	})
	reg("Market Square", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.PlusBuys(1)
		},
		Reacts:   engine.On(engine.EvTrash),
		CanReact: func(c *ctx, e *ev) bool { return e.P == c.P && e.X != c.Me },
		React: func(c *ctx, e *ev) bool { // "reacts with a Market Square.", "discards a Market Square.", "gains a Gold."; "(Market Square)" on both at the start of a turn
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			a := c
			if c.G.T2.Phase == engine.StartPhase {
				a = c.Away()
			}
			a.Discard(c.P, one(c.Me))
			a.Gain(c.P, c.Card("Gold"), discard)
			return true
		},
	})
	reg("Mystic", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		c.PlusCoins(2)
		named := c.NameCard(c.P)
		x := c.TopCard(c.P)
		if x < 0 {
			return
		}
		if c.G.CardOf(x) == named {
			c.Log("WISH_CORRECT", engine.Seat(c.P), engine.Name(named))
			c.ToHand(c.P, one(x), "")
		} else {
			c.Log("WISH_WRONG", engine.Seat(c.P), engine.Name(named), engine.Name(c.G.CardOf(x)))
			c.MoveTo(c.P, one(x), deck, "")
		}
	}})
	reg("Pillage", rules{
		Play: func(c *ctx) {
			if c.G.Where(c.Me).Zone != inPlay || len(c.Trash(c.P, one(c.Me))) == 0 {
				return
			}
			c.GainN(c.P, c.Card("Spoils"), 2, discard, "GAIN")
			c.Attack(func(v int) {
				if len(c.Hand(v)) < 5 {
					return
				}
				c.RevealHand(v)
				c.Discard(v, cards(c.Choose(c.P, engine.ToDiscard, c.Options(v, hand, nil), 1, 1)))
			})
		},
		Setup: needSpoils,
	})
	reg("Poor House", rules{Play: func(c *ctx) {
		c.PlusCoins(4)
		c.RevealHand(c.P)
		c.LoseCoins(c.Count(c.P, hand, treasure))
	}})
	reg("Procession", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToPlay, hand, and(action, not(func(d *def) bool { return d.Is(engine.TDuration) })), true)
		if x < 0 {
			return
		}
		c.PlayCard(x, 0)
		c.PlayCard(x, 1)
		if c.G.Where(x).Zone == inPlay {
			c.Trash(c.P, one(x))
		}
		c.ChooseGain(c.P, and(action, costExactly(c, x, 1)), discard)
	}})
	reg("Rats", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.Gain(c.P, c.Card("Rats"), discard)
			if c.Count(c.P, hand, not(named("Rats"))) == 0 {
				c.RevealHand(c.P)
				return
			}
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, not(named("Rats")), 1, 1))
		},
		OnTrash: func(c *ctx, e *ev) { c.Draw(1) },
	})
	reg("Rebuild", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		name := c.NameCard(c.P)
		c.Log("NAMES", engine.Seat(c.P), engine.Name(name))
		all, found := c.RevealUntil(c.P, func(d *def) bool { return d.Victory() && d.ID != name }, 1)
		c.Discard(c.P, c.In(revealed, all[:len(all)-len(found)]))
		if len(found) == 0 {
			return
		}
		c.Trash(c.P, found)
		c.ChooseGain(c.P, and(victory, costUp(c, found[0], 3)), discard)
	}})
	reg("Rogue", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		if xs := inTrash(c, costs3to6(c)); len(xs) > 0 {
			o := c.ChooseOne(c.P, engine.ToGain, c.OptionsOf(xs), false)
			c.GainInst(c.P, o.Inst, discard, "GAIN")
			return
		}
		c.Attack(func(v int) { sack(c, v) })
	}})
	reg("Sage", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		revealAndTake(c, func(d *def) bool { return c.CostOf(d.ID).Coins >= 3 }, 1)
	}})
	reg("Scavenger", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		if len(c.Zone(c.P, deck)) > 0 && c.Yes(c.P, "put your deck into your discard pile", "DECK_TO_DISCARD") {
			c.MoveTo(c.P, append([]inst{}, c.Zone(c.P, deck)...), discard, "")
			c.Log("DECK_TO_DISCARD", engine.Seat(c.P))
		}
		c.LookAt(c.P, c.Zone(c.P, discard))
		c.Topdeck(c.P, pick(c, c.P, engine.ToTopdeck, discard, nil, 1, 1))
	}})
	reg("Squire", rules{
		Play: func(c *ctx) {
			c.PlusCoins(1)
			switch c.Mode1(c.P, mode("+2 Actions", "GETS_ACTIONS"), mode("+2 Buys", "GETS_BUYS"), mode("gain a Silver", "GAIN")) {
			case 0:
				c.PlusActions(2)
			case 1:
				c.PlusBuys(2)
			case 2:
				c.Gain(c.P, c.Card("Silver"), discard)
			}
		},
		OnTrash: func(c *ctx, e *ev) { c.ChooseGain(c.P, attack, discard) },
	})
	reg("Storeroom", rules{Play: func(c *ctx) {
		c.PlusBuys(1)
		got := c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 0, len(c.Hand(c.P)))) // "discards 2 cards.", "draws 2 cards."
		c.Draw(len(got))
		if got = c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 0, len(c.Hand(c.P)))); len(got) > 0 {
			c.PlusCoins(len(got))
		}
	}})
	reg("Urchin", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.Attack(func(v int) { discardDownTo(c, v, 4) })
		},
		Hears: engine.On(engine.EvAttack),
		Hear: func(c *ctx, e *ev) { // another Attack played: "trashes an Urchin.", "gains a Mercenary." before its effects
			if e.P != c.P || e.X == c.Me || c.G.Where(c.Me).Zone != inPlay || !c.Yes(c.P, "trash it for a Mercenary", "TRASH") {
				return
			}
			h := c.Here()
			h.Trash(c.P, one(c.Me))
			h.Gain(c.P, c.Card("Mercenary"), discard)
		},
		Setup: func(g *engine.Game) { g.Need("Mercenary", 10) },
	})
	reg("Vagrant", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 1, "REVEAL")
		if len(top) == 0 {
			return
		}
		if d := c.G.Def(top[0]); d.Name == "Curse" || d.Victory() || d.Is(engine.TRuins) || d.Is(engine.TShelter) {
			c.ToHand(c.P, top, "PUT_IN_HAND")
			return
		}
		c.Topdeck(c.P, top) // "topdecks a Copper."
	}})
	reg("Wandering Minstrel", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(2)
		xs := c.FromTop(c.P, 3, "REVEAL")
		var acts []inst
		for _, x := range xs {
			if c.G.Def(x).Action() {
				acts = append(acts, x)
			}
		}
		topdeckAny(c, c.P, acts)
		c.Discard(c.P, c.In(revealed, xs))
	}})
}
