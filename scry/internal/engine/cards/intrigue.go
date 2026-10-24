package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	reg("Baron", rules{Play: func(c *ctx) {
		c.PlusBuys(1)
		if x := pick1(c, c.P, engine.ToDiscard, hand, named("Estate"), true); x >= 0 {
			c.Discard(c.P, one(x))
			c.PlusCoins(4)
			return
		}
		c.Gain(c.P, c.Card("Estate"), discard)
	}})
	reg("Bridge", rules{Play: func(c *ctx) {
		c.PlusBuys(1)
		c.PlusCoins(1)
		c.T2().Reduce++
	}})
	reg("Conspirator", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		if c.T2().ActionsIn >= 3 {
			c.Draw(1)
			c.PlusActions(1)
		}
	}})
	reg("Courtier", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToReveal, hand, nil, false)
		if x < 0 {
			return
		}
		c.Reveal(c.P, one(x))
		n := min(c.G.Def(x).NTypes(), 4)
		for _, m := range c.Modes(c.P, n, n, mode("+1 Action", "GETS_ACTION"), mode("+1 Buy", "GETS_BUY"), mode("+$3", "GETS_COINS"), mode("gain a Gold", "GAIN")) {
			switch m {
			case 0:
				c.PlusActions(1)
			case 1:
				c.PlusBuys(1)
			case 2:
				c.PlusCoins(3)
			case 3:
				c.Gain(c.P, c.Card("Gold"), discard)
			}
		}
	}})
	reg("Courtyard", rules{Play: func(c *ctx) {
		c.Draw(3)
		c.Topdeck(c.P, pick(c, c.P, engine.ToTopdeck, hand, nil, 1, 1))
	}})
	reg("Diplomat", rules{
		Play: func(c *ctx) {
			c.Draw(2)
			if len(c.Hand(c.P)) <= 5 {
				c.PlusActions(2)
			}
		},
		Reacts:   engine.On(engine.EvAttack),
		CanReact: func(c *ctx, e *ev) bool { return attacked(c, e) && len(c.Hand(c.P)) >= 5 },
		React: func(c *ctx, e *ev) bool {
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			c.Draw(2)
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 3, 3))
			return true
		},
	})
	reg("Duke", rules{VP: func(g *engine.Game, p int) int { return g.Count(p, named("Duchy")) }})
	reg("Farm", rules{Coins: 2, VP: worth(2)})
	reg("Ironworks", rules{Play: func(c *ctx) {
		x := c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		if x < 0 {
			return
		}
		d := c.G.Def(x)
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
	reg("Lurker", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		if c.Mode1(c.P, mode("trash an Action from the Supply", "TRASH"), mode("gain an Action from the trash", "GAIN")) == 0 {
			if o := c.ChooseOne(c.P, engine.ToTrash, c.SupplyOptions(action), false); o != nil {
				c.TrashFromSupply(c.P, o.Card)
			}
			return
		}
		var acts []inst
		for _, x := range c.G.Trashed {
			if c.G.Def(x).Action() {
				acts = append(acts, x)
			}
		}
		if got := pickOf(c, c.P, engine.ToGain, acts, 1, 1); len(got) == 1 {
			c.GainInst(c.P, got[0], discard, "GAIN")
		}
	}})
	reg("Masquerade", rules{Play: func(c *ctx) {
		c.Draw(2)
		if len(c.Hand(c.P)) > 0 && len(c.Hand(c.Other())) > 0 {
			mine := pick1(c, c.P, engine.ToPass, hand, nil, false)
			theirs := pick1(c, c.Other(), engine.ToPass, hand, nil, false)
			c.Pass(c.P, c.Other(), mine)
			c.Pass(c.Other(), c.P, theirs)
		}
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 1))
	}})
	reg("Mill", rules{VP: worth(1), Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		if got := c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 0, 2)); len(got) == 2 {
			c.PlusCoins(2)
		}
	}})
	reg("Mining Village", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(2)
		if c.G.Where(c.Me).Zone == inPlay && c.Yes(c.P, "trash it for +$2", "TRASH") {
			c.Trash(c.P, one(c.Me))
			c.PlusCoins(2)
		}
	}})
	reg("Minion", rules{Play: func(c *ctx) {
		c.PlusActions(1)
		if c.Mode1(c.P, mode("+$2", "GETS_COINS"), mode("discard your hand, +4 Cards", "DISCARD")) == 0 {
			c.PlusCoins(2)
			return
		}
		c.Discard(c.P, append([]inst{}, c.Hand(c.P)...))
		c.Draw(4)
		c.Attack(func(v int) {
			if len(c.Hand(v)) >= 5 {
				c.Discard(v, append([]inst{}, c.Hand(v)...))
				c.DrawFor(v, 4)
			}
		})
	}})
	reg("Nobles", rules{VP: worth(2), Play: func(c *ctx) {
		if c.Mode1(c.P, mode("+3 Cards", "DRAW"), mode("+2 Actions", "GETS_ACTIONS")) == 0 {
			c.Draw(3)
		} else {
			c.PlusActions(2)
		}
	}})
	reg("Patrol", rules{Play: func(c *ctx) {
		c.Draw(3)
		top := c.FromTop(c.P, 4, "REVEAL")
		var green []inst
		for _, x := range top {
			if c.G.Def(x).Is(engine.TVictory | engine.TCurse) {
				green = append(green, x)
			}
		}
		c.ToHand(c.P, green, "PUT_IN_HAND")
		topdeckAny(c, c.P, c.In(revealed, top))
	}})
	reg("Pawn", rules{Play: func(c *ctx) {
		for _, m := range c.Modes(c.P, 2, 2, mode("+1 Card", "DRAW"), mode("+1 Action", "GETS_ACTION"), mode("+1 Buy", "GETS_BUY"), mode("+$1", "GETS_COIN")) {
			switch m {
			case 0:
				c.Draw(1)
			case 1:
				c.PlusActions(1)
			case 2:
				c.PlusBuys(1)
			case 3:
				c.PlusCoins(1)
			}
		}
	}})
	reg("Replace", rules{Play: func(c *ctx) {
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		c.Trash(c.P, one(x))
		y := c.ChooseGain(c.P, costUp(c, x, 2), discard)
		if y < 0 {
			return
		}
		d := c.G.Def(y)
		if d.Is(engine.TAction|engine.TTreasure) && c.G.Where(y).Zone == discard {
			c.Topdeck(c.P, one(y))
		}
		if d.Victory() {
			c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
		}
	}})
	reg("Secret Passage", rules{Play: func(c *ctx) {
		c.Draw(2)
		c.PlusActions(1)
		x := pick1(c, c.P, engine.ToTopdeck, hand, nil, false)
		if x < 0 {
			return
		}
		opts := []engine.Option{{Inst: -1, Mode: 0, Label: "on top", Line: "TOPDECK"}}
		for n := 1; n <= len(c.Zone(c.P, deck)); n++ {
			opts = append(opts, engine.Option{Inst: -1, Mode: n, Label: "under " + itoa(n), Line: "INSERT_IN_DECK"})
		}
		c.Insert(c.P, x, c.Choose(c.P, engine.ToChoose, opts, 1, 1)[0].Mode)
	}})
	reg("Shanty Town", rules{Play: func(c *ctx) {
		c.PlusActions(2)
		c.RevealHand(c.P)
		if c.Count(c.P, hand, action) == 0 {
			c.Draw(2)
		}
	}})
	reg("Steward", rules{Play: func(c *ctx) {
		switch c.Mode1(c.P, mode("+2 Cards", "DRAW"), mode("+$2", "GETS_COINS"), mode("trash 2 cards", "TRASH")) {
		case 0:
			c.Draw(2)
		case 1:
			c.PlusCoins(2)
		case 2:
			n := min(2, len(c.Hand(c.P)))
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, n, n))
		}
	}})
	reg("Swindler", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		c.Attack(func(v int) {
			x := c.TopCard(v)
			if x < 0 {
				return
			}
			c.Trash(v, one(x))
			cost := c.CostOf(c.G.CardOf(x))
			if o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(func(d *def) bool { return c.CostOf(d.ID) == cost }), false); o != nil {
				c.Gain(v, o.Card, discard)
			}
		})
	}})
	reg("Torturer", rules{Play: func(c *ctx) {
		c.Draw(3)
		c.Attack(func(v int) {
			if c.Mode1(v, mode("discard 2 cards", "DISCARD"), mode("gain a Curse to your hand", "GAIN")) == 0 {
				n := min(2, len(c.Hand(v)))
				c.Discard(v, pick(c, v, engine.ToDiscard, hand, nil, n, n))
			} else {
				c.Gain(v, c.Card("Curse"), hand)
			}
		})
	}})
	reg("Trading Post", rules{Play: func(c *ctx) {
		n := min(2, len(c.Hand(c.P)))
		if got := c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, n, n)); len(got) == 2 {
			c.Gain(c.P, c.Card("Silver"), hand)
		}
	}})
	reg("Upgrade", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 {
			return
		}
		c.Trash(c.P, one(x))
		c.ChooseGain(c.P, costExactly(c, x, 1), discard)
	}})
	reg("Wishing Well", rules{Play: func(c *ctx) {
		c.Draw(1)
		c.PlusActions(1)
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
}
