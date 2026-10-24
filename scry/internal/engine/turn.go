package engine

import "fmt"

// Dur is a card waiting for a later turn: a Duration in play, with what it set
// aside, or anything else that acts at the start of its owner's turns.
type Dur struct {
	P     int
	X     Inst   // the card, -1 for none
	C     CardID // whose Rules.Next runs
	Hold  []Inst // cards that stay in play with it: the Throne Room that played it
	Aside []Inst // cards it set aside
	N     int    // what it counts
	Made  int    // the turn it was made, by Game.turns
	Done  bool   // its last effect happened: it leaves play in this turn's Clean-up
	Ever  bool   // it never leaves play (Champion)
}

// Durs lists the cards waiting for later turns.
func (g *Game) Durs() []*Dur { return g.durs }

// takeTurn plays one turn of g.Cur; extra is the card that gave an extra turn, or 0.
func (g *Game) takeTurn(extra CardID) {
	p := g.Cur
	pl := &g.P[p]
	g.turns++
	g.T2 = turnState{Extra: extra, HandSize: 5}
	pl.Actions, pl.Buys, pl.Coins, pl.Potions = 1, 1, 0, 0
	text := fmt.Sprintf("Turn %d - %s", g.turn, pl.Name)
	if extra != 0 {
		text += " [" + g.T.Defs[extra].Name + "]"
	}
	g.Log.add(Entry{Kind: 'T', Text: text})

	g.T2.Phase = StartPhase
	g.Log.lazy(&Entry{Template: "STARTS_TURN", Args: []any{Seat(p)}})
	g.startTurn(p)
	g.Log.lazy(nil)

	g.T2.Phase = ActionPhase
	g.actionPhase(p)

	g.T2.Phase = BuyPhase
	g.Log.lazy(&Entry{Template: "BEGINS_BUYPHASE", Args: []any{Seat(p)}})
	g.fire(&Ev{Kind: EvBuyStart, P: p, X: -1, Level: 1})
	g.Log.lazy(nil)
	g.buyPhase(p)
	g.Log.lazy(&Entry{Template: "ENDS_BUYPHASE", Args: []any{Seat(p)}})
	g.fire(&Ev{Kind: EvBuyEnd, P: p, X: -1, Level: 1})
	g.Log.lazy(nil)

	g.T2.Phase = NightPhase
	g.nightPhase(p)

	// Clean-up, except after the last turn: dominion.games ends the game and its log there
	g.T2.Phase = CleanupPhase
	if g.over() || g.capped() {
		g.prevGained[p] = g.T2.Gained
		return
	}
	g.Log.lazy(&Entry{Template: "BEGINS_CLEANUP_PHASE", Args: []any{Seat(p)}})
	g.fire(&Ev{Kind: EvCleanup, P: p, X: -1, Level: 1})
	g.cleanup(p)
	g.fire(&Ev{Kind: EvTurnEnd, P: p, X: -1, Level: 1})
	g.prevGained[p] = g.T2.Gained
}

// startTurn resolves Durations and everything else that happens at the start of a turn.
func (g *Game) startTurn(p int) {
	for i := 0; i < len(g.durs); i++ {
		d := g.durs[i]
		if d.P != p || d.Done || d.Made == g.turns {
			continue
		}
		r := g.T.Defs[d.C].Rules
		if r == nil || r.Next == nil {
			continue
		}
		g.Log.emit()
		c := g.ctx(p, d.X, 1)
		c.Src, c.from = d.C, true
		if r.Next(c, d) && !d.Ever {
			d.Done = true
		}
	}
	g.fire(&Ev{Kind: EvTurnStart, P: p, X: -1, Level: 1})
}

func (g *Game) actionPhase(p int) {
	pl := &g.P[p]
	for {
		if pl.Actions == 0 && pl.Villagers == 0 {
			return
		}
		opts := g.actionOptions(p)
		if len(opts) == 0 {
			return
		}
		got := g.ask(&Decision{Player: p, Kind: PlayAction, Options: opts, Min: 0, Max: 1})
		if len(got) == 0 {
			return
		}
		if pl.Actions == 0 {
			pl.Villagers--
			g.Log.add(Entry{Template: "USES_VILLAGER", Args: []any{Seat(p), 1}})
		} else {
			pl.Actions--
		}
		o := got[0]
		var way CardID
		if o.Mode > 0 {
			way = g.Land[o.Mode-1]
		}
		g.play(p, o.Inst, 0, playHow{way: way})
	}
}

// actionOptions lists the Actions a player may play: from hand, each also
// with each Way, and Shadow cards from the deck.
func (g *Game) actionOptions(p int) []Option {
	var ways []int
	for i, c := range g.Land {
		if g.T.Defs[c].Is(TWay) {
			ways = append(ways, i)
		}
	}
	var out []Option
	add := func(x Inst) {
		out = append(out, Option{Card: g.cards[x], Inst: x})
		for _, w := range ways {
			out = append(out, Option{Card: g.cards[x], Inst: x, Mode: w + 1, Label: g.T.Defs[g.Land[w]].Name})
		}
	}
	for _, x := range g.P[p].Z[Hand] {
		if g.Def(x).Action() {
			add(x)
		}
	}
	for _, x := range g.P[p].Z[Deck] {
		if d := g.Def(x); d.Action() && d.Is(TShadow) {
			add(x)
		}
	}
	return out
}

func (g *Game) buyPhase(p int) {
	pl := &g.P[p]
	for {
		opts := g.options(p, Hand, func(d *Def) bool { return d.Treasure() || d.ID == g.curse && g.CurseTreasure })
		if len(opts) == 0 {
			break
		}
		got := g.ask(&Decision{Player: p, Kind: PlayTreasures, Options: opts, Min: 0, Max: len(opts)})
		if len(got) == 0 {
			break
		}
		for _, o := range got {
			if g.loc[o.Inst] == (Loc{Hand, int16(p)}) { // not one a Crystal Ball's Masquerade passed
				g.play(p, o.Inst, 0, playHow{})
			}
		}
		g.Log.flush()
	}
	if pl.Coffers > 0 {
		var opts []Option
		for n := 0; n <= pl.Coffers; n++ {
			opts = append(opts, Option{Mode: n, Label: fmt.Sprint(n), Inst: -1})
		}
		got := g.ask(&Decision{Player: p, Kind: UseCoffers, Options: opts, Min: 1, Max: 1})
		if n := got[0].Mode; n > 0 {
			pl.Coffers -= n
			g.Log.add(Entry{Template: plural(n, "USES_COFFER", "USES_COFFERS"), Args: []any{Seat(p), n}})
			pl.Coins += n
			g.Log.add(Entry{Template: plural(n, "GETS_COIN", "GETS_COINS"), Args: []any{Seat(p), n}})
		}
	}
	for {
		g.repay(p)
		if pl.Buys == 0 {
			break
		}
		opts := g.buyOptions(p)
		got := g.ask(&Decision{Player: p, Kind: Buy, Options: opts, Min: 0, Max: 1})
		if len(got) == 0 {
			break
		}
		g.buy(p, got[0].Card)
	}
}

// repay pays off debt with $.
func (g *Game) repay(p int) {
	pl := &g.P[p]
	n := min(pl.Debt, pl.Coins)
	if n == 0 {
		return
	}
	pl.Debt -= n
	pl.Coins -= n
	if pl.Debt > 0 {
		g.Log.add(Entry{Template: "REPAYS_SOME_DEBT", Args: []any{Seat(p), n, pl.Debt}})
	} else {
		g.Log.add(Entry{Template: "REPAYS_DEBT", Args: []any{Seat(p), n}})
	}
}

// CanAfford: a player has the $ and Potions for a cost, and no debt.
func (g *Game) CanAfford(p int, cost Cost) bool {
	pl := &g.P[p]
	return pl.Debt == 0 && cost.Coins <= pl.Coins && cost.Potion <= pl.Potions
}

// buyOptions lists the cards, Events and Projects a player can buy now.
func (g *Game) buyOptions(p int) []Option {
	var out []Option
	for i := range g.Piles {
		pile := &g.Piles[i]
		if !pile.Supply || len(pile.Cards) == 0 || g.T2.NoBuy {
			continue
		}
		c := g.cards[pile.Cards[len(pile.Cards)-1]]
		if g.CanBuy(p, c) {
			out = append(out, Option{Card: c, Inst: -1})
		}
	}
	for _, c := range g.Land {
		d := g.T.Defs[c]
		if d.Is(TEvent|TProject) && g.CanBuy(p, c) {
			out = append(out, Option{Card: c, Inst: -1})
		}
	}
	return out
}

// CanBuy: player p may buy c now, cost and rules allowing.
func (g *Game) CanBuy(p int, c CardID) bool {
	d := g.T.Defs[c]
	if !g.CanAfford(p, g.CostOf(c)) {
		return false
	}
	if d.Is(TProject) && g.HasProject(p, c) {
		return false
	}
	for _, f := range g.T2.Forbid {
		if f == c {
			return false
		}
	}
	if r := d.Rules; r != nil && r.CanBuy != nil && !r.CanBuy(g, p) {
		return false
	}
	for _, h := range g.Held {
		if h.P == p && g.T.Defs[h.C].Name == "Deluded" && d.Action() && !d.Landscape() {
			return false
		}
	}
	return true
}

// HasProject: player p has a cube on Project c.
func (g *Game) HasProject(p int, c CardID) bool {
	for _, x := range g.P[p].Projects {
		if x == c {
			return true
		}
	}
	return false
}

// buy buys a card, Event or Project.
func (g *Game) buy(p int, c CardID) {
	pl := &g.P[p]
	d := g.T.Defs[c]
	cost := g.CostOf(c)
	pl.Buys--
	pl.Coins -= cost.Coins
	pl.Potions -= cost.Potion
	g.T2.Bought = append(g.T2.Bought, c)
	if d.Landscape() {
		if d.Rules == nil {
			panic(MissingRules{d.Name})
		}
		g.Log.add(Entry{Template: "BUY", Args: []any{Seat(p), []CardID{c}}})
		g.P[p].Debt += cost.Debt
		if d.Is(TProject) {
			pl.Projects = append(pl.Projects, c)
		}
		g.fire(&Ev{Kind: EvBuy, P: p, X: -1, C: c, Level: 1})
		if d.Rules != nil && d.Rules.Buy != nil {
			cx := g.ctx(p, -1, 1)
			cx.Src = c
			d.Rules.Buy(cx)
		}
		return
	}
	i := g.PileOf(c)
	pile := g.Piles[i].Cards
	x := pile[len(pile)-1]
	over := 0
	if d.Rules != nil && d.Rules.Overpay != nil && pl.Coins > 0 {
		over = g.ctx(p, x, 1).Number(p, 0, pl.Coins, "LOSES_COINS")
	}
	if over > 0 { // "buys an Infirmary.", "loses 2 Coins.", "gains an Infirmary."
		g.Log.add(Entry{Template: "BUY", Args: []any{Seat(p), []CardID{c}}})
		pl.Coins -= over
		g.Log.add(Entry{Level: 1, Template: plural(over, "LOSES_COIN", "LOSES_COINS"), Args: []any{Seat(p), over}})
	} else {
		g.Log.add(Entry{Template: "BUY_AND_GAIN", Args: []any{Seat(p), []CardID{c}}})
	}
	line := g.Log.last()
	g.P[p].Debt += cost.Debt
	ev := &Ev{Kind: EvBuy, P: p, X: x, C: c, Level: 1}
	g.fire(ev)
	for range g.Piles[i].Embargo {
		g.ctx(p, -1, 1).Gain(p, g.curse, Discard)
	}
	if g.loc[x].Zone != Supply { // what the buy caused took the card: an Embargo's Curse, Talisman's copy
		if pile = g.Piles[i].Cards; len(pile) == 0 || g.cards[pile[len(pile)-1]] != c {
			g.Log.unGain(line) // "buys a Curse": the pile ran out first
			return
		}
		x = pile[len(pile)-1]
	}
	if over > 0 {
		g.Log.add(Entry{Template: "GAIN", Args: []any{Seat(p), []CardID{c}}})
	}
	g.gainX(p, x, Discard, 1, 0, true)
	if over > 0 {
		d.Rules.Overpay(g.ctx(p, x, 1), over)
	}
}

func (g *Game) takeDebt(p, n, level int) {
	if n > 0 {
		g.P[p].Debt += n
		g.Log.add(Entry{Level: level, Template: "TAKES_DEBT", Args: []any{Seat(p), n}})
	}
}

func (g *Game) nightPhase(p int) {
	for {
		opts := g.options(p, Hand, func(d *Def) bool { return d.Is(TNight) })
		if len(opts) == 0 {
			return
		}
		got := g.ask(&Decision{Player: p, Kind: PlayNight, Options: opts, Min: 0, Max: 1})
		if len(got) == 0 {
			return
		}
		g.play(p, got[0].Inst, 0, playHow{})
	}
}

// cleanup discards the hand and the cards in play that don't stay, and draws the next hand.
func (g *Game) cleanup(p int) {
	pl := &g.P[p]
	stay := map[Inst]bool{}
	for _, d := range g.durs {
		if !d.Done {
			if d.X >= 0 {
				stay[d.X] = true
			}
			for _, h := range d.Hold {
				stay[h] = true
			}
		}
	}
	// the other player's cards too: what they played on this turn (Guard Dog) leaves with it
	for _, q := range [2]int{p, 1 - p} {
		for _, x := range append([]Inst{}, g.P[q].Z[InPlay]...) {
			if stay[x] || g.loc[x].Zone != InPlay {
				continue
			}
			g.move(x, InPlay, Loc{Discard, int16(q)})
			g.fire(&Ev{Kind: EvDiscard, P: q, X: x, C: g.cards[x], From: InPlay, Cleanup: true, Level: 1})
		}
	}
	for len(pl.Z[Hand]) > 0 {
		g.move(pl.Z[Hand][0], Hand, Loc{Discard, int16(p)})
	}
	g.Log.lazy(nil)
	live := g.durs[:0]
	for _, d := range g.durs {
		if !d.Done || d.P != p {
			live = append(live, d)
		}
	}
	g.durs = live
	w := g.watches[:0]
	for _, x := range g.watches {
		if x.Long {
			w = append(w, x)
		}
	}
	g.watches = w
	pl.Actions, pl.Buys, pl.Coins, pl.Potions = 0, 0, 0, 0
	g.draw(p, g.T2.HandSize, 0, 0)
}

// playHow says how a card is played: again by a Throne Room, with a Way, by what.
type playHow struct {
	times   int    // 0 the first play, 1 again, 2 a third time
	line    string // the play line's template instead of PLAY: REACTS_PLAYING
	way     CardID // the Way used instead of the card's effect
	by      CardID // the card that plays it outside its own turn's flow ("(Prince)"), for PLAY_WITH
	holders []Inst // cards that played it: they stay in play while it does
}

// play puts a card into play from where it is (or plays it where it is, again)
// and runs its rules. level is the play line's log nesting.
func (g *Game) play(p int, x Inst, level int, how playHow) {
	d := g.Def(x)
	if d.Rules == nil {
		panic(MissingRules{d.Name})
	}
	if z := g.loc[x].Zone; how.times == 0 && z != InPlay && z != Supply { // a Trail played from the trash comes into play
		g.move(x, z, Loc{InPlay, int16(p)})
	}
	coins := g.coinsOf(p, x) // before it counts as played this turn (Fool's Gold)
	c := g.ctx(p, x, level+1)
	c.holders = how.holders
	card := []CardID{g.cards[x]}
	switch {
	case how.way != 0:
		if g.T.Defs[how.way].Rules == nil {
			panic(MissingRules{g.T.Defs[how.way].Name})
		}
		g.Log.add(Entry{Level: level, Template: "PLAY_USING_WAY", Args: []any{Seat(p), card, Name(how.way)}})
	case how.by != 0 && how.times == 0:
		g.Log.add(Entry{Level: level, Template: "PLAY_WITH", Args: []any{Seat(p), card, Name(how.by)}})
	case how.line != "":
		g.Log.add(Entry{Level: level, Template: how.line, Args: []any{Seat(p), card}})
	case g.isTreasure(x) && !d.Action() && !d.Rules.Alone && level == 0 && how.times == 0 && g.T2.Phase == BuyPhase:
		g.Log.treasure(p, g.cards[x], coins)
	case g.isTreasure(x) && !d.Action() && !d.Rules.Alone: // played by another card: "plays a Copper again. (+$1)"
		t := [...]string{"PLAY_TREASURES_FOR", "PLAY_TREASURES_AGAIN_FOR", "PLAY_TREASURES_THIRD_FOR"}[min(how.times, 2)]
		g.Log.add(Entry{Level: level, Template: t, Args: []any{Seat(p), card, coins}})
	case how.times == 1 || how.times > 2: // a fourth Infirmary is "again" too
		g.Log.add(Entry{Level: level, Template: "PLAY_AGAIN", Args: []any{Seat(p), card}})
	case how.times == 2:
		g.Log.add(Entry{Level: level, Template: "PLAY_THIRD", Args: []any{Seat(p), card}})
	default:
		g.Log.add(Entry{Level: level, Template: "PLAY", Args: []any{Seat(p), card}})
	}
	g.T2.Played = append(g.T2.Played, x)
	if d.Action() {
		g.T2.ActionsIn++
	}
	if how.way != 0 {
		w := g.T.Defs[how.way]
		c.Src = how.way
		if w.Rules != nil && w.Rules.Play != nil {
			w.Rules.Play(c)
		}
		c.Src = g.cards[x]
	} else {
		if d.Attack() {
			ev := &Ev{Kind: EvAttack, P: p, X: x, C: g.cards[x], Victim: 1 - p, Level: level + 1}
			g.fire(ev)
			c.unaffected[1-p] = ev.Unaffected
		}
		g.P[p].Coins += coins
		if d.Rules.Play != nil {
			d.Rules.Play(c)
		}
	}
	g.fire(&Ev{Kind: EvPlay, P: p, X: x, C: g.cards[x], Level: level + 1})
}

// isTreasure: the card is a Treasure in this game (Charlatan makes Curses Treasures).
func (g *Game) isTreasure(x Inst) bool {
	return g.Def(x).Treasure() || g.cards[x] == g.curse && g.CurseTreasure
}

// coinsOf is a Treasure's fixed $ as played now (Envious makes Silver and Gold $1).
func (g *Game) coinsOf(p int, x Inst) int {
	r := g.Def(x).Rules
	n := r.Coins
	if r.Worth != nil {
		n = r.Worth(g, p)
	}
	if g.cards[x] == g.curse && g.CurseTreasure {
		n = 1
	}
	if g.cards[x] == g.copper {
		n += g.T2.CopperBonus
	}
	if n > 1 && (g.cards[x] == g.silver || g.cards[x] == g.gold) {
		for _, h := range g.Held {
			if h.P == p && g.T.Defs[h.C].Name == "Envious" {
				return 1
			}
		}
	}
	return n
}

// CostOf is a card's cost now, with everything that changes it.
func (g *Game) CostOf(c CardID) Cost {
	d := g.T.Defs[c]
	cost := d.Cost
	if d.Rules != nil && d.Rules.SelfCost != nil {
		d.Rules.SelfCost(g, g.Cur, &cost)
	}
	if d.Landscape() {
		return cost
	}
	off := g.T2.Reduce
	for p := range g.P {
		for _, x := range g.P[p].Z[InPlay] {
			if r := g.Def(x).Rules; r != nil && r.Reduce != nil {
				off += r.Reduce(g, x, c)
			}
		}
	}
	for _, l := range g.Land {
		if r := g.T.Defs[l].Rules; r != nil && r.Reduce != nil {
			off += r.Reduce(g, -1, c)
		}
	}
	if i := g.PileOf(c); i >= 0 && g.P[g.Cur].Tokens[TokCost] == i {
		off += 2
	}
	cost.Coins = max(0, cost.Coins-off)
	return cost
}

// Cost is a card's current cost in $ only.
func (g *Game) Cost(c CardID) int { return g.CostOf(c).Coins }
