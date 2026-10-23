package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	reg("Copper", rules{Coins: 1})
	reg("Silver", rules{Coins: 2})
	reg("Gold", rules{Coins: 3})
	reg("Platinum", rules{Coins: 5})
	reg("Potion", rules{Play: func(c *ctx) { c.PlusPotions(1) }})
	reg("Estate", rules{VP: worth(1)})
	reg("Duchy", rules{VP: worth(3)})
	reg("Province", rules{VP: worth(6)})
	reg("Colony", rules{VP: worth(10)})
	reg("Curse", rules{VP: worth(-1)})

	reg("Artisan", rules{Play: func(c *ctx) {
		c.ChooseGain(c.P, c.CostsUpTo(5), hand)
		c.Topdeck(c.P, pick(c, c.P, engine.ToTopdeck, hand, nil, 1, 1))
	}})
	reg("Bandit", rules{Play: func(c *ctx) {
		c.Gain(c.P, c.Card("Gold"), discard)
		c.Attack(func(v int) {
			top := c.FromTop(v, 2, "REVEAL")
			var loot []inst
			for _, x := range top {
				if d := c.G.Def(x); d.Treasure() && d.Name != "Copper" {
					loot = append(loot, x)
				}
			}
			c.Trash(v, pickOf(c, v, engine.ToTrash, loot, 1, 1))
			c.Discard(v, c.In(revealed, top))
		})
	}})
	reg("Bureaucrat", rules{Play: func(c *ctx) {
		c.Gain(c.P, c.Card("Silver"), deck)
		c.Attack(func(v int) {
			if c.Count(v, hand, victory) == 0 {
				c.RevealHand(v)
				return
			}
			c.Topdeck(v, pick(c, v, engine.ToTopdeck, hand, victory, 1, 1))
		})
	}})
	reg("Cellar", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		got := c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 0, len(c.Hand(c.P))))
		c.Draw(len(got))
	}})
	reg("Chapel", rules{Play: func(c *ctx) {
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 4))
	}})
	reg("Council Room", rules{Play: func(c *ctx) {
		c.Draw(4)
		c.PlusBuys(1)
		c.DrawFor(c.Other(), 1)
	}})
	reg("Festival", rules{Play: func(c *ctx) {
		c.PlusActions(2)
		c.PlusBuys(1)
		c.PlusCoins(2)
	}})
	reg("Gardens", rules{VP: func(g *engine.Game, p int) int { return len(g.Owned(p)) / 10 }})
	reg("Harbinger", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		pile := c.Zone(c.P, discard)
		c.LookAt(c.P, pile)
		c.Topdeck(c.P, pick(c, c.P, engine.ToTopdeck, discard, nil, 0, 1))
	}})
	reg("Laboratory", rules{Play: func(c *ctx) {
		c.Draw(2)
		c.PlusActions(1)
	}})
	reg("Library", rules{Play: func(c *ctx) {
		var skipped []inst
		for len(c.Hand(c.P)) < 7 {
			x := c.DrawOne(c.P)
			if x < 0 {
				break
			}
			c.LookAt(c.P, one(x)) // the client logs each card Library draws as looked at
			if c.G.Def(x).Action() && len(pickOf(c, c.P, engine.ToSetAside, one(x), 0, 1)) == 1 {
				c.SetAsideWith(c.P, x)
				skipped = append(skipped, x)
			}
		}
		c.Discard(c.P, skipped)
	}})
	reg("Market", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusBuys(1)
		c.PlusCoins(1)
	}})
	reg("Merchant", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			for _, x := range c.T2().Played {
				if c.G.Def(x).Name == "Silver" {
					return // the first Silver was already played
				}
			}
			c.AddWatch(engine.On(engine.EvPlay), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P || c.G.D(e.C).Name != "Silver" {
				return false
			}
			c.CoinsFrom(1)
			return true
		},
	})
	reg("Militia", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		c.Attack(func(v int) {
			if n := len(c.Hand(v)) - 3; n > 0 {
				c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, n, n))
			}
		})
	}})
	reg("Mine", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToTrash, hand, treasure, true)
		if x < 0 {
			return
		}
		c.Trash(c.P, one(x))
		c.ChooseGain(c.P, and(treasure, costUp(c, x, 3)), hand)
	}})
	reg("Moat", rules{
		Play:     func(c *ctx) { c.Draw(2) },
		Reacts:   engine.On(engine.EvAttack),
		CanReact: attacked,
		React:    moat,
	})
	reg("Moneylender", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToTrash, hand, named("Copper"), true); x >= 0 {
			c.Trash(c.P, one(x))
			c.PlusCoins(3)
		}
	}})
	reg("Poacher", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusCoins(1)
		if n := c.G.EmptyPiles(); n > 0 {
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, n, n))
		}
	}})
	reg("Remodel", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		c.Trash(c.P, one(x))
		c.ChooseGain(c.P, costUp(c, x, 2), discard)
	}})
	reg("Sentry", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 2, "LOOK_AT")
		c.Trash(c.P, pickOf(c, c.P, engine.ToTrash, top, 0, len(top)))
		rest := c.In(revealed, top)
		c.Discard(c.P, pickOf(c, c.P, engine.ToDiscard, rest, 0, len(rest)))
		topdeckAny(c, c.P, c.In(revealed, rest))
	}})
	reg("Smithy", rules{Play: func(c *ctx) { c.Draw(3) }})
	reg("Throne Room", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToPlay, hand, action, true)
		if x < 0 {
			return
		}
		c.PlayCard(x, 0)
		c.PlayCard(x, 1)
	}})
	reg("Vassal", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		x := c.DiscardTop(c.P)
		if x >= 0 && c.G.Def(x).Action() && len(pickOf(c, c.P, engine.ToPlay, one(x), 0, 1)) == 1 {
			c.PlayCard(x, 0)
		}
	}})
	reg("Village", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(2)
	}})
	reg("Witch", rules{Play: func(c *ctx) {
		c.Draw(2)
		c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
	}})
	reg("Workshop", rules{Play: func(c *ctx) { c.ChooseGain(c.P, c.CostsUpTo(4), discard) }})

	// first edition
	reg("Adventurer", rules{Play: func(c *ctx) {
		all, found := c.RevealUntil(c.P, treasure, 2)
		c.ToHand(c.P, found, "PUT_IN_HAND")
		c.Discard(c.P, c.In(revealed, all))
	}})
	reg("Chancellor", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		if len(c.Zone(c.P, deck)) > 0 && c.Yes(c.P, "put your deck into your discard pile", "DECK_TO_DISCARD") {
			c.MoveTo(c.P, append([]inst{}, c.Zone(c.P, deck)...), discard, "")
			c.Log("DECK_TO_DISCARD", engine.Seat(c.P))
		}
	}})
	reg("Feast", rules{Play: func(c *ctx) {
		c.Trash(c.P, c.In(inPlay, one(c.Me)))
		c.ChooseGain(c.P, c.CostsUpTo(5), discard)
	}})
	reg("Spy", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		spy := func(v int) {
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
		spy(c.P)
		c.Attack(spy)
	}})
	reg("Thief", rules{Play: func(c *ctx) {
		c.Attack(func(v int) {
			top := c.FromTop(v, 2, "REVEAL")
			var ts []inst
			for _, x := range top {
				if c.G.Def(x).Treasure() {
					ts = append(ts, x)
				}
			}
			got := c.Trash(v, pickOf(c, c.P, engine.ToTrash, ts, 1, 1))
			for _, x := range got {
				if len(pickOf(c, c.P, engine.ToGain, got, 0, 1)) == 1 {
					c.GainInst(c.P, x, discard, "GAIN")
				}
			}
			c.Discard(v, c.In(revealed, top))
		})
	}})
	reg("Woodcutter", rules{Play: func(c *ctx) {
		c.PlusBuys(1)
		c.PlusCoins(2)
	}})
}
