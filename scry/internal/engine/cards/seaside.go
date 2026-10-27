package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Seaside, second edition.
func init() {
	reg("Astrolabe", rules{Coins: 1,
		Play: func(c *ctx) {
			c.PlusBuys(1)
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool { // "gets +1 Buy. (Astrolabe)", "gets +$1. (Astrolabe)"
			c.PlusBuys(1)
			c.PlusCoins(1)
			return true
		},
	})
	reg("Bazaar", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(2)
		c.PlusCoins(1)
	}})
	reg("Blockade", rules{
		Play: func(c *ctx) {
			o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(c.CostsUpTo(4)), false)
			if o == nil {
				return
			}
			x := c.GainAs(c.P, o.Card, aside, "GAIN_SETTING_ASIDE")
			if x < 0 || c.G.Where(x).Zone != aside {
				return
			}
			d := c.Stay()
			d.Aside = one(x)
			if c.Unaffected(c.Other()) {
				d.N = 1
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.ToHand(c.P, c.In(aside, d.Aside), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			return true
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if e.P == c.P || e.P != c.G.Cur {
				return
			}
			for _, d := range c.Durs() {
				if d.N == 0 && len(c.In(aside, d.Aside)) == 1 && c.G.CardOf(d.Aside[0]) == e.C {
					c.Here().Gain(e.P, c.Card("Curse"), discard) // "gains a Curse.", not "with Blockade"
				}
			}
		},
	})
	reg("Caravan", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.Draw(1)
			return true
		},
	})
	reg("Corsair", rules{
		Play: func(c *ctx) {
			c.PlusCoins(2)
			d := c.Stay()
			if c.Unaffected(c.Other()) {
				d.N = 1
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.Draw(1)
			return true
		},
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) {
			if e.P == c.P || !silverOrGold(c.G.D(e.C)) {
				return
			}
			first := true
			for _, x := range c.T2().Played {
				if x != e.X && silverOrGold(c.G.Def(x)) {
					first = false
				}
			}
			for _, d := range c.Durs() {
				if first && d.N == 0 { // "trashes a Silver.", unnamed
					c.As(e.P).Here().Trash(e.P, one(e.X))
					return
				}
			}
		},
	})
	reg("Cutpurse", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		c.Attack(func(v int) {
			if x := pick1(c, v, engine.ToDiscard, hand, named("Copper"), false); x >= 0 {
				c.Discard(v, one(x))
				return
			}
			c.RevealHand(v)
		})
	}})
	reg("Fishing Village", rules{
		Play: func(c *ctx) {
			c.PlusActions(2)
			c.PlusCoins(1)
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.PlusActions(1)
			c.PlusCoins(1)
			return true
		},
	})
	reg("Haven", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			if x := pick1(c, c.P, engine.ToSetAside, hand, nil, false); x >= 0 {
				c.SetAsideWith(c.P, x)
				c.Stay().Aside = one(x)
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.ToHand(c.P, c.In(aside, d.Aside), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			return true
		},
	})
	reg("Island", rules{VP: worth(2), Play: func(c *ctx) {
		c.PutOnMat(c.P, one(c.Me), engine.Island)
		c.PutOnMat(c.P, one(pick1(c, c.P, engine.ToSetAside, hand, nil, false)), engine.Island)
	}})
	reg("Lighthouse", rules{
		Play: func(c *ctx) {
			c.PlusActions(1)
			c.PlusCoins(1)
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.PlusCoins(1)
			return true
		},
		Hears: engine.On(engine.EvAttack),
		Hear: func(c *ctx, e *ev) {
			if e.Victim == c.P && len(c.Durs()) > 0 {
				e.Unaffected = true
			}
		},
	})
	reg("Lookout", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		xs := c.FromTop(c.P, 3, "LOOK_AT")
		if t := pickOf(c, c.P, engine.ToTrash, xs, 1, 1); len(t) == 1 {
			c.Trash(c.P, t)
			xs = c.In(revealed, xs)
		}
		if d := pickOf(c, c.P, engine.ToDiscard, xs, 1, 1); len(d) == 1 {
			c.Discard(c.P, d)
			xs = c.In(revealed, xs)
		}
		c.Topdeck(c.P, xs)
	}})
	reg("Merchant Ship", rules{
		Play: func(c *ctx) {
			c.PlusCoins(2)
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.PlusCoins(2)
			return true
		},
	})
	reg("Monkey", rules{
		Play: func(c *ctx) { c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool {
			c.Draw(1)
			return true
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if e.P != c.P {
				for range c.Durs() {
					c.Draw(1)
				}
			}
		},
	})
	reg("Native Village", rules{Play: func(c *ctx) {
		c.PlusActions(2)
		if c.Mode1(c.P, mode("set aside the top card", "SET_ASIDE_WITH"), mode("take the mat's cards", "PUT_IN_HAND")) == 0 {
			if x := c.TopCard(c.P); x >= 0 {
				c.MoveTo(c.P, one(x), engine.NativeVillage, "SET_ASIDE_WITH", engine.Name(c.Src))
			}
			return
		}
		c.ToHand(c.P, append([]inst{}, c.Zone(c.P, engine.NativeVillage)...), "PUT_IN_HAND")
	}})
	reg("Outpost", rules{
		Play: func(c *ctx) {
			n := 0
			for _, x := range c.T2().Played {
				if c.G.CardOf(x) == c.Src {
					n++
				}
			}
			if c.T2().Extra != 0 || n > 1 {
				return
			}
			c.T2().HandSize = 3
			c.ExtraTurn()
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool { return true },
	})
	reg("Pirate", rules{
		Play: func(c *ctx) { c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool {
			c.ChooseGain(c.P, and(treasure, c.CostsUpTo(6)), hand)
			return true
		},
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return c.G.D(e.C).Treasure() },
		React: func(c *ctx, e *ev) bool {
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			c.PlayNow(c.Me)
			return true
		},
	})
	reg("Sailor", rules{
		Play: func(c *ctx) {
			c.PlusActions(1)
			c.AddWatch(engine.On(engine.EvGain), 0)
			c.Stay()
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P || !c.G.D(e.C).Is(engine.TDuration) || !stillGained(c, e) {
				return false
			}
			if !c.Yes(c.P, "play it", "PLAY") { // "plays a Sailor.", unnamed
				return false
			}
			e.Done = true
			c.PlayNow(e.X)
			return true
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.PlusCoins(2)
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 1))
			return true
		},
	})
	reg("Salvager", rules{Play: func(c *ctx) {
		c.PlusBuys(1)
		if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
			cost := c.CostOf(c.G.CardOf(x)).Coins
			c.Trash(c.P, one(x))
			if cost > 0 {
				c.PlusCoins(cost)
			}
		}
	}})
	reg("Sea Chart", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		x := c.FromTop(c.P, 1, "REVEAL")
		if len(x) == 0 {
			return
		}
		if c.Count(c.P, inPlay, func(d *def) bool { return d.ID == c.G.CardOf(x[0]) }) > 0 {
			c.ToHand(c.P, x, "PUT_IN_HAND")
			return
		}
		c.MoveTo(c.P, x, deck, "BACK_ON_DECK")
	}})
	reg("Sea Witch", rules{
		Play: func(c *ctx) {
			c.Draw(2)
			c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.Draw(2)
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, min(2, len(c.Hand(c.P))), 2))
			return true
		},
	})
	reg("Smugglers", rules{Play: func(c *ctx) {
		var opts []engine.Option
		seen := map[engine.CardID]bool{}
		for _, card := range c.G.PrevGained(c.Other()) {
			if i := c.G.PileOf(card); !seen[card] && i >= 0 && c.G.Top(i) == card && c.CostOf(card).LE(engine.Upto(6)) {
				seen[card] = true
				opts = append(opts, engine.Option{Card: card, Inst: -1})
			}
		}
		if o := c.ChooseOne(c.P, engine.ToGain, opts, false); o != nil {
			c.Gain(c.P, o.Card, discard)
		}
	}})
	reg("Tactician", rules{
		Play: func(c *ctx) {
			if len(c.Discard(c.P, append([]inst{}, c.Hand(c.P)...))) > 0 {
				c.Stay()
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.Draw(5)
			c.PlusBuys(1)
			c.PlusActions(1)
			return true
		},
	})
	reg("Tide Pools", rules{
		Play: func(c *ctx) {
			c.Draw(3)
			c.PlusActions(1)
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, min(2, len(c.Hand(c.P))), 2))
			return true
		},
	})
	reg("Treasure Map", rules{Play: func(c *ctx) {
		n := len(c.Trash(c.P, one(c.Me)))
		if x := pick1(c, c.P, engine.ToTrash, hand, named("Treasure Map"), false); x >= 0 {
			n += len(c.Trash(c.P, one(x)))
		}
		if n == 2 {
			c.GainN(c.P, c.Card("Gold"), 4, deck, "GAIN_ON_DRAWPILE")
		}
	}})
	reg("Treasury", rules{
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			c.PlusCoins(1)
		},
		OnDiscard: func(c *ctx, e *ev) {
			if !e.Cleanup || c.G.Where(c.Me) != (engine.Loc{Zone: discard, Owner: int16(c.P)}) { // a Scheme took it first
				return
			}
			for _, b := range c.T2().BuyGained {
				if c.G.D(b).Victory() {
					return
				}
			}
			if c.Yes(c.P, "put it on your deck", "TOPDECK") {
				c.Topdeck(c.P, one(c.Me))
			}
		},
	})
	reg("Warehouse", rules{Play: func(c *ctx) {
		c.Draw(3)
		c.PlusActions(1)
		c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, min(3, len(c.Hand(c.P))), 3))
	}})
	reg("Wharf", rules{
		Play: func(c *ctx) {
			c.Draw(2)
			c.PlusBuys(1)
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.Draw(2)
			c.PlusBuys(1)
			return true
		},
	})
}

func silverOrGold(d *def) bool { return d.Name == "Silver" || d.Name == "Gold" }
