package cards

import (
	"slices"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// Renaissance: Villagers, Coffers, the Artifacts and the Projects. Simplifications:
// Capitalism makes the Actions it names Treasures only for playing them in the
// Buy phase; Improve offers every Action in play that isn't staying for a Duration.

// take gives player p an Artifact or a State: "takes the Flag."; nothing when they have it
// already. An Artifact and Lost in the Woods move from whoever has them; other States have a copy each.
func take(c *ctx, p int, name string) {
	a := c.Card(name)
	single := c.G.D(a).Is(engine.TArtifact) || name == "Lost in the Woods"
	for i, h := range c.G.Held {
		if h.C == a && h.P == p {
			return
		}
		if h.C == a && single {
			c.G.Held = slices.Delete(c.G.Held, i, i+1)
			break
		}
	}
	c.G.Held = append(c.G.Held, engine.Held{C: a, P: p})
	c.Log("TAKE", engine.Seat(p), []engine.CardID{a})
}

// holds: player p has the Artifact.
func holds(g *engine.Game, p int, name string) bool {
	for _, h := range g.Held {
		if h.P == p && g.D(h.C).Name == name {
			return true
		}
	}
	return false
}

// project: player p has a cube on this Project, whose Hear runs for every event.
func project(c *ctx, p int) bool { return c.G.HasProject(p, c.Src) }

// hasProject: player p has a cube on the Project, by name, for rules that don't know their own card.
func hasProject(g *engine.Game, p int, name string) bool {
	return slices.ContainsFunc(g.P[p].Projects, func(x engine.CardID) bool { return g.D(x).Name == name })
}

func nameSet(ns ...string) map[string]bool {
	m := make(map[string]bool, len(ns))
	for _, n := range ns {
		m[n] = true
	}
	return m
}

// atStart is a Project's effect at the start of its owner's turns.
func atStart(f func(c *ctx, p int)) rules {
	return rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) {
			if project(c, e.P) {
				f(c.Away(), e.P)
			}
		},
	}
}

// leaving is what of player p's in play the Clean-up would discard: not a Duration staying, nor what holds it.
func leaving(c *ctx, p int) []inst {
	stay := map[inst]bool{}
	for _, d := range c.G.Durs() {
		if d.P == p && !d.Done && d.X >= 0 {
			stay[d.X] = true
			for _, h := range d.Hold {
				stay[h] = true
			}
		}
	}
	var out []inst
	for _, x := range c.Zone(p, inPlay) {
		if !stay[x] {
			out = append(out, x)
		}
	}
	return out
}

// capitalism are the Actions with +$ in their text that Capitalism makes Treasures.
var capitalism = nameSet(
	// Base, Intrigue
	"Festival", "Market", "Merchant", "Militia", "Moneylender", "Poacher", "Vassal",
	"Baron", "Bridge", "Conspirator", "Courtier", "Ironworks", "Mill", "Mining Village", "Minion", "Pawn", "Steward", "Swindler",
	// Seaside, Prosperity
	"Astrolabe", "Bazaar", "Corsair", "Cutpurse", "Fishing Village", "Lighthouse", "Merchant Ship", "Monkey", "Pirate", "Salvager", "Sea Witch", "Tide Pools",
	"Bishop", "City", "Clerk", "Grand Market", "Monument", "Rabble", "Worker's Village",
	// Cornucopia & Guilds, Hinterlands
	"Baker", "Butcher", "Candlestick Maker", "Carnival", "Farrier", "Ferryman", "Hamlet", "Herald", "Hunting Party", "Merchant Guild", "Plaza", "Remake", "Shop",
	"Berserker", "Cartographer", "Crossroads", "Guard Dog", "Highway", "Inn", "Margrave", "Nomads", "Oasis", "Spice Merchant", "Souk", "Trader", "Weaver", "Wheelwright", "Witch's Hut",
	// Dark Ages, Adventures
	"Bandit Camp", "Count", "Hermit", "Ironmonger", "Junk Dealer", "Madman", "Marauder", "Pillage", "Poor House", "Rogue", "Squire", "Storeroom", "Urchin", "Vagrant",
	"Amulet", "Artificer", "Caravan Guard", "Dungeon", "Gear", "Guide", "Haunted Woods", "Lost City", "Magpie", "Messenger", "Miser", "Port", "Ranger", "Raze", "Swamp Hag", "Treasure Hunter", "Warrior", "Hero",
	// Empires, Renaissance
	"Archive", "Capital", "Chariot Race", "City Quarter", "Emporium", "Enchantress", "Engineer", "Forum", "Gladiator", "Legionary", "Royal Blacksmith", "Sacrifice", "Settlers", "Temple", "Villa", "Wild Hunt",
	"Cargo Ship", "Flag Bearer", "Improve", "Patron", "Priest", "Treasurer", "Swashbuckler",
	// Nocturne
	"Bard", "Conclave", "Sacred Grove", "Secret Cave", "Tormentor", "Tracker",
	// Menagerie, Plunder
	"Animal Fair", "Barge", "Bounty Hunter", "Camel Train", "Cavalry", "Coven", "Falconer", "Fisherman", "Goatherd", "Hunting Lodge", "Livery", "Paddock", "Sanctuary", "Scrap", "Wayfarer",
	"Buried Treasure", "Cabin Boy", "Crucible", "Flagship", "Fortune Hunter", "Gondola", "Harbor Village", "King's Cache", "Longship", "Mapmaker", "Maroon", "Mining Road", "Pickaxe", "Pilgrim", "Quartermaster", "Rope", "Sack of Loot", "Silver Mine", "Swamp Shacks", "Tools", "Trickster", "Wealthy Village",
)

func init() {
	// the Artifacts
	reg("Flag", rules{
		Hears: engine.On(engine.EvCleanup),
		Hear: func(c *ctx, e *ev) { // silent: "draws 6 cards."
			if e.P == c.P {
				c.T2().HandSize++
			}
		},
	})
	reg("Horn", rules{
		Hears: engine.On(engine.EvDiscard),
		Hear: func(c *ctx, e *ev) { // "begins their Cleanup phase.", "topdecks a Border Guard."
			if e.P == c.P && e.Cleanup && c.G.D(e.C).Name == "Border Guard" && c.T2().Used[c.Src] == 0 && c.Yes(c.P, "put it onto your deck", "TOPDECK") {
				c.T2().Used[c.Src]++
				c.Topdeck(c.P, one(e.X))
			}
		},
	})
	reg("Key", rules{
		Hears: engine.On(engine.EvTurnStart),
		Hear: func(c *ctx, e *ev) {
			if e.P == c.P {
				c.Away().PlusCoins(1) // "gets +$1. (Key)"
			}
		},
	})
	reg("Lantern", rules{}) // Border Guard reads it
	reg("Treasure Chest", rules{
		Hears: engine.On(engine.EvBuyStart),
		Hear: func(c *ctx, e *ev) {
			if e.P == c.P {
				c.Away().Gain(c.P, c.Card("Gold"), discard) // "gains a Gold. (Treasure Chest)"
			}
		},
	})

	// Actions and Treasures
	reg("Acting Troupe", rules{Play: func(c *ctx) {
		c.PlusVillagers(4)
		c.Trash(c.P, inPlayOnly(c, c.Me))
	}})
	reg("Border Guard", rules{Play: func(c *ctx) { // "reveals 2 Border Guards.", "puts a Border Guard into their hand.", "discards a Border Guard.", "takes the Lantern."
		c.PlusActions(1)
		n, lantern := 2, holds(c.G, c.P, "Lantern")
		if lantern {
			n = 3
		}
		xs := c.FromTop(c.P, n, "REVEAL")
		if len(xs) == 0 {
			return
		}
		c.ToHand(c.P, pickOf(c, c.P, engine.ToPick, xs, 1, 1), "PUT_IN_HAND")
		c.Discard(c.P, c.In(revealed, xs))
		if len(xs) < n || slices.ContainsFunc(xs, func(x inst) bool { return !c.G.Def(x).Action() }) {
			return
		}
		if lantern || c.Mode1(c.P, mode("take the Horn", "TAKE"), mode("take the Lantern", "TAKE")) == 0 {
			take(c, c.P, "Horn")
		} else {
			take(c, c.P, "Lantern")
		}
	}})
	reg("Cargo Ship", rules{
		Play: func(c *ctx) {
			c.PlusCoins(2)
			c.Stay()
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.ToHand(c.P, c.In(aside, d.Aside), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			return true
		},
		Hears: engine.On(engine.EvGain, engine.EvCleanup),
		Hear: func(c *ctx, e *ev) { // "sets a Replace aside with Cargo Ship."; next turn "puts a Replace in hand (Cargo Ship)."
			if e.P != c.P {
				return
			}
			for _, d := range c.Durs() {
				switch {
				case d.Done || d.N > 0:
				case e.Kind == engine.EvCleanup:
					d.Done = true // it set nothing aside this turn
				case stillGained(c, e) && c.Yes(c.P, "set it aside", "SET_ASIDE_WITH"):
					e.Done = true
					d.N = 1
					d.Aside = c.SetAsideWith(c.P, e.X)
				}
			}
		},
	})
	reg("Ducat", rules{
		Play: func(c *ctx) { c.PlusCoffers(1); c.PlusBuys(1) },
		OnGain: func(c *ctx, e *ev) {
			c.Trash(e.P, one(pick1(c, e.P, engine.ToTrash, hand, named("Copper"), true)))
		},
	})
	reg("Experiment", rules{
		Play: func(c *ctx) { // "returns an Experiment to the Experiment pile."
			c.Draw(2)
			c.PlusActions(1)
			c.ReturnTo(c.P, c.Me)
		},
		OnGain: func(c *ctx, e *ev) {
			if e.By != c.Src { // not the one an Experiment came with
				c.Gain(e.P, c.Src, discard)
			}
		},
	})
	reg("Flag Bearer", rules{
		Play:    func(c *ctx) { c.PlusCoins(2) },
		OnGain:  func(c *ctx, e *ev) { take(c, e.P, "Flag") },
		OnTrash: func(c *ctx, e *ev) { take(c, e.P, "Flag") },
	})
	reg("Hideout", rules{Play: func(c *ctx) { // "trashes an Estate.", "gains a Curse."
		c.Draw(1)
		c.PlusActions(2)
		got := c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
		if len(got) == 1 && c.G.Def(got[0]).Victory() {
			c.Gain(c.P, c.Card("Curse"), discard)
		}
	}})
	reg("Improve", rules{
		Play: func(c *ctx) {
			c.PlusCoins(2)
			c.AddWatch(engine.On(engine.EvCleanup), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P != w.P {
				return false
			}
			xs := slices.DeleteFunc(leaving(c, w.P), func(x inst) bool { return !c.G.Def(x).Action() })
			if len(xs) == 0 {
				return true
			}
			if got := pickOf(c, w.P, engine.ToTrash, xs, 0, 1); len(got) == 1 {
				ok := costExactly(c, got[0], 1)
				if len(c.Trash(w.P, got)) == 1 {
					c.ChooseGain(w.P, ok, discard)
				}
			}
			return true
		},
	})
	reg("Inventor", rules{Play: func(c *ctx) {
		c.ChooseGain(c.P, c.CostsUpTo(4), discard)
		c.T2().Reduce++
	}})
	reg("Lackeys", rules{
		Play:   func(c *ctx) { c.Draw(2) },
		OnGain: func(c *ctx, e *ev) { c.As(e.P).PlusVillagers(2) },
	})
	reg("Mountain Village", rules{Play: func(c *ctx) { // "looks at 2 Coppers and a Silver.", "puts a Silver into their hand."; with none "draws a card."
		c.PlusActions(2)
		ds := slices.Clone(c.Zone(c.P, discard))
		if len(ds) == 0 {
			c.Draw(1)
			return
		}
		c.LookAt(c.P, ds)
		c.ToHand(c.P, pickOf(c, c.P, engine.ToPick, ds, 1, 1), "PUT_IN_HAND")
	}})
	reg("Old Witch", rules{Play: func(c *ctx) {
		c.Draw(3)
		c.Attack(func(v int) {
			c.Gain(v, c.Card("Curse"), discard)
			c.Trash(v, one(pick1(c, v, engine.ToTrash, hand, named("Curse"), true)))
		})
	}})
	reg("Patron", rules{
		Play: func(c *ctx) { c.PlusVillagers(1); c.PlusCoins(2) },
		OnReveal: func(c *ctx, e *ev) { // "reveals a Patron.", "gets a Coffers from Patron."
			c.Pl(e.P).Coffers++
			c.Log("COFFERS_FROM_PATRON", engine.Seat(e.P))
		},
	})
	reg("Priest", rules{
		Play: func(c *ctx) {
			c.PlusCoins(2)
			c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
			c.AddWatch(engine.On(engine.EvTrash), 0)
		},
		Watch: func(c *ctx, e *ev, w *engine.Watch) bool {
			if e.P == w.P && e.P == c.G.Cur {
				c.CoinsFromMerged(2) // "trashes 2 Coppers.", "gets +$4. (Priest)"
			}
			return false
		},
	})
	reg("Recruiter", rules{Play: func(c *ctx) { // "trashes an Estate.", "gets +2 Villagers."
		c.Draw(2)
		got := c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
		if len(got) == 1 {
			if n := c.CostOf(c.G.CardOf(got[0])).Coins; n > 0 {
				c.PlusVillagers(n)
			}
		}
	}})
	reg("Research", rules{
		Play: func(c *ctx) { // "trashes an Estate.", "sets a Copper and an Estate aside."; next turn "puts ... in hand (Research)."
			c.PlusActions(1)
			got := c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, nil, false)))
			if len(got) == 0 {
				return
			}
			if n := c.CostOf(c.G.CardOf(got[0])).Coins; n > 0 {
				if xs := c.SetAside(c.P, c.FromTop(c.P, n, "")); len(xs) > 0 {
					c.Stay().Aside = xs
				}
			}
		},
		Next: func(c *ctx, d *engine.Dur) bool {
			c.ToHand(c.P, c.In(aside, d.Aside), "PUT_IN_HAND_FROM", engine.Name(c.Src))
			return true
		},
	})
	reg("Scepter", rules{
		Alone: true,
		Play: func(c *ctx) { // "plays a Scepter.", "plays a Livery again."
			var xs []inst
			for _, x := range c.T2().Played {
				if c.G.Def(x).Action() && c.G.Where(x) == (engine.Loc{Zone: inPlay, Owner: int16(c.P)}) && !slices.Contains(xs, x) {
					xs = append(xs, x)
				}
			}
			if len(xs) == 0 || c.Mode1(c.P, mode("+$2", "GETS_COINS"), mode("replay an Action", "PLAY_AGAIN")) == 0 {
				c.PlusCoins(2)
				return
			}
			x := pickOf(c, c.P, engine.ToPlay, xs, 1, 1)[0]
			c.PlayCard(x, 1)
			for _, d := range c.G.Durs() { // it stays as long as the Duration, whichever play keeps that
				if d.X == x && !d.Done && !slices.Contains(d.Hold, c.Me) {
					d.Hold = append(d.Hold, c.Me)
				}
			}
		},
	})
	reg("Scholar", rules{Play: func(c *ctx) {
		c.Discard(c.P, slices.Clone(c.Hand(c.P)))
		c.Draw(7)
	}})
	reg("Sculptor", rules{Play: func(c *ctx) { // "gains a Potion.", "gets +1 Villager."
		if x := c.ChooseGain(c.P, c.CostsUpTo(4), hand); x >= 0 && c.G.Def(x).Treasure() {
			c.PlusVillagers(1)
		}
	}})
	reg("Seer", rules{Play: func(c *ctx) { // "reveals a Copper, a Fortress, and a Dungeon.", "puts a Fortress and a Dungeon into their hand.", "topdecks a Copper."
		c.Draw(1)
		c.PlusActions(1)
		xs := c.FromTop(c.P, 3, "REVEAL")
		var keep []inst
		for _, x := range xs {
			if m := c.CostOf(c.G.CardOf(x)); m.Coins >= 2 && m.Coins <= 4 && m.Potion == 0 && m.Debt == 0 {
				keep = append(keep, x)
			}
		}
		c.ToHand(c.P, keep, "PUT_IN_HAND")
		topdeckAny(c, c.P, c.In(revealed, xs))
	}})
	reg("Silk Merchant", rules{
		Play:    func(c *ctx) { c.Draw(2); c.PlusBuys(1) },
		OnGain:  func(c *ctx, e *ev) { c.As(e.P).PlusCoffers(1); c.As(e.P).PlusVillagers(1) },
		OnTrash: func(c *ctx, e *ev) { c.As(e.P).PlusCoffers(1); c.As(e.P).PlusVillagers(1) },
	})
	reg("Spices", rules{
		Coins:  2,
		Play:   func(c *ctx) { c.PlusBuys(1) },
		OnGain: func(c *ctx, e *ev) { c.As(e.P).PlusCoffers(2) },
	})
	reg("Swashbuckler", rules{Play: func(c *ctx) { // "gets +1 Coffers.", "takes the Treasure Chest."
		c.Draw(3)
		if len(c.Zone(c.P, discard)) > 0 {
			c.PlusCoffers(1)
			if c.Pl(c.P).Coffers >= 4 {
				take(c, c.P, "Treasure Chest")
			}
		}
	}})
	reg("Treasurer", rules{Play: func(c *ctx) {
		c.PlusCoins(3)
		switch c.Mode1(c.P, mode("trash a Treasure", "TRASH"), mode("gain a Treasure from the trash", "GAIN"), mode("take the Key", "TAKE")) {
		case 0:
			c.Trash(c.P, one(pick1(c, c.P, engine.ToTrash, hand, treasure, false)))
		case 1:
			var opts []engine.Option
			for _, x := range c.G.Trashed {
				if c.G.Def(x).Treasure() {
					opts = append(opts, engine.Option{Card: c.G.CardOf(x), Inst: x})
				}
			}
			if o := c.ChooseOne(c.P, engine.ToGain, opts, false); o != nil {
				c.GainInst(c.P, o.Inst, hand, "GAIN")
			}
		case 2:
			take(c, c.P, "Key")
		}
	}})
	reg("Villain", rules{Play: func(c *ctx) {
		c.PlusCoffers(2)
		c.Attack(func(v int) {
			if len(c.Hand(v)) < 5 {
				return
			}
			if x := pick1(c, v, engine.ToDiscard, hand, func(d *def) bool { return c.CostOf(d.ID).Coins >= 2 }, false); x >= 0 {
				c.Discard(v, one(x))
			} else {
				c.RevealHand(v)
			}
		})
	}})

	// the Projects
	reg("Academy", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if project(c, e.P) && c.G.D(e.C).Action() {
				c.Away().PlusVillagers(1) // "gets +1 Villager. (Academy)"
			}
		},
	})
	reg("Barracks", atStart(func(c *ctx, p int) { c.PlusActions(1) }))
	reg("Canal", rules{Reduce: func(g *engine.Game, _ inst, _ engine.CardID) int {
		if hasProject(g, g.Cur, "Canal") {
			return 1
		}
		return 0
	}})
	reg("Capitalism", rules{AlsoTreasure: func(g *engine.Game, p int, d *def) bool {
		return capitalism[d.Name] && hasProject(g, p, "Capitalism")
	}})
	reg("Cathedral", atStart(func(c *ctx, p int) { // "trashes an Estate. (Cathedral)"
		c.Trash(p, one(pick1(c, p, engine.ToTrash, hand, nil, false)))
	}))
	reg("Citadel", rules{
		Hears: engine.On(engine.EvPlaying, engine.EvPlay),
		Hear: func(c *ctx, e *ev) { // the first Action again, on its own line: "plays a Settlers again."
			if e.P != c.G.Cur || !project(c, e.P) || !c.G.D(e.C).Action() {
				return
			}
			u := c.T2().Used
			switch {
			case e.Kind == engine.EvPlaying && u[c.Src] == 0:
				u[c.Src] = int(e.X) + 1
			case e.Kind == engine.EvPlay && u[c.Src] == int(e.X)+1:
				u[c.Src] = -1
				c.At(e.Level - 1).PlayAgain(e.X)
			}
		},
	})
	reg("City Gate", atStart(func(c *ctx, p int) { // "draws a card. (City Gate)", "topdecks a Copper with City Gate."
		c.Draw(1)
		c.MoveTo(p, one(pick1(c, p, engine.ToTopdeck, hand, nil, false)), deck, "TOPDECK_WITH", engine.Name(c.Src))
	}))
	reg("Crop Rotation", atStart(func(c *ctx, p int) { // "discards an Estate. (Crop Rotation)", "draws 2 cards. (Crop Rotation)"
		if len(c.Discard(p, one(pick1(c, p, engine.ToDiscard, hand, victory, true)))) == 1 {
			c.Draw(2)
		}
	}))
	reg("Exploration", rules{
		Hears: engine.On(engine.EvBuyEnd),
		Hear: func(c *ctx, e *ev) { // "gets +1 Coffers. (Exploration)", "gets +1 Villager. (Exploration)"
			if project(c, e.P) && !slices.ContainsFunc(c.T2().Bought, func(b engine.CardID) bool { return !c.G.D(b).Landscape() }) {
				c.Away().PlusCoffers(1)
				c.Away().PlusVillagers(1)
			}
		},
	})
	reg("Fair", atStart(func(c *ctx, p int) { c.PlusBuys(1) }))
	reg("Fleet", rules{}) // the engine's game end reads it
	reg("Guildhall", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if project(c, e.P) && c.G.D(e.C).Treasure() {
				c.Away().PlusCoffers(1) // "gets +1 Coffers. (Guildhall)"
			}
		},
	})
	reg("Innovation", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) { // "gains a Vagrant.", then a level down "plays a Vagrant."
			if e.P != c.G.Cur || !project(c, e.P) || !c.G.D(e.C).Action() || c.T2().Used[c.Src] > 0 {
				return
			}
			c.T2().Used[c.Src]++
			if stillGained(c, e) && c.Yes(e.P, "set it aside and play it", "PLAY") {
				e.Done = true
				c.MoveTo(e.P, one(e.X), aside, "")
				c.PlayNow(e.X)
			}
		},
	})
	reg("Pageant", rules{
		Hears: engine.On(engine.EvBuyEnd),
		Hear: func(c *ctx, e *ev) { // "ends their buy phase.", "loses 1 Coin.", "gets +1 Coffers. (Pageant)"
			if project(c, e.P) && c.Pl(e.P).Coins > 0 && c.Yes(e.P, "pay $1 for +1 Coffers", "LOSES_COIN") {
				c.LoseCoins(1)
				c.Away().PlusCoffers(1)
			}
		},
	})
	reg("Piazza", atStart(func(c *ctx, p int) { // "reveals a Copper.", "topdecks a Copper with Piazza."; an Action "plays a Council Room. (|)"
		x := c.TopCard(p)
		if x < 0 {
			return
		}
		c.Reveal(p, one(x))
		if c.G.Def(x).Action() {
			c.PlayBlank(x)
		} else {
			c.MoveTo(p, one(x), deck, "TOPDECK_WITH", engine.Name(c.Src))
		}
	}))
	reg("Road Network", rules{
		Hears: engine.On(engine.EvGain),
		Hear: func(c *ctx, e *ev) {
			if q := 1 - e.P; project(c, q) && c.G.D(e.C).Victory() {
				c.As(q).Away().Draw(1) // "draws a card. (Road Network)"
			}
		},
	})
	reg("Sewers", rules{
		Hears: engine.On(engine.EvTrash),
		Hear: func(c *ctx, e *ev) {
			if project(c, e.P) && e.By != c.Src {
				c.Away().Trash(e.P, one(pick1(c, e.P, engine.ToTrash, hand, nil, true)))
			}
		},
	})
	reg("Silos", atStart(func(c *ctx, p int) {
		if got := c.Discard(p, pick(c, p, engine.ToDiscard, hand, named("Copper"), 0, len(c.Hand(p)))); len(got) > 0 {
			c.Draw(len(got))
		}
	}))
	reg("Sinister Plot", atStart(func(c *ctx, p int) { // a token silently; "draws 2 cards. (Sinister Plot)"
		mat := c.Pl(p).Mat
		if n := mat[c.Src]; n > 0 && c.Mode1(p, mode("add a token", ""), mode("remove the tokens", "DRAW")) == 1 {
			mat[c.Src] = 0
			c.Draw(n)
			return
		}
		mat[c.Src]++
	}))
	reg("Star Chart", rules{
		Hears: engine.On(engine.EvShuffling),
		Hear: func(c *ctx, e *ev) { // silent: the card goes on top of the new deck
			if project(c, e.P) {
				c.MoveTo(e.P, one(pick1(c, e.P, engine.ToTopdeck, discard, nil, true)), deck, "")
			}
		},
	})
}
