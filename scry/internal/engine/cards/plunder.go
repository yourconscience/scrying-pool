package cards

import (
	"slices"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Plunder: the Loot, the Events and the Traits. Simplifications: Search
// hears a pile emptying only by a gain; Fated cards only go on top; Harbor
// Village counts any rise in $ during the next Action as "it gave you +$".

func loot(g *engine.Game) { g.AddLoot() }

// inPlayOnly is the card when it is still in play: a Reckless Duration's second
// "this" finds it moved by the first.
func inPlayOnly(c *ctx, x inst) []inst {
	if c.G.Where(x).Zone == inPlay {
		return one(x)
	}
	return nil
}

// gainLoot gains the top Loot, revealing it first: "reveals a Jewels.", "gains a Jewels."; -1 if none is left.
func gainLoot(c *ctx, p int, to engine.Zone) inst {
	i := c.G.PileOf(c.Card("Loot"))
	if i < 0 || c.G.Top(i) == 0 {
		return -1
	}
	top := c.G.Top(i)
	c.Log("REVEAL", engine.Seat(p), []engine.CardID{top})
	return c.Gain(p, top, to)
}

// kindsOf is the cards of instances, for a log line.
func kindsOf(c *ctx, xs []inst) []engine.CardID {
	out := make([]engine.CardID, len(xs))
	for i, x := range xs {
		out[i] = c.G.CardOf(x)
	}
	return out
}

// playLine is the line a card writes when another card plays it: "plays a Gold. (+$3)" for a Treasure of fixed $.
func playLine(d *def) string {
	if d.Treasure() && !d.Action() && d.Rules != nil && !d.Rules.Alone && (d.Rules.Coins != 0 || d.Rules.Worth != nil) { // "plays a Potion." has no $
		return "PLAY_TREASURES_FOR"
	}
	return "PLAY"
}

// treasureNames counts the differently named Treasures a player has in play.
func treasureNames(g *engine.Game, p int) int {
	seen := map[engine.CardID]bool{}
	for _, x := range g.P[p].Z[inPlay] {
		if g.Def(x).Treasure() {
			seen[g.CardOf(x)] = true
		}
	}
	return len(seen)
}

// traited tests for a card of the pile with this Trait.
func traited(c *ctx) func(*def) bool {
	return func(d *def) bool { return c.G.TraitOf(d.ID) == c.Src }
}

// traitTop is the top card of the pile with this Trait, 0 for none.
func traitTop(c *ctx) engine.CardID {
	for i, p := range c.G.Piles {
		if p.Trait == c.Src {
			return c.G.Top(i)
		}
	}
	return 0
}

func init() {
	curse := func(c *ctx) engine.CardID { return c.Card("Curse") }

	// Kingdom cards
	reg("Cage", rules{Alone: true,
		Play: func(c *ctx) { // "sets 3 cards aside with Cage."
			xs := c.SetAsideWith(c.P, pick(c, c.P, engine.ToSetAside, hand, nil, 0, 4)...)
			c.Stay().Aside = xs
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "buys and gains a Province.", "trashes a Cage."; after the Clean-up draw "puts 3 cards in hand (Cage)."
			if e.P != c.P || !c.G.D(e.C).Victory() {
				return
			}
			for _, d := range c.Durs() {
				if d.N == 0 {
					d.N = 1
					c.Here().Trash(c.P, inPlayOnly(c, c.Me))
					c.AddWatch(engine.On(engine.EvTurnEnd), 0).Long = true
				}
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			for _, d := range c.G.Durs() {
				if d.C == c.Src && d.P == w.P && d.N == 1 && !d.Done {
					d.Done = true
					c.At(0).ToHand(w.P, c.In(aside, d.Aside), "PUT_IN_HAND_FROM", engine.Name(c.Src))
				}
			}
			return true
		},
	})
	reg("Grotto", rules{
		Play: func(c *ctx) {
			c.PlusActions(1)
			if xs := c.SetAsideWith(c.P, pick(c, c.P, engine.ToSetAside, hand, nil, 0, 4)...); len(xs) > 0 {
				c.Stay().Aside = xs // it doesn't stay without them, as Gear
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool { // "discards a card and an Estate.", "draws 2 cards. (Grotto)"
			xs := c.In(aside, d.Aside) // set aside face down: the replay learns them here
			n := len(c.Here().Discard(c.P, pickOf(c, c.P, engine.ToDiscard, xs, len(xs), len(xs))))
			c.Draw(n)
			return true
		},
	})
	reg("Jewelled Egg", rules{Coins: 1,
		Play:    func(c *ctx) { c.PlusBuys(1) },
		OnTrash: func(c *ctx, _ *ev) { gainLoot(c, c.P, discard) },
		Setup:   loot,
	})
	reg("Search", rules{
		Play:  func(c *ctx) { c.PlusCoins(2); c.Stay() },
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "trashes a Search.", "Search will stop functioning after this turn.", then the Loot
			i := c.G.PileOf(e.C)
			if i < 0 || !c.G.Piles[i].Supply || len(c.G.Piles[i].Cards) > 0 {
				return
			}
			for _, d := range c.Durs() {
				d.Done = true
				h := c.Here()
				if len(h.Trash(c.P, inPlayOnly(c, c.Me))) == 1 {
					h.Log("DURATION_STOPS_WORKING", engine.Seat(c.P), engine.Name(c.Src))
				}
				gainLoot(h, c.P, discard)
			}
		},
		Setup: loot,
	})
	reg("Shaman", rules{
		Play: func(c *ctx) {
			c.PlusActions(1)
			c.PlusCoins(1)
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 1))
		},
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) { // at the start of each turn, "gains a Silver. (Shaman)" from the trash
			if c.Me >= 0 {
				return
			}
			phaseLine(c)
			a := c.Away()
			if o := a.ChooseOne(c.P, engine.ToGain, a.OptionsOf(inTrash(c, c.CostsUpTo(6))), false); o != nil {
				a.GainInst(c.P, o.Inst, discard, "GAIN")
			}
		},
		Setup: func(g *engine.Game) { g.Global = append(g.Global, g.T.MustCard("Shaman")) },
	})
	reg("Secluded Shrine", rules{
		Play:  func(c *ctx) { c.PlusCoins(1); c.Stay() },
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if e.P != c.P || !c.G.D(e.C).Treasure() {
				return
			}
			for _, d := range c.Durs() { // "trashes 2 Estates.", unnamed
				d.Done = true
				c.Here().Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 2))
			}
		},
	})
	reg("Siren", rules{
		Play: func(c *ctx) {
			c.Attack(func(v int) { c.Gain(v, curse(c), discard) })
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			drawTo(c, 8)
			return true
		},
		OnGain: func(c *ctx, e *ev) { // "trashes an Importer."; or the Siren, or "Siren can no longer move Siren (it already moved)."
			if x := pick1(c, c.P, engine.ToTrash, hand, action, true); x >= 0 {
				c.Trash(c.P, one(x))
			} else if stillGained(c, e) {
				c.Trash(c.P, one(e.X))
			} else {
				c.Log("LOSE_TRACK_MOVED", engine.Name(c.Src), engine.Name(c.Src))
			}
		},
	})
	reg("Stowaway", rules{
		Play:     func(c *ctx) { c.Stay() },
		Next:     func(c *ctx, d *engine.Dur) bool { c.Draw(2); return true },
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return c.G.D(e.C).Is(engine.TDuration) },
		React:    reactPlaying,
	})
	taskmaster := func(c *ctx) { c.PlusActions(1); c.PlusCoins(1) }
	reg("Taskmaster", rules{ // d.N: a card costing exactly $5 was gained this turn
		Play: func(c *ctx) { taskmaster(c); c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool {
			taskmaster(c)
			d.N = 0
			return false
		},
		Hears: engine.On(engine.EvGain, engine.EvCleanup),
		Hear: func(c *ctx, e *ev) {
			for _, d := range c.Durs() {
				switch {
				case e.Kind == engine.EvGain && e.P == c.P && c.G.Cur == c.P && c.CostOf(e.C) == engine.Upto(5):
					d.N = 1
				case e.Kind == engine.EvCleanup && e.P == c.P && d.N == 0:
					d.Done = true
				}
			}
		},
	})
	reg("Abundance", rules{Alone: true,
		Play:  func(c *ctx) { c.Stay() },
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "gets +1 Buy.", "gets +$3."; "(Abundance)" at the start of a turn
			if e.P != c.P || !c.G.D(e.C).Action() {
				return
			}
			if c.T2().Phase != engine.StartPhase {
				c = c.Here()
			}
			for _, d := range c.Durs() {
				d.Done = true
				c.PlusBuys(1)
				c.PlusCoins(3)
			}
		},
	})
	reg("Cabin Boy", rules{
		Play: func(c *ctx) { c.Draw(1); c.PlusActions(1); c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool { // "gets +$2. (Cabin Boy)"; or "trashes a Cabin Boy. (Cabin Boy)", "gains a Prince. (Cabin Boy)"
			if c.Mode1(c.P, mode("+$2", "GETS_COINS_FROM"), mode("trash this to gain a Duration card", "TRASH_WITH")) == 0 {
				c.PlusCoins(2)
			} else if len(c.Trash(c.P, one(c.Me))) == 1 {
				c.ChooseGain(c.P, duration, discard)
			}
			return true
		},
	})
	reg("Crucible", rules{Alone: true, Play: func(c *ctx) { // "trashes an Estate."; the $ comes without a line
		if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
			n := c.CostOf(c.G.CardOf(x)).Coins
			c.Trash(c.P, one(x))
			c.Pl(c.P).Coins += n
		}
	}})
	reg("Flagship", rules{
		Play:  func(c *ctx) { c.PlusCoins(2); c.Stay() },
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) { // after the next Action's effects, "plays a Sailor. (Flagship)"
			d := c.G.D(e.C)
			if e.P != c.P || e.X == c.Me || !d.Action() || d.Is(engine.TCommand) || e.By == c.Src { // even a card that trashed itself; a second Flagship replays the first play too, not the replay
				return
			}
			if ds := c.Durs(); len(ds) > 0 {
				ds[0].Done = true
				held := append(append([]inst{}, ds[0].Hold...), c.Me) // with the Royal Carriage that replayed this Flagship
				c.PlayCardWith(e.X, 1)
				for _, d := range c.G.Durs() { // it stays as long as the Duration it replayed, whichever play keeps that
					for _, h := range held {
						if d.X == e.X && !d.Done && !slices.Contains(d.Hold, h) {
							d.Hold = append(d.Hold, h)
						}
					}
				}
			}
		},
	})
	reg("Fortune Hunter", rules{Play: func(c *ctx) { // "looks at 3 cards", "plays a Gold. (+$3)", "topdecks 2 cards"
		c.PlusCoins(2)
		top := c.FromTop(c.P, 3, "LOOK_AT")
		var ok []inst
		for _, x := range top {
			if c.G.Def(x).Treasure() {
				ok = append(ok, x)
			}
		}
		if x := pickOf(c, c.P, engine.ToPlay, ok, 0, 1); len(x) == 1 {
			c.PlayNow(x[0])
		}
		topdeckAny(c, c.P, c.In(revealed, top))
	}})
	gondola := nowOrLater(func(c *ctx) { c.PlusCoins(2) }, "GETS_COINS")
	gondola.Alone = true
	gondola.OnGain = func(c *ctx, _ *ev) {
		if x := pick1(c, c.P, engine.ToPlay, hand, action, true); x >= 0 {
			if c.T2().Phase == engine.StartPhase {
				c.PlayBlank(x) // at the start of a turn: "plays a Merchant. (|)"
			} else {
				c.PlayNow(x)
			}
		}
	}
	reg("Gondola", gondola)
	reg("Harbor Village", rules{ // the watch's N: 0, then the next Action and the $ before it
		Play: func(c *ctx) {
			c.Draw(1)
			c.PlusActions(2)
			c.AddWatch(engine.On(engine.EvPlaying, engine.EvPlay), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P || !c.G.D(e.C).Action() {
				return false
			}
			coins := c.Pl(w.P).Coins
			if e.Kind == engine.EvPlaying && w.N == 0 {
				w.N = (int(e.X)+1)<<16 | coins
				return false
			}
			if e.Kind != engine.EvPlay || w.N>>16 != int(e.X)+1 {
				return false
			}
			if coins > w.N&0xffff { // "gets +$1. (Harbor Village)"
				c.CoinsFrom(1)
			}
			return true
		},
	})
	reg("Landing Party", rules{
		Play:  func(c *ctx) { c.Draw(2); c.PlusActions(2); c.Stay() },
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) { // "plays a Silver. (+$2)", "topdecks a Landing Party."
			played := c.T2().Played
			if e.P != c.P || c.G.Cur != c.P || len(played) == 0 || played[0] != e.X || !c.G.D(e.C).Treasure() {
				return
			}
			for _, d := range c.Durs() { // a Reckless one has two: the second finds it gone
				d.Done = true
				c.Topdeck(c.P, inPlayOnly(c, c.Me))
			}
		},
	})
	reg("Mapmaker", rules{
		Play: func(c *ctx) { // "looks at 4 cards", "puts 2 cards into their hand", "discards 2 cards"
			top := c.FromTop(c.P, 4, "LOOK_AT")
			n := min(2, len(top))
			c.ToHand(c.P, pickOf(c, c.P, engine.ToPick, top, n, n), "PUT_IN_HAND")
			c.Discard(c.P, c.In(revealed, top))
		},
		Reacts:   engine.On(engine.EvGain),
		CanReact: func(c *ctx, e *ev) bool { return c.G.D(e.C).Victory() },
		React:    reactPlaying,
	})
	reg("Maroon", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
			n := c.G.Def(x).NTypes()
			c.Trash(c.P, one(x))
			c.Draw(2 * n)
		}
	}})
	reg("Rope", rules{Alone: true, // "gets +1 Buy.", "gets +$1."
		Play: func(c *ctx) { c.PlusBuys(1); c.PlusCoins(1); c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool { // "draws a card. (Rope)", "trashes an Estate. (Rope)"
			c.Draw(1)
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 1))
			return true
		},
	})
	reg("Swamp Shacks", rules{Play: func(c *ctx) {
		c.PlusActions(2)
		if n := len(c.Zone(c.P, inPlay)) / 3; n > 0 {
			c.Draw(n)
		}
	}})
	reg("Tools", rules{Alone: true, Play: func(c *ctx) { // "gains a Sailor."
		played := map[engine.CardID]bool{}
		for p := range 2 {
			for _, x := range c.Zone(p, inPlay) {
				played[c.G.CardOf(x)] = true
			}
		}
		c.ChooseGain(c.P, func(d *def) bool { return played[d.ID] }, discard)
	}})
	reg("Buried Treasure", rules{ // "plays 2 Buried Treasures."
		Play: func(c *ctx) { c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool { c.PlusBuys(1); c.PlusCoins(3); return true },
		OnGain: func(c *ctx, e *ev) { // "gains a Buried Treasure.", "plays a Buried Treasure."; "(|)" at the start of a turn
			if stillGained(c, e) {
				e.Done = true
				if c.T2().Phase == engine.StartPhase {
					c.PlayBlank(e.X)
				} else {
					c.PlayNow(e.X)
				}
			}
		},
	})
	reg("Crew", rules{
		Play: func(c *ctx) { c.Draw(3); c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool { // "topdecks a Crew with Crew.", "Crew will stop functioning after this turn."
			if len(c.MoveTo(c.P, inPlayOnly(c, c.Me), deck, "TOPDECK_WITH", engine.Name(c.Src))) == 1 {
				c.Log("DURATION_STOPS_WORKING", engine.Seat(c.P), engine.Name(c.Src))
			}
			return true
		},
	})
	reg("Cutthroat", rules{
		Play: func(c *ctx) {
			c.Attack(func(v int) { downTo(c, v, 3) })
			c.Stay()
		},
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // anyone's Gold: "reveals an Amphora.", "gains an Amphora. (Cutthroat)"
			if d := c.G.D(e.C); !d.Treasure() || c.CostOf(e.C).Coins < 5 {
				return
			}
			// every waiting Cutthroat goes off for this gain, not for the Loot another one gains
			for _, d := range c.G.Durs() {
				if d.C == c.Src && !d.Done && d.N == 0 {
					d.N = int(e.X) + 1
				}
			}
			for _, d := range c.Durs() {
				if d.N == int(e.X)+1 {
					d.Done = true
					gainLoot(c, c.P, discard)
				}
			}
		},
		Setup: loot,
	})
	enlarge := func(c *ctx) {
		if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
			ok := costUp(c, x, 2)
			c.Trash(c.P, one(x))
			c.ChooseGain(c.P, ok, discard)
		}
	}
	reg("Enlarge", rules{
		Play: func(c *ctx) { enlarge(c); c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool { enlarge(c); return true },
	})
	reg("Figurine", rules{Alone: true, Play: func(c *ctx) { // "draws 2 cards.", "discards an Oasis.", "gets +1 Buy."; the $1 without a line
		c.Draw(2)
		if x := pick1(c, c.P, engine.ToDiscard, hand, action, true); x >= 0 && len(c.Discard(c.P, one(x))) == 1 {
			c.PlusBuys(1)
			c.Pl(c.P).Coins++
		}
	}})
	reg("First Mate", rules{Play: func(c *ctx) {
		if x := pick1(c, c.P, engine.ToPlay, hand, action, true); x >= 0 {
			name := c.G.Def(x).Name
			c.PlayNow(x)
			for {
				y := pick1(c, c.P, engine.ToPlay, hand, named(name), true)
				if y < 0 {
					break
				}
				c.PlayNow(y)
			}
		}
		drawTo(c, 6)
	}})
	reg("Frigate", rules{ // d.N 1: a Reaction protected the other player
		Play: func(c *ctx) {
			c.PlusCoins(3)
			if d := c.Stay(); c.Unaffected(c.Other()) {
				d.N = 1
			}
		},
		Next:  func(c *ctx, d *engine.Dur) bool { return true },
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) {
			if e.P == c.P || !c.G.D(e.C).Action() {
				return
			}
			for _, d := range c.Durs() {
				if d.N == 0 {
					downTo(c.Here(), e.P, 4)
					return
				}
			}
		},
	})
	reg("Longship", rules{
		Play: func(c *ctx) { c.PlusActions(2); c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool { c.Draw(2); return true },
	})
	reg("Mining Road", rules{
		Play: func(c *ctx) {
			c.PlusActions(1)
			c.PlusBuys(1)
			c.PlusCoins(2)
			c.AddWatch(engine.On(engine.EvGain), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "gains a Hammer.", "plays a Hammer."
			d := c.G.D(e.C)
			if e.P != w.P || !d.Treasure() || !stillGained(c, e) || !c.Yes(w.P, "play it", playLine(d)) {
				return false
			}
			e.Done = true
			c.PlayNow(e.X)
			return true
		},
	})
	reg("Pendant", rules{Worth: treasureNames})
	reg("Pickaxe", rules{Coins: 1, Alone: true, Play: func(c *ctx) { // "trashes a Silver.", "reveals a Shield.", "gains a card."
		if x := pick1(c, c.P, engine.ToTrash, hand, nil, false); x >= 0 {
			n := c.CostOf(c.G.CardOf(x)).Coins
			c.Trash(c.P, one(x))
			if n >= 3 {
				gainLoot(c, c.P, hand)
			}
		}
	}, Setup: loot})
	reg("Pilgrim", rules{Play: func(c *ctx) {
		c.Draw(4)
		c.Topdeck(c.P, one(pick1(c, c.P, engine.ToTopdeck, hand, nil, false)))
	}})
	reg("Quartermaster", rules{
		Play: func(c *ctx) { c.Stay().Ever = true },
		Next: func(c *ctx, d *engine.Dur) bool { // "gains a Merchant. (Quartermaster)", or "puts a card in hand (Quartermaster)."
			ms := []engine.Mode{mode("gain a card costing up to $4", "GAIN_WITH")}
			held := c.In(aside, d.Aside)
			if len(held) > 0 {
				ms = append(ms, mode("put a card from this into your hand", "PUT_IN_HAND_FROM"))
			}
			if c.Mode1(c.P, ms...) == 1 {
				c.ToHand(c.P, pickOf(c, c.P, engine.ToPick, held, 1, 1), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			} else if x := c.ChooseGain(c.P, c.CostsUpTo(4), aside); x >= 0 && c.G.Where(x).Zone == aside {
				d.Aside = append(held, x)
			}
			return false
		},
	})
	reg("Silver Mine", rules{Alone: true, Play: func(c *ctx) { // "gains a Jewelled Egg." to hand
		c.ChooseGain(c.P, and(treasure, cheaperThan(c, c.Src)), hand)
	}})
	reg("Trickster", rules{ // the watch's N: 1 + the Treasure it set aside
		Play: func(c *ctx) {
			c.Attack(func(v int) { c.Gain(v, curse(c), discard) })
			c.AddWatch(engine.On(engine.EvDiscard, engine.EvTurnEnd), 0).Long = true
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // in Clean-up "sets a Collection aside with Trickster."
			if e.Kind == engine.EvTurnEnd {
				if w.N > 0 {
					c.At(0).ToHand(w.P, c.In(aside, one(inst(w.N-1))), "PUT_IN_HAND") // "puts a Silver into their hand."
				}
				return true
			}
			if e.P != w.P || e.From != inPlay || w.N > 0 || !c.G.D(e.C).Treasure() || c.G.Where(e.X).Zone != discard ||
				!c.Yes(w.P, "set it aside", "SET_ASIDE_WITH") {
				return false
			}
			c.SetAsideWith(w.P, e.X)
			w.N = int(e.X) + 1
			return false
		},
	})
	reg("Wealthy Village", rules{
		Play: func(c *ctx) { c.Draw(1); c.PlusActions(2) },
		OnGain: func(c *ctx, _ *ev) {
			if treasureNames(c.G, c.P) >= 3 {
				gainLoot(c, c.P, discard)
			}
		},
		Setup: loot,
	})
	reg("Sack of Loot", rules{Coins: 1, Alone: true, // "gets +1 Buy.", "reveals a Jewels.", "gains a Jewels."
		Play:  func(c *ctx) { c.PlusBuys(1); gainLoot(c, c.P, discard) },
		Setup: loot,
	})
	reg("King's Cache", rules{Alone: true, Play: func(c *ctx) { // "plays a Silver. (+$2)", "again", "a third time"
		if x := pick1(c, c.P, engine.ToPlay, hand, treasure, true); x >= 0 {
			for k := range 3 {
				c.PlayCard(x, k)
			}
		}
	}})

	// Loot
	buy3 := func(c *ctx) { c.PlusBuys(1); c.PlusCoins(3) } // "gets +1 Buy.", "gets +$3."
	amphora := nowOrLater(buy3, "GETS_BUY")
	amphora.Alone = true
	reg("Amphora", amphora)
	reg("Doubloons", rules{Coins: 3, OnGain: func(c *ctx, _ *ev) { c.Gain(c.P, c.Card("Gold"), discard) }})
	reg("Endless Chalice", rules{Coins: 1, // "plays an Endless Chalice. (+$1)", "gets +1 Buy."
		Play: func(c *ctx) { c.PlusBuys(1); c.Stay().Ever = true },
		Next: func(c *ctx, d *engine.Dur) bool { c.PlusBuys(1); c.PlusCoins(1); return false },
	})
	reg("Figurehead", rules{Alone: true,
		Play: func(c *ctx) { c.PlusCoins(3); c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool { c.Draw(2); return true },
	})
	reg("Hammer", rules{Alone: true, Play: func(c *ctx) {
		c.PlusCoins(3)
		c.ChooseGain(c.P, c.CostsUpTo(4), discard)
	}})
	reg("Insignia", rules{Coins: 3,
		Play: func(c *ctx) { c.AddWatch(engine.On(engine.EvGain), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "gains an Amphora.", "topdecks an Amphora with Insignia."
			if e.P == w.P && stillGained(c, e) && c.Yes(w.P, "put it onto your deck", "TOPDECK_WITH") {
				e.Done = true
				c.MoveTo(w.P, one(e.X), deck, "TOPDECK_WITH", engine.Name(c.Src))
			}
			return false
		},
	})
	reg("Jewels", rules{Alone: true,
		Play: func(c *ctx) { buy3(c); c.Stay() },
		Next: func(c *ctx, d *engine.Dur) bool { // "puts a Jewels on the bottom of Draw Pile."
			c.BottomdeckAs(c.P, inPlayOnly(c, c.Me), "PUT_ON_BOTTOM_OF", "Draw Pile")
			return true
		},
	})
	reg("Orb", rules{Alone: true, Play: func(c *ctx) { // "looks at" the discard pile, then "plays a Highway." or "+1 Buy", "+$3"
		c.LookAt(c.P, c.Zone(c.P, discard))
		if x := pick1(c, c.P, engine.ToPlay, discard, func(d *def) bool { return d.Action() || d.Treasure() }, true); x >= 0 {
			c.PlayNow(x)
			return
		}
		buy3(c)
	}})
	reg("Prize Goat", rules{Alone: true, Play: func(c *ctx) {
		buy3(c)
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 0, 1))
	}})
	reg("Puzzle Box", rules{Alone: true,
		Play: func(c *ctx) { // "sets a Gondola aside with Puzzle Box."; after the Clean-up draw "puts a Gondola in hand (Puzzle Box)."
			buy3(c)
			if x := pick1(c, c.P, engine.ToSetAside, hand, nil, true); x >= 0 {
				c.SetAsideWith(c.P, x)
				c.AddWatch(engine.On(engine.EvTurnEnd), int(x)).Long = true
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			c.At(0).ToHand(w.P, c.In(aside, one(inst(w.N))), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			return true
		},
	})
	reg("Sextant", rules{Alone: true, Play: func(c *ctx) {
		buy3(c)
		top := c.FromTop(c.P, 5, "LOOK_AT")
		c.Discard(c.P, pickOf(c, c.P, engine.ToDiscard, top, 0, len(top)))
		topdeckAny(c, c.P, c.In(revealed, top))
	}})
	reg("Shield", rules{ // "plays a Buried Treasure and a Shield.", "gets +1 Buy.", "gets +$3."
		Play:     buy3,
		Reacts:   engine.On(engine.EvAttack),
		CanReact: attacked,
		React:    moat,
	})
	reg("Spell Scroll", rules{Play: func(c *ctx) { // "trashes a Spell Scroll.", "gains a Litter.", "plays a Litter."
		if len(c.Trash(c.P, one(c.Me))) == 0 {
			c.Log("SPELL_SCROLL_FAILS")
			return
		}
		x := c.ChooseGain(c.P, cheaperThan(c, c.Src), discard)
		if x < 0 || c.G.Where(x).Zone != discard {
			return
		}
		if d := c.G.Def(x); (d.Action() || d.Treasure()) && c.Yes(c.P, "play it", playLine(d)) {
			c.PlayNow(x)
		}
	}})
	reg("Staff", rules{Alone: true, Play: func(c *ctx) {
		buy3(c)
		if x := pick1(c, c.P, engine.ToPlay, hand, action, true); x >= 0 {
			c.PlayNow(x)
		}
	}})
	reg("Sword", rules{Alone: true, Play: func(c *ctx) {
		buy3(c)
		c.Attack(func(v int) { downTo(c, v, 4) })
	}})

	// Events
	reg("Bury", rules{Buy: func(c *ctx) { // "looks at a Village Green.", "puts a Village Green on the bottom of Draw Pile."
		c.PlusBuys(1)
		c.LookAt(c.P, c.Zone(c.P, discard))
		c.BottomdeckAs(c.P, one(pick1(c, c.P, engine.ToPick, discard, nil, false)), "PUT_ON_BOTTOM_OF", "Draw Pile")
	}})
	reg("Avoid", rules{ // the watch's N: how many cards it keeps out of the shuffle
		Buy: func(c *ctx) {
			c.PlusBuys(1)
			for i, w := range c.G.Watches() {
				if w.C == c.Src && w.P == c.P {
					c.G.Watches()[i].N += 3
					return
				}
			}
			c.AddWatch(engine.On(engine.EvShuffling, engine.EvTurnEnd), 3).Long = true
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool { // "puts 4 Coppers, an Estate, and a Province into their discard pile.", then the shuffle
			if e.Kind == engine.EvTurnEnd {
				return true
			}
			if e.P != w.P {
				return false
			}
			d := c.Deeper()
			xs := pickOf(d, w.P, engine.ToPick, c.Zone(w.P, discard), 0, w.N)
			if len(xs) > 0 {
				d.Log("PUT_IN_DISCARD", engine.Seat(w.P), kindsOf(c, xs))
			}
			e.Keep = append(e.Keep, xs...)
			return true
		},
	})
	reg("Deliver", rules{
		Buy: func(c *ctx) {
			c.PlusBuys(1)
			c.Later()
			c.AddWatch(engine.On(engine.EvGain, engine.EvTurnEnd), 0).Long = true
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			for _, d := range c.G.Durs() {
				if d.C != c.Src || d.P != w.P || d.Done {
					continue
				}
				if e.Kind == engine.EvTurnEnd {
					d.Done = true
					c.At(0).ToHand(w.P, c.In(aside, d.Aside), "PUT_IN_HAND_FROM", engine.Name(c.Src))
					return true
				}
				if e.P == w.P && stillGained(c, e) {
					e.Done = true
					d.Aside = append(d.Aside, c.SetAside(w.P, one(e.X))...)
				}
				return false
			}
			return true
		},
	})
	reg("Peril", rules{Buy: func(c *ctx) { // "trashes a Guard Dog.", "reveals a Shield.", "gains a Shield."
		if len(c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, action, true)))) == 0 {
			c.Log("PERIL_FAIL")
			return
		}
		gainLoot(c, c.P, discard)
	}, Setup: loot})
	reg("Rush", rules{
		Buy: func(c *ctx) { c.PlusBuys(1); c.AddWatch(engine.On(engine.EvGain), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P || !c.G.D(e.C).Action() || !stillGained(c, e) {
				return false
			}
			e.Done = true
			c.PlayWith(e.X)
			return true
		},
	})
	reg("Foray", rules{Buy: func(c *ctx) { // "discards and reveals a Copper, a Silver, and an Estate.", then a Loot
		n := min(3, len(c.Hand(c.P)))
		xs := c.DiscardAs(c.P, pick(c, c.P, engine.ToDiscard, hand, nil, n, n), "DISCARD_REVEALED")
		if len(xs) == 3 && distinct(c, xs) == 3 {
			gainLoot(c, c.P, discard)
		}
	}, Setup: loot})
	reg("Launch", rules{
		CanBuy: once("Launch"),
		Buy: func(c *ctx) { // "draws a card.", "gets +1 Action.", "gets +1 Buy.", back to the Action phase
			c.T2().Used[c.Src]++
			c.Draw(1)
			c.PlusActions(1)
			c.PlusBuys(1)
			c.T2().Back = true
		},
	})
	reg("Mirror", rules{ // the watch's N: 1 once it gained the copy
		Buy: func(c *ctx) { c.PlusBuys(1); c.AddWatch(engine.On(engine.EvGain), 0) },
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P || w.N > 0 || !c.G.D(e.C).Action() || e.By == c.Src { // two Mirrors copy the bought card, not each other's copy
				return false
			}
			w.N = 1
			c.Gain(w.P, e.C, discard)
			return true
		},
	})
	reg("Prepare", rules{
		Buy: func(c *ctx) { // "sets a Copper and a River Shrine aside with Prepare."; next turn it plays the Actions and Treasures, then discards the rest
			xs := c.SetAsideWith(c.P, append([]inst{}, c.Hand(c.P)...)...)
			c.Later().Aside = xs
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			played := map[inst]bool{} // once each: Way of the Turtle sets one aside again
			for {
				var ok []inst
				for _, x := range c.In(aside, d.Aside) {
					if dd := c.G.Def(x); !played[x] && (dd.Action() || dd.Treasure()) {
						ok = append(ok, x)
					}
				}
				x := pickOf(c, c.P, engine.ToPlay, ok, 1, 1)
				if len(x) == 0 {
					break
				}
				played[x[0]] = true
				if dd := c.G.Def(x[0]); dd.Treasure() && !dd.Action() { // "plays a Copper. (+$1)"; "plays a River Shrine. (|)"
					c.PlayNow(x[0])
				} else {
					c.PlayBlank(x[0])
				}
			}
			c.Discard(c.P, slices.DeleteFunc(c.In(aside, d.Aside), func(x inst) bool { return played[x] }))
			return true
		},
	})
	reg("Scrounge", rules{Buy: func(c *ctx) { // "gains an Estate.", "gains a Frigate."
		estates := inTrash(c, named("Estate"))
		ms := []engine.Mode{mode("trash a card from your hand", "TRASH")}
		if len(estates) > 0 {
			ms = append(ms, mode("gain an Estate from the trash", "GAIN"))
		}
		if c.Mode1(c.P, ms...) == 0 {
			c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, 1, 1))
			return
		}
		c.GainInst(c.P, estates[0], discard, "GAIN")
		c.ChooseGain(c.P, c.CostsUpTo(5), discard)
	}})
	reg("Journey", rules{Buy: func(c *ctx) { // the extra turn: "Turn 1 - gamrevinu [Journey]"; not a third turn in a row
		c.T2().KeepPlay = true
		if c.T2().Extra == 0 && c.T2().Used[c.Src] == 0 {
			c.ExtraTurn()
		}
		c.T2().Used[c.Src]++
	}})
	reg("Maelstrom", rules{Buy: func(c *ctx) { // "trashes 3 Estates.", the other player "trashes an Estate."
		n := min(3, len(c.Hand(c.P)))
		c.Trash(c.P, pick(c, c.P, engine.ToTrash, hand, nil, n, n))
		if v := c.Other(); len(c.Hand(v)) >= 5 {
			c.Trash(v, pick(c, v, engine.ToTrash, hand, nil, 1, 1))
		}
	}})
	reg("Looting", rules{Buy: func(c *ctx) { gainLoot(c, c.P, discard) }, Setup: loot})
	reg("Invasion", rules{Buy: func(c *ctx) { // "gains a Duchy.", "gains an Apprentice.", the Loot, "plays a Shield."
		if x := pick1(c, c.P, engine.ToPlay, hand, attack, true); x >= 0 {
			c.PlayNow(x)
		}
		c.Gain(c.P, c.Card("Duchy"), discard)
		c.ChooseGain(c.P, action, deck)
		if x := gainLoot(c, c.P, discard); x >= 0 && c.G.Where(x) == (engine.Loc{Zone: discard, Owner: int16(c.P)}) {
			c.PlayNow(x)
		}
	}, Setup: loot})
	reg("Prosper", rules{Buy: func(c *ctx) { // the Loot, "gains a Gold.", "gains a Silver."
		gainLoot(c, c.P, discard)
		got := map[engine.CardID]bool{}
		for {
			o := c.ChooseOne(c.P, engine.ToGain, c.SupplyOptions(func(d *def) bool { return d.Treasure() && !got[d.ID] }), true)
			if o == nil {
				return
			}
			got[o.Card] = true
			c.Gain(c.P, o.Card, discard)
		}
	}, Setup: loot})

	// Traits
	reg("Cheap", rules{Reduce: func(g *engine.Game, _ inst, c engine.CardID) int {
		if g.D(g.TraitOf(c)).Name == "Cheap" {
			return 1
		}
		return 0
	}})
	reg("Cursed", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if traited(c)(c.G.D(e.C)) {
				gainLoot(c, e.P, discard)
				c.Gain(e.P, curse(c), discard)
			}
		},
		Setup: loot,
	})
	reg("Fated", rules{
		Hears: engine.On(engine.EvShuffling),
		Hear: func(c *ctx, e *ev) { // "shuffles a Pasture, a Replace, and 4 Coppers.", "topdecks a Replace.", then the shuffle; not at the start
			if c.G.P[0].Turns == 0 {
				return
			}
			d := c.Deeper()
			xs := c.Zone(e.P, discard)
			d.Log("SHUFFLES_CARDS", engine.Seat(e.P), kindsOf(c, xs))
			var fated []inst
			for _, x := range xs {
				if traited(c)(c.G.Def(x)) {
					fated = append(fated, x)
				}
			}
			d.Topdeck(e.P, pickOf(d, e.P, engine.ToTopdeck, fated, 0, len(fated)))
		},
	})
	reg("Fawning", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if c.G.D(e.C).Name == "Province" {
				if top := traitTop(c); top != 0 {
					c.Gain(e.P, top, discard)
				}
			}
		},
	})
	reg("Friendly", rules{
		Hears: engine.On(engine.EvTurnStart, engine.EvCleanup),
		Hear: func(c *ctx, e *ev) {
			if e.Kind == engine.EvTurnStart { // dominion.games logs "starts their turn." every turn with Friendly, as with Patient
				phaseLine(c)
				return
			}
			if x := pick1(c, e.P, engine.ToDiscard, hand, traited(c), true); x >= 0 && len(c.Discard(e.P, one(x))) == 1 {
				if top := traitTop(c); top != 0 {
					c.Gain(e.P, top, discard)
				}
			}
		},
	})
	reg("Hasty", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "sets a Groundskeeper aside."; next turn "plays a Groundskeeper. (Hasty)"
			if traited(c)(c.G.D(e.C)) && stillGained(c, e) {
				e.Done = true
				c.Later().Aside = c.SetAside(e.P, one(e.X))
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			for _, x := range c.In(aside, d.Aside) {
				c.PlayWith(x)
			}
			return true
		},
	})
	reg("Inherited", rules{}) // the engine swaps the starting card
	reg("Inspiring", rules{
		Hears: engine.On(engine.EvPlay),
		Hear: func(c *ctx, e *ev) {
			if e.P != c.G.Cur || !traited(c)(c.G.D(e.C)) {
				return
			}
			have := map[engine.CardID]bool{}
			for _, x := range c.Zone(e.P, inPlay) {
				have[c.G.CardOf(x)] = true
			}
			if x := pick1(c, e.P, engine.ToPlay, hand, func(d *def) bool { return d.Action() && !have[d.ID] }, true); x >= 0 {
				c.PlayNow(x)
			}
		},
	})
	reg("Nearby", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "gets +1 Buy. (Nearby)"
			if traited(c)(c.G.D(e.C)) {
				c.Away().PlusBuys(1)
			}
		},
	})
	reg("Patient", rules{
		Hears: engine.On(engine.EvTurnStart, engine.EvCleanup),
		Hear: func(c *ctx, e *ev) { // "sets a Nobles aside."; next turn "plays a Nobles. (|)"
			if e.Kind == engine.EvTurnStart {
				phaseLine(c)
				return
			}
			if xs := c.SetAside(e.P, pick(c, e.P, engine.ToSetAside, hand, traited(c), 0, len(c.Hand(e.P)))); len(xs) > 0 {
				c.Later().Aside = xs
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			for _, x := range c.In(aside, d.Aside) {
				c.PlayBlank(x)
			}
			return true
		},
	})
	reg("Pious", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "trashes an Estate."; "(Pious)" at the start of a turn
			if traited(c)(c.G.D(e.C)) {
				a := c.Away()
				if c.T2().Phase != engine.StartPhase {
					a = c.Here()
				}
				a.Trash(e.P, pick(a, e.P, engine.ToTrash, hand, nil, 0, 1))
			}
		},
	})
	reg("Reckless", rules{ // the engine follows its instructions twice
		Hears: engine.On(engine.EvDiscard),
		Hear: func(c *ctx, e *ev) { // "returns a Sacrifice to the Sacrifice pile."
			if e.From == inPlay && traited(c)(c.G.D(e.C)) {
				c.ReturnTo(e.P, e.X)
			}
		},
	})
	reg("Rich", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if traited(c)(c.G.D(e.C)) {
				c.Gain(e.P, c.Card("Silver"), discard)
			}
		},
	})
	reg("Shy", rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) { // "discards a Patron.", "draws 2 cards. (Shy)"
			phaseLine(c)
			if x := pick1(c, e.P, engine.ToDiscard, hand, traited(c), true); x >= 0 && len(c.Discard(e.P, one(x))) == 1 {
				c.Away().Draw(2)
			}
		},
	})
	reg("Tireless", rules{
		Hears: engine.On(engine.EvDiscard),
		Hear: func(c *ctx, e *ev) { // "sets a Crown aside."; after the Clean-up draw "topdecks a Crown."
			if e.From == inPlay && traited(c)(c.G.D(e.C)) && len(c.SetAside(e.P, one(e.X))) == 1 {
				c.AddWatch(engine.On(engine.EvTurnEnd), int(e.X)).Long = true
			}
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			c.At(0).Topdeck(w.P, c.In(aside, one(inst(w.N))))
			return true
		},
	})
}
