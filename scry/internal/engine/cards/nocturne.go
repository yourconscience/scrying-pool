package cards

import (
	"slices"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Nocturne: Boons, Hexes, States, Heirlooms, Spirits and Night cards.

// nextOf takes the top card of the Boon or Hex deck, shuffling its discards when it is empty; 0 for none.
func nextOf(c *ctx, deck, out *[]engine.CardID) engine.CardID {
	if len(*deck) == 0 {
		c.G.Reshuffle(deck, out)
	}
	d := *deck
	if len(d) == 0 {
		return 0
	}
	*deck = d[:len(d)-1]
	return d[len(d)-1]
}

// takeBoon takes the next Boon: "takes The Sea's Gift."; 0 for none.
func takeBoon(c *ctx) engine.CardID {
	b := nextOf(c, &c.G.Boons, &c.G.BoonsOut)
	if b != 0 {
		c.Log("TAKE", engine.Seat(c.P), []engine.CardID{b})
	}
	return b
}

// receive gives player p a Boon's or Hex's effect, its lines at this level; line writes
// "receives The Sea's Gift." first (not Pixie's first time, nor Sacred Grove's other player).
func receive(c *ctx, p int, b engine.CardID, line bool) {
	if line {
		c.Log("RECEIVES", engine.Seat(p), []engine.CardID{b})
	}
	c.G.D(b).Rules.Play(c.Receiving(p, b))
}

// keeps are the Boons kept until Clean-up.
var keeps = nameSet("The Field's Gift", "The Forest's Gift", "The River's Gift")

// boonDone discards a Boon the player received, or keeps it until Clean-up.
func boonDone(c *ctx, b engine.CardID) {
	if keeps[c.G.D(b).Name] {
		c.Receiving(c.P, b).AddWatch(engine.On(engine.EvCleanup), 0)
		return
	}
	discardBoon(c, b)
}

// discardBoon: "discards The Sea's Gift."
func discardBoon(c *ctx, b engine.CardID) {
	c.Log("DISCARD", engine.Seat(c.P), []engine.CardID{b})
	c.G.BoonsOut = append(c.G.BoonsOut, b)
}

// boon: the player receives the next Boon. "takes The Sea's Gift.", "receives The Sea's Gift.",
// "draws a Copper.", "discards The Sea's Gift."
func boon(c *ctx) engine.CardID {
	b := takeBoon(c)
	if b != 0 {
		receive(c, c.P, b, true)
		boonDone(c, b)
	}
	return b
}

// kept discards a Boon kept until Clean-up, before the Clean-up draw: "discards The Field's Gift."
// N 1 is The River's Gift's card for a player whose turn it isn't, at the end of the turn.
func kept(c *ctx, _ *ev, w *engine.Watch) bool {
	l := c.At(0)
	if w.N == 1 {
		l.DrawFor(w.P, 1)
		return true
	}
	l.Quietly(func() { l.Log("DISCARD", engine.Seat(w.P), []engine.CardID{c.Src}) })
	c.G.BoonsOut = append(c.G.BoonsOut, c.Src)
	return true
}

// hex: player v receives the next Hex, which this card's player takes and discards.
// "takes War.", "receives War.", "reveals a Gold and a Skulk.", ..., "discards War."
func hex(c *ctx, v int) {
	h := nextOf(c, &c.G.Hexes, &c.G.HexesOut)
	if h == 0 {
		return
	}
	c.Log("TAKE", engine.Seat(c.P), []engine.CardID{h})
	receive(c, v, h, true)
	c.Log("DISCARD", engine.Seat(c.P), []engine.CardID{h})
	c.G.HexesOut = append(c.G.HexesOut, h)
}

// hexAttack: each other player receives the next Hex.
func hexAttack(c *ctx) { c.Attack(func(v int) { hex(c, v) }) }

// drop returns a State: "returns Envious."
func drop(c *ctx, p int, name string) bool {
	s := c.Card(name)
	for i, h := range c.G.Held {
		if h.C == s && h.P == p {
			c.G.Held = slices.Delete(c.G.Held, i, i+1)
			c.Log("RETURN", engine.Seat(p), []engine.CardID{s})
			return true
		}
	}
	return false
}

// toHand moves a card gained to the discard pile into hand: Den of Sin, Ghost Town, Guardian, Night Watchman.
func toHand(c *ctx, e *ev) {
	if e.To == discard && stillGained(c, e) {
		c.MoveTo(c.P, one(c.Me), hand, "")
		e.To = hand
	}
}

// playUnique plays an Action from hand that the player has no copy of in play (Imp, Conclave).
func playUnique(c *ctx) bool {
	x := pick1(c, c.P, engine.ToPlay, hand, func(d *def) bool { return d.Action() && c.Count(c.P, inPlay, named(d.Name)) == 0 }, true)
	if x < 0 {
		return false
	}
	c.PlayNow(x)
	return true
}

// boonOptions are Boons to pick from: Druid's, Fool's.
func boonOptions(bs []engine.CardID) []engine.Option {
	opts := make([]engine.Option, len(bs))
	for i, b := range bs {
		opts[i] = engine.Option{Card: b, Inst: -1, Mode: i}
	}
	return opts
}

func others(xs, not []inst) []inst {
	var out []inst
	for _, x := range xs {
		if !slices.Contains(not, x) {
			out = append(out, x)
		}
	}
	return out
}

func only(c *ctx, xs []inst, ok func(*def) bool) []inst {
	var out []inst
	for _, x := range xs {
		if ok(c.G.Def(x)) {
			out = append(out, x)
		}
	}
	return out
}

// givesCoin are the Boons with +$1, which Sacred Grove doesn't share.
var givesCoin = nameSet("The Field's Gift", "The Forest's Gift")

// shareLine is the first line a Boon writes, for a replay to tell whether Sacred Grove's other player received it.
var shareLine = map[string]string{"The Earth's Gift": "DISCARD", "The Flame's Gift": "TRASH", "The Moon's Gift": "LOOK_AT",
	"The Mountain's Gift": "GAIN", "The River's Gift": "DRAW", "The Sea's Gift": "DRAW", "The Sky's Gift": "DISCARD",
	"The Sun's Gift": "LOOK_AT", "The Swamp's Gift": "GAIN", "The Wind's Gift": "DRAW"}

var spirits = []string{"Will-o'-Wisp", "Imp", "Ghost"}

func need(names ...string) func(*engine.Game) {
	sizes := map[string]int{"Will-o'-Wisp": 12, "Imp": 13, "Ghost": 6, "Wish": 12, "Bat": 10}
	return func(g *engine.Game) {
		for _, n := range names {
			g.Need(n, sizes[n])
		}
	}
}

func init() {
	// the Boons
	reg("The Earth's Gift", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToDiscard, hand, treasure, true); x >= 0 && len(c.Discard(c.P, one(x))) == 1 {
			c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		}
	}})
	reg("The Field's Gift", rules{Play: func(c *ctx) { c.PlusActions(1); c.PlusCoins(1) }, Watch: kept})
	reg("The Flame's Gift", rules{Play: func(c *ctx) {
		c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, true)))
	}})
	reg("The Forest's Gift", rules{Play: func(c *ctx) { c.PlusBuys(1); c.PlusCoins(1) }, Watch: kept})
	reg("The Moon's Gift", rules{Play: func(c *ctx) { // "looks at 3 Coppers and an Estate.", "topdecks an Estate."
		c.LookAt(c.P, c.Zone(c.P, discard))
		c.Topdeck(c.P, one(pick1(c, c.P, engine.ToTopdeck, discard, nil, true)))
	}})
	reg("The Mountain's Gift", rules{Play: func(c *ctx) { c.Gain(c.P, c.Card("Silver"), discard) }})
	reg("The River's Gift", rules{
		Play: func(c *ctx) { // +1 Card at the end of the turn: "draws 6 cards."
			if c.P == c.G.Cur {
				c.T2().HandSize++
			} else {
				c.AddWatch(engine.On(engine.EvTurnEnd), 1).Long = true
			}
		},
		Watch: kept,
	})
	reg("The Sea's Gift", rules{Play: func(c *ctx) { c.Draw(1) }})
	reg("The Sky's Gift", rules{Play: func(c *ctx) { // its discard names no card at the start of a turn, its gain does
		if len(c.Hand(c.P)) >= 3 && c.Yes(c.P, "discard 3 cards", "DISCARD") {
			if len(c.Here().Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 3, 3))) == 3 {
				c.Gain(c.P, c.Card("Gold"), discard)
			}
			return
		}
		c.Log("THE_SKYS_GIFT_FAIL", engine.Seat(c.P))
	}})
	reg("The Sun's Gift", rules{Play: func(c *ctx) { // at the start of a turn its discard is plain, its topdeck "with The Sun's Gift"
		top := c.FromTop(c.P, 4, "LOOK_AT")
		c.Here().Discard(c.P, pickOf(c, c.P, engine.ToDiscard, top, 0, len(top)))
		xs := c.In(revealed, top)
		if len(xs) > 1 {
			xs = pickOf(c, c.P, engine.ToOrder, xs, len(xs), len(xs))
			slices.Reverse(xs) // the first picked goes on top, so it moves last
		}
		c.TopdeckNamed(c.P, xs)
	}})
	reg("The Swamp's Gift", rules{Play: func(c *ctx) { c.Gain(c.P, c.Card("Will-o'-Wisp"), discard) }})
	reg("The Wind's Gift", rules{Play: func(c *ctx) {
		c.Draw(2)
		c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 2, 2))
	}})

	// the Hexes
	reg("Bad Omens", rules{Play: func(c *ctx) { // "moves their deck to the discard.", "looks at 8 cards.", "topdecks 2 Coppers."
		if len(c.Zone(c.P, deck)) > 0 { // no line for an empty deck
			c.MoveTo(c.P, slices.Clone(c.Zone(c.P, deck)), discard, "")
			c.Log("DECK_TO_DISCARD", engine.Seat(c.P))
		}
		c.LookAt(c.P, c.Zone(c.P, discard))
		var coppers []inst
		for _, x := range c.Zone(c.P, discard) {
			if c.G.Def(x).Name == "Copper" && len(coppers) < 2 {
				coppers = append(coppers, x)
			}
		}
		c.Topdeck(c.P, coppers)
		if len(coppers) < 2 {
			c.Reveal(c.P, slices.Clone(c.Zone(c.P, discard)))
		}
	}})
	reg("Delusion", rules{Play: func(c *ctx) {
		if !holds(c.G, c.P, "Deluded") && !holds(c.G, c.P, "Envious") {
			take(c, c.P, "Deluded")
		}
	}})
	reg("Envy", rules{Play: func(c *ctx) {
		if !holds(c.G, c.P, "Deluded") && !holds(c.G, c.P, "Envious") {
			take(c, c.P, "Envious")
		}
	}})
	reg("Famine", rules{Play: func(c *ctx) { // "reveals 3 Coppers.", "shuffles their deck.", "shuffles 3 Coppers into their deck."
		top := c.FromTop(c.P, 3, "REVEAL")
		c.Discard(c.P, only(c, top, action))
		if rest := c.In(revealed, top); len(rest) > 0 {
			c.ShuffleLine(c.P)
			c.ShuffleInto(c.P, rest)
		}
	}})
	reg("Fear", rules{Play: func(c *ctx) {
		if len(c.Hand(c.P)) < 5 {
			return
		}
		if x := pick1(c, c.P, engine.ToDiscard, hand, func(d *def) bool { return d.Action() || d.Treasure() }, false); x >= 0 {
			c.Discard(c.P, one(x))
		} else {
			c.RevealHand(c.P)
		}
	}})
	reg("Greed", rules{Play: func(c *ctx) { c.GainAs(c.P, c.Card("Copper"), deck, "GAIN") }})
	reg("Haunting", rules{Play: func(c *ctx) {
		if len(c.Hand(c.P)) >= 4 {
			c.Topdeck(c.P, one(pick1(c, c.P, engine.ToTopdeck, hand, nil, false)))
		}
	}})
	reg("Locusts", rules{Play: func(c *ctx) { // "trashes a Silver.", "gains a Copper."
		x := c.TopCard(c.P)
		if x < 0 {
			return
		}
		c.Trash(c.P, one(x))
		d := c.G.Def(x)
		if d.Name == "Copper" || d.Name == "Estate" {
			c.Gain(c.P, c.Card("Curse"), discard)
			return
		}
		cost := c.CostOf(d.ID)
		c.ChooseGain(c.P, func(e *def) bool { return c.CostOf(e.ID).Less(cost) && e.T&d.T != 0 }, discard)
	}})
	reg("Misery", rules{Play: func(c *ctx) {
		switch {
		case holds(c.G, c.P, "Twice Miserable"):
		case drop(c, c.P, "Miserable"): // "returns Miserable.", "takes Twice Miserable."
			take(c, c.P, "Twice Miserable")
		default:
			take(c, c.P, "Miserable")
		}
	}})
	reg("Plague", rules{Play: func(c *ctx) { c.GainAs(c.P, c.Card("Curse"), hand, "GAIN") }})
	reg("Poverty", rules{Play: func(c *ctx) {
		if n := len(c.Hand(c.P)) - 3; n > 0 {
			c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, n, n))
		}
	}})
	reg("War", rules{Play: func(c *ctx) {
		all, found := c.RevealUntil(c.P, func(d *def) bool { k := c.CostOf(d.ID); return k == engine.Upto(3) || k == engine.Upto(4) }, 1)
		c.Trash(c.P, found)
		c.Discard(c.P, c.In(revealed, all))
	}})

	// the States
	reg("Lost in the Woods", rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) { // "discards an Estate. (Lost In The Woods)", "takes The Sky's Gift.", ...
			if e.P != c.P {
				return
			}
			phaseLine(c) // "starts their turn." whether or not a card is discarded
			if x := pick1(c, c.P, engine.ToDiscard, hand, nil, true); x >= 0 && len(c.Away().Discard(c.P, one(x))) == 1 {
				boon(c.Away())
			}
		},
	})
	reg("Deluded", rules{
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, e *ev) { // "begins their buy phase.", "returns Deluded."
			if e.P == c.P && drop(c.At(0), c.P, "Deluded") {
				c.T2().Deluded = true
			}
		},
	})
	reg("Envious", rules{
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, e *ev) {
			if e.P == c.P && drop(c.At(0), c.P, "Envious") {
				c.T2().Envious = true
			}
		},
	})
	reg("Miserable", rules{VP: worth(-2)})
	reg("Twice Miserable", rules{VP: worth(-4)})

	// the Heirlooms
	reg("Haunted Mirror", rules{Coins: 1, OnTrash: func(c *ctx, _ *ev) {
		if x := pick1(c, c.P, engine.ToDiscard, hand, action, true); x >= 0 && len(c.Discard(c.P, one(x))) == 1 {
			c.Gain(c.P, c.Card("Ghost"), discard)
		}
	}})
	reg("Lucky Coin", rules{Coins: 1, Play: func(c *ctx) { c.Gain(c.P, c.Card("Silver"), discard) }})
	reg("Goat", rules{Coins: 1, Play: func(c *ctx) { c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, true))) }})
	reg("Cursed Gold", rules{Coins: 3, Play: func(c *ctx) { c.Gain(c.P, c.Card("Curse"), discard) }})
	reg("Magic Lamp", rules{Coins: 1, Play: func(c *ctx) { // "trashes a Magic Lamp.", "gains 3 Wishes."
		n := map[engine.CardID]int{}
		for _, x := range c.Zone(c.P, inPlay) {
			n[c.G.CardOf(x)]++
		}
		single := 0
		for _, k := range n {
			if k == 1 {
				single++
			}
		}
		if single >= 6 && len(c.Trash(c.P, inPlayOnly(c, c.Me))) == 1 {
			c.GainN(c.P, c.Card("Wish"), 3, discard, "GAIN")
		}
	}})
	reg("Pasture", rules{Coins: 1, VP: func(g *engine.Game, p int) int { return g.Count(p, named("Estate")) }})
	reg("Pouch", rules{Coins: 1, Play: func(c *ctx) { c.PlusBuys(1) }})

	// the Spirits, Wish, Bat and the Zombies
	reg("Will-o'-Wisp", rules{Play: func(c *ctx) { // "reveals a Silver.", "topdecks a Silver."
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 1, "REVEAL")
		if len(top) == 0 {
			return
		}
		if c.CostOf(c.G.CardOf(top[0])).LE(engine.Upto(2)) {
			c.ToHand(c.P, top, "PUT_IN_HAND")
		} else {
			c.MoveTo(c.P, top, deck, "TOPDECK")
		}
	}})
	reg("Imp", rules{Play: func(c *ctx) { c.Draw(2); playUnique(c) }})
	reg("Ghost", rules{
		Play: func(c *ctx) { // "reveals a Silver and a Witch.", "discards a Silver.", "sets a Witch aside."
			all, found := c.RevealUntil(c.P, action, 1)
			c.Discard(c.P, others(c.In(revealed, all), found))
			if xs := c.SetAside(c.P, c.In(revealed, found)); len(xs) == 1 {
				c.Stay().Aside = xs
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool { // "plays a Witch. (Ghost)" twice
			if xs := c.In(aside, d.Aside); len(xs) == 1 {
				c.PlayCardWith(xs[0], 0)
				c.PlayCardWith(xs[0], 1)
			}
			return true
		},
	})
	reg("Wish", rules{Play: func(c *ctx) { // "returns a Wish to the Wish pile.", "gains a Destrier."
		c.PlusActions(1)
		if c.ReturnTo(c.P, c.Me) {
			c.ChooseGain(c.P, c.CostsUpTo(6), hand)
		}
	}})
	reg("Bat", rules{Play: func(c *ctx) { // "returns a Bat.", "receives a Vampire."
		if len(c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 2))) > 0 {
			c.Exchange(c.Me, c.Card("Vampire"))
		}
	}})
	reg("Zombie Apprentice", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToTrash, hand, action, true); x >= 0 && len(c.Trash(c.P, one(x))) == 1 {
			c.Draw(3)
			c.PlusActions(1)
		}
	}})
	reg("Zombie Mason", rules{Play: func(c *ctx) {
		x := c.TopCard(c.P)
		if x < 0 {
			return
		}
		c.Trash(c.P, one(x))
		if o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(costUp(c, x, 1)), true); o != nil {
			c.Gain(c.P, o.Card, discard)
		}
	}})
	reg("Zombie Spy", rules{Play: func(c *ctx) { // "looks at an Estate.", "topdecks an Estate."
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 1, "LOOK_AT")
		if len(top) == 0 {
			return
		}
		if c.Mode1(c.P, mode("discard it", "DISCARD"), mode("put it back", "TOPDECK")) == 0 {
			c.Discard(c.P, top)
		} else {
			c.MoveTo(c.P, top, deck, "TOPDECK")
		}
	}})

	// the kingdom cards
	reg("Bard", rules{Play: func(c *ctx) { c.PlusCoins(2); boon(c) }})
	reg("Blessed Village", rules{
		Play: func(c *ctx) { c.Draw(1); c.PlusActions(2) },
		OnGain: func(c *ctx, _ *ev) { // "takes The Wind's Gift.", then "receives" it or "sets The Wind's Gift aside."
			b := takeBoon(c)
			if b == 0 {
				return
			}
			if c.Mode1(c.P, mode("receive it now", "RECEIVES"), mode("receive it next turn", "SET_ASIDE")) == 0 {
				receive(c, c.P, b, true)
				boonDone(c, b)
				return
			}
			c.Log("SET_ASIDE", engine.Seat(c.P), []engine.CardID{b})
			c.Later().N = int(b)
		},
		Next: func(c *ctx, d *engine.Dur) bool { // "receives The Wind's Gift.", "draws 2 cards. (Blessed Village)"
			b := engine.CardID(d.N)
			receive(c, c.P, b, true)
			boonDone(c, b)
			return true
		},
	})
	reg("Cemetery", rules{VP: worth(2), Setup: need("Ghost"), OnGain: func(c *ctx, _ *ev) {
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 4))
	}})
	reg("Changeling", rules{
		Setup: func(g *engine.Game) { g.Global = append(g.Global, g.T.MustCard("Changeling")) },
		Play: func(c *ctx) { // "trashes a Changeling.", "gains a Gold."
			c.Trash(c.P, inPlayOnly(c, c.Me))
			var opts []engine.Option
			for _, x := range c.Zone(c.P, inPlay) {
				k := c.G.CardOf(x)
				if i := c.G.PileOf(k); i >= 0 && c.G.Top(i) == k && !slices.ContainsFunc(opts, func(o engine.Option) bool { return o.Card == k }) {
					opts = append(opts, engine.Option{Card: k, Inst: -1})
				}
			}
			if o := c.ChooseOne(c.P, engine.ToGain, opts, false); o != nil {
				c.Gain(c.P, o.Card, discard)
			}
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "gains a Silver.", "returns a Silver.", "receives a Changeling."
			if c.Me >= 0 || e.C == c.Src || c.CostOf(e.C).Coins < 3 || !stillGained(c, e) || c.G.Left(c.Src) == 0 {
				return
			}
			if c.Mode1(e.P, mode("exchange it for a Changeling", "EXCHANGE_RETURN"), mode("keep it", "")) == 0 {
				e.Done = true
				c.As(e.P).Exchange(e.X, c.Src)
			}
		},
	})
	reg("Cobbler", rules{
		Play: func(c *ctx) { c.Stay() },
		Next: func(c *ctx, _ *engine.Dur) bool { c.ChooseGain(c.P, c.CostsUpTo(4), hand); return true },
	})
	reg("Conclave", rules{Play: func(c *ctx) {
		c.PlusCoins(2)
		if playUnique(c) {
			c.PlusActions(1)
		}
	}})
	reg("Crypt", rules{
		Play: func(c *ctx) { // "sets 2 Coppers aside."
			if xs := c.SetAside(c.P, pick(c, c.P, engine.ToSetAside, inPlay, and(treasure, not(duration)), 0, 99)); len(xs) > 0 {
				c.Stay().Aside = xs
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool { // "puts a Gold in hand (Crypt)."
			left := c.In(aside, d.Aside)
			if len(left) == 0 {
				return true
			}
			c.ToHand(c.P, pickOf(c, c.P, engine.ToPick, left, 1, 1), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			d.Aside = c.In(aside, left) // it no longer has that card, even if another Crypt sets it aside again
			return len(d.Aside) == 0
		},
	})
	reg("Cursed Village", rules{
		Play: func(c *ctx) {
			c.PlusActions(2)
			drawTo(c, 6)
		},
		OnGain: func(c *ctx, _ *ev) { hex(c, c.P) },
	})
	reg("Den of Sin", rules{
		Play:   func(c *ctx) { c.Stay() },
		Next:   func(c *ctx, _ *engine.Dur) bool { c.Draw(2); return true },
		OnGain: toHand,
	})
	reg("Devil's Workshop", rules{Setup: need("Imp"), Play: func(c *ctx) {
		switch n := len(c.T2().Gained); {
		case n >= 2:
			c.Gain(c.P, c.Card("Imp"), discard)
		case n == 1:
			c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		default:
			c.Gain(c.P, c.Card("Gold"), discard)
		}
	}})
	reg("Druid", rules{
		Start: func(g *engine.Game, me engine.CardID) { // "Druid sets The Field's Gift aside."
			cx := g.CtxFor(0, me, 0)
			for range 3 {
				if b := nextOf(cx, &g.Boons, &g.BoonsOut); b != 0 {
					g.Druid = append(g.Druid, b)
					cx.Log("DRUID_SELECTS_BOONS", engine.Name(0), []engine.CardID{b})
				}
			}
		},
		Play: func(c *ctx) { // "receives The Field's Gift.", left set aside
			c.PlusBuys(1)
			if o := c.ChooseOne(c.P, engine.ToPick, boonOptions(c.G.Druid), false); o != nil {
				receive(c, c.P, o.Card, true)
			}
		},
	})
	reg("Exorcist", rules{Setup: need(spirits...), Play: func(c *ctx) { // "trashes an Estate.", "gains a Will-o'-Wisp."
		x := pick1(c, c.P, engine.ToTrash, hand, nil, false)
		if x < 0 || len(c.Trash(c.P, one(x))) == 0 {
			return
		}
		cost := c.CostOf(c.G.CardOf(x))
		var opts []engine.Option
		for _, n := range spirits {
			s := c.Card(n)
			if i := c.G.PileOf(s); i >= 0 && c.G.Top(i) == s && c.CostOf(s).Less(cost) {
				opts = append(opts, engine.Option{Card: s, Inst: -1})
			}
		}
		if o := c.ChooseOne(c.P, engine.ToGain, opts, false); o != nil {
			c.Gain(c.P, o.Card, discard)
		}
	}})
	reg("Faithful Hound", rules{
		Play: func(c *ctx) { c.Draw(2) },
		OnDiscard: func(c *ctx, e *ev) { // "discards a Faithful Hound.", "reacts with a Faithful Hound.", "sets a Faithful Hound aside."
			if e.Cleanup || c.G.Where(c.Me) != (engine.Loc{Zone: discard, Owner: int16(c.P)}) || !c.Yes(c.P, "set it aside", "REACTS_WITH") {
				return
			}
			c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
			c.SetAside(c.P, one(c.Me))
			c.AddWatch(engine.On(engine.EvTurnEnd), int(c.Me)).Long = true
		},
		Watch: func(c *ctx, _ *ev, w *engine.Watch) bool { // after the Clean-up draw: "puts a Faithful Hound into their hand."
			c.At(0).ToHand(w.P, c.In(aside, one(inst(w.N))), "PUT_IN_HAND")
			return true
		},
	})
	reg("Fool", rules{Play: func(c *ctx) { // "takes Lost In The Woods.", "takes" 3 Boons, "receives" each in turn
		if holds(c.G, c.P, "Lost in the Woods") {
			return
		}
		take(c, c.P, "Lost in the Woods")
		var bs []engine.CardID
		for range 3 {
			if b := takeBoon(c); b != 0 {
				bs = append(bs, b)
			}
		}
		for len(bs) > 0 {
			i := c.ChooseOne(c.P, engine.ToPick, boonOptions(bs), false).Mode
			b := bs[i]
			bs = slices.Delete(bs, i, i+1)
			receive(c, c.P, b, true)
			boonDone(c, b)
		}
	}})
	reg("Ghost Town", rules{
		Play:   func(c *ctx) { c.Stay() },
		Next:   func(c *ctx, _ *engine.Dur) bool { c.Draw(1); c.PlusActions(1); return true },
		OnGain: toHand,
	})
	reg("Guardian", rules{
		Play:   func(c *ctx) { c.Stay() },
		Next:   func(c *ctx, _ *engine.Dur) bool { c.PlusCoins(1); return true },
		OnGain: toHand,
		Hears:  engine.On(engine.EvAttack),
		Hear: func(c *ctx, e *ev) {
			if e.Victim == c.P && len(c.Durs()) > 0 {
				e.Unaffected = true
			}
		},
	})
	reg("Idol", rules{Coins: 2, Play: func(c *ctx) {
		if c.Count(c.P, inPlay, named("Idol"))%2 == 1 {
			boon(c)
			return
		}
		c.Attack(func(v int) { c.Gain(v, c.Card("Curse"), discard) })
	}})
	reg("Leprechaun", rules{Setup: need("Wish"), Play: func(c *ctx) {
		c.Gain(c.P, c.Card("Gold"), discard)
		if c.Count(c.P, inPlay, nil) == 7 {
			c.Gain(c.P, c.Card("Wish"), discard)
		} else {
			hex(c, c.P)
		}
	}})
	reg("Monastery", rules{Play: func(c *ctx) { // "trashes 2 Estates." from hand, then "trashes a Copper." from play
		n := len(c.T2().Gained)
		if n > 0 {
			n -= len(c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, n)))
		}
		if n > 0 {
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, inPlay, named("Copper"), 0, n))
		}
	}})
	reg("Necromancer", rules{
		Setup: func(g *engine.Game) {
			for _, z := range []string{"Zombie Apprentice", "Zombie Mason", "Zombie Spy"} {
				g.AddToTrash(z)
			}
		},
		Play: func(c *ctx) { // "plays a Zombie Spy.", left in the trash, face down for the turn: a watch that never fires
			var opts []engine.Option
			for _, x := range c.G.Trashed {
				d := c.G.Def(x)
				if d.Action() && !d.Is(engine.TDuration) && !slices.ContainsFunc(c.G.Watches(), func(w engine.Watch) bool { return w.C == c.Src && w.N == int(x) }) {
					opts = append(opts, engine.Option{Card: d.ID, Inst: x})
				}
			}
			if o := c.ChooseOne(c.P, engine.ToPlay, opts, false); o != nil {
				c.AddWatch(0, int(o.Inst))
				c.PlayHere(o.Inst)
			}
		},
		Watch: func(*ctx, *ev, *engine.Watch) bool { return false },
	})
	reg("Night Watchman", rules{
		Play: func(c *ctx) {
			top := c.FromTop(c.P, 5, "LOOK_AT")
			c.Discard(c.P, pickOf(c, c.P, engine.ToDiscard, top, 0, len(top)))
			topdeckAny(c, c.P, c.In(revealed, top))
		},
		OnGain: toHand,
	})
	reg("Pixie", rules{Play: func(c *ctx) { // "takes The Swamp's Gift.", "trashes a Pixie.", "gains a Will-o'-Wisp.", "receives The Swamp's Gift.", ...
		c.Draw(1)
		c.PlusActions(1)
		b := takeBoon(c)
		if b == 0 {
			return
		}
		if c.G.Where(c.Me).Zone == inPlay && c.Yes(c.P, "trash it to receive the Boon twice", "TRASH") && len(c.Trash(c.P, one(c.Me))) == 1 {
			receive(c, c.P, b, false)
			receive(c, c.P, b, true)
			boonDone(c, b)
			return
		}
		discardBoon(c, b)
	}})
	reg("Pooka", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToTrash, hand, and(treasure, not(named("Cursed Gold"))), true); x >= 0 && len(c.Trash(c.P, one(x))) == 1 {
			c.Draw(4)
		}
	}})
	reg("Raider", rules{
		Play: func(c *ctx) { // "discards a Ducat.", or "reveals their hand: ..."
			names := map[string]bool{}
			for _, x := range c.Zone(c.P, inPlay) {
				names[c.G.Def(x).Name] = true
			}
			c.Attack(func(v int) {
				if len(c.Hand(v)) < 5 {
					return
				}
				if x := pick1(c, v, engine.ToDiscard, hand, func(d *def) bool { return names[d.Name] }, false); x >= 0 {
					c.Discard(v, one(x))
				} else {
					c.RevealHand(v)
				}
			})
			c.Stay()
		},
		Next: func(c *ctx, _ *engine.Dur) bool { c.PlusCoins(3); return true },
	})
	reg("Sacred Grove", rules{Play: func(c *ctx) { // the other player's effect comes after the discard, without lines of its own
		c.PlusBuys(1)
		c.PlusCoins(3)
		b := boon(c)
		if name := c.G.D(b).Name; b != 0 && !givesCoin[name] && c.Yes(c.Other(), "receive "+name, shareLine[name]) {
			receive(c, c.Other(), b, false)
		}
	}})
	reg("Secret Cave", rules{
		Setup: need("Wish"),
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(1)
			if len(c.Hand(c.P)) >= 3 && c.Yes(c.P, "discard 3 cards", "DISCARD") && len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, 3, 3))) == 3 {
				c.Stay()
			}
		},
		Next: func(c *ctx, _ *engine.Dur) bool { c.PlusCoins(3); return true },
	})
	reg("Shepherd", rules{Play: func(c *ctx) { // "discards 2 Estates.", "draws 4 cards."
		c.PlusActions(1)
		c.Draw(2 * len(c.Discard(c.P, pick(c, c.P, engine.ToDiscard, hand, victory, 0, 99))))
	}})
	reg("Skulk", rules{
		Play:   func(c *ctx) { c.PlusBuys(1); hexAttack(c) },
		OnGain: func(c *ctx, _ *ev) { c.Gain(c.P, c.Card("Gold"), discard) },
	})
	reg("Tormentor", rules{Setup: need("Imp"), Play: func(c *ctx) {
		c.PlusCoins(2)
		if slices.ContainsFunc(c.Zone(c.P, inPlay), func(x inst) bool { return x != c.Me }) {
			hexAttack(c)
		} else {
			c.Gain(c.P, c.Card("Imp"), discard)
		}
	}})
	reg("Tracker", rules{
		Play:  func(c *ctx) { c.PlusCoins(1); boon(c) },
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "gains a Silver.", "topdecks a Silver with Tracker."
			if e.P != c.P || e.To == deck || c.G.Where(c.Me).Zone != inPlay || !stillGained(c, e) {
				return
			}
			if c.Yes(c.P, "put it onto your deck", "TOPDECK_WITH") {
				e.Done = true
				c.MoveTo(c.P, one(e.X), deck, "TOPDECK_WITH", engine.Name(c.Src))
			}
		},
	})
	reg("Tragic Hero", rules{Play: func(c *ctx) {
		c.Draw(3)
		c.PlusBuys(1)
		if len(c.Hand(c.P)) >= 8 {
			c.Trash(c.P, inPlayOnly(c, c.Me))
			c.ChooseGain(c.P, treasure, discard)
		}
	}})
	reg("Vampire", rules{Setup: need("Bat"), Play: func(c *ctx) { // "gains a Duchy.", "returns a Vampire.", "receives a Bat."
		hexAttack(c)
		c.ChooseGain(c.P, and(c.CostsUpTo(5), not(named("Vampire"))), discard)
		c.Exchange(c.Me, c.Card("Bat"))
	}})
	reg("Werewolf", rules{Play: func(c *ctx) {
		if c.T2().Phase == engine.NightPhase {
			hexAttack(c)
			return
		}
		c.Draw(3)
	}})
}
