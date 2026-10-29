package engine

import (
	"fmt"
	"slices"
)

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
	g.T2 = turnState{Extra: extra, HandSize: 5, Used: map[CardID]int{}}
	pl.Actions, pl.Buys, pl.Coins, pl.Potions = 1, 1, 0, 0
	text := fmt.Sprintf("Turn %d - %s", g.turn, pl.Name)
	if extra != 0 {
		text += " [" + g.T.Defs[extra].Name + "]"
	}
	g.Log.add(Entry{Kind: 'T', Text: text})
	if pl.Skip > 0 { // Lich: "fails to take a Turn (from |).", "skips a Turn."
		pl.Skip--
		g.Log.add(Entry{Template: "CANCELS_A_TURN", Args: []any{Seat(p), Name(0)}})
		if pl.Skip > 0 {
			g.Log.add(Entry{Template: "SKIPS_A_TURN_REMAINING", Args: []any{Seat(p), pl.Skip}})
		} else {
			g.Log.add(Entry{Template: "SKIPS_A_TURN", Args: []any{Seat(p)}})
		}
		return
	}

	g.T2.Phase = StartPhase
	g.Log.lazy(&Entry{Template: "STARTS_TURN", Args: []any{Seat(p)}})
	g.startTurn(p)
	if g.KeepStart != nil && g.KeepStart(p) {
		g.Log.emit()
	}
	g.Log.lazy(nil)

	for { // Villa gained in the Buy phase returns to the Action phase, silently
		g.T2.Phase = ActionPhase
		g.actionPhase(p)

		g.T2.Phase = BuyPhase
		g.Log.lazy(&Entry{Template: "BEGINS_BUYPHASE", Args: []any{Seat(p)}})
		g.fire(&Ev{Kind: EvBuyStart, P: p, X: -1, Level: 1})
		g.Log.lazy(nil)
		g.buyPhase(p)
		if g.T2.Back {
			g.T2.Back = false
			continue
		}
		g.Log.lazy(&Entry{Template: "ENDS_BUYPHASE", Args: []any{Seat(p)}})
		g.fire(&Ev{Kind: EvBuyEnd, P: p, X: -1, Level: 1})
		g.Log.lazy(nil)
		break
	}

	g.T2.Phase = NightPhase
	g.nightPhase(p)

	// Clean-up, except after the last turn: dominion.games ends the game and its log there
	g.T2.Phase = CleanupPhase
	if g.over() && len(g.fleetLeft()) == 0 || !g.over() && g.capped() { // once over, the Fleet round decides
		g.prevGained[p], g.prevTrashed[p] = g.T2.Gained, g.T2.Trashed
		return
	}
	g.Log.lazy(&Entry{Template: "BEGINS_CLEANUP_PHASE", Args: []any{Seat(p)}})
	g.fire(&Ev{Kind: EvCleanup, P: p, X: -1, Level: 1})
	g.cleanup(p)
	g.fire(&Ev{Kind: EvTurnEnd, P: p, X: -1, Level: 1})
	g.prevGained[p], g.prevTrashed[p] = g.T2.Gained, g.T2.Trashed
}

// startTurn resolves Durations and everything else that happens at the start of a turn.
func (g *Game) startTurn(p int) {
	g.react(p, &Ev{Kind: EvTurnStart, P: p, X: -1, Level: 1}) // a Clerk before the Durations, or after them below
	ran, started := map[*Dur]bool{}, false
	for {
		var ds []*Dur
		for _, d := range g.durs {
			if r := g.T.Defs[d.C].Rules; d.P == p && !d.Done && d.Made != g.turns && !ran[d] && r != nil && r.Next != nil {
				ds = append(ds, d)
			}
		}
		if len(ds) == 0 {
			break
		}
		d := ds[0]
		if g.NextDur != nil && (len(ds) > 1 || !started) {
			k := g.NextDur(p, ds)
			if k < 0 && !started { // the rest of the turn's start first: a Reserve called before an Importer's gain
				started = true
				g.fire(&Ev{Kind: EvTurnStart, P: p, X: -1, Level: 1})
				continue
			}
			d = ds[max(k, 0)]
		}
		ran[d] = true
		r := g.T.Defs[d.C].Rules
		g.Log.emit()
		c := g.ctx(p, d.X, 1)
		c.Src, c.from = d.C, true
		if r.Next(c, d) && !d.Ever {
			d.Done = true
		}
	}
	if !started {
		g.fire(&Ev{Kind: EvTurnStart, P: p, X: -1, Level: 1})
	}
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
		if g.forbidden(p, x) {
			return
		}
		out = append(out, Option{Card: g.cards[x], Inst: x})
		for _, w := range ways {
			out = append(out, Option{Card: g.cards[x], Inst: x, Mode: w + 1, Label: g.T.Defs[g.Land[w]].Name})
		}
	}
	for _, x := range g.handPlays(p) {
		switch {
		case g.Def(x).Action() || g.inherits(p, x):
			add(x)
		case g.CameTrue("Enlightenment") && g.isTreasure(x) && !g.forbidden(p, x): // Treasures are also Actions
			out = append(out, Option{Card: g.cards[x], Inst: x})
		}
	}
	for _, x := range g.P[p].Z[Deck] {
		if d := g.Def(x); d.Action() && d.Is(TShadow) && !slices.ContainsFunc(g.P[p].Z[Hand], func(y Inst) bool { return g.cards[y] == g.cards[x] }) {
			add(x) // not a copy of one in hand: the log can't tell them apart
		}
	}
	return out
}

// handPlays is the player's hand, or nothing when Voyage's limit on cards played from hand is reached.
func (g *Game) handPlays(p int) []Inst {
	if g.T2.HandLimit > 0 && g.T2.FromHand >= g.T2.HandLimit {
		return nil
	}
	return g.P[p].Z[Hand]
}

// forbidden: a card in the other player's play forbids playing x from hand (Warlord).
func (g *Game) forbidden(p int, x Inst) bool {
	for _, y := range g.P[1-p].Z[InPlay] {
		if r := g.Def(y).Rules; r != nil && r.Forbid != nil && r.Forbid(g, y, p, x) {
			return true
		}
	}
	return false
}

func (g *Game) buyPhase(p int) {
	pl := &g.P[p]
	for {
		opts := g.options(p, Hand, func(d *Def) bool { return d.Treasure() || d.ID == g.curse && g.CurseTreasure || g.alsoTreasure(p, d) })
		if len(g.handPlays(p)) == 0 || len(opts) == 0 {
			break
		}
		max := len(opts)
		if g.T2.HandLimit > 0 {
			max = min(max, g.T2.HandLimit-g.T2.FromHand)
		}
		got := g.ask(&Decision{Player: p, Kind: PlayTreasures, Options: opts, Min: 0, Max: max})
		if len(got) == 0 && g.repay(p) { // the debt first, then more Treasures: "plays a Copper.", "repays 1 debt.", "plays 2 Coppers."
			continue
		}
		if len(got) == 0 {
			break
		}
		for _, o := range got {
			if g.loc[o.Inst] == (Loc{Hand, int16(p)}) && !g.T2.Back { // not one a Crystal Ball's Masquerade passed
				g.play(p, o.Inst, 0, playHow{})
			}
		}
		g.Log.flush()
		if g.T2.Back {
			return
		}
	}
	for {
		g.useCoffers(p) // before any buy: "buys and gains a Conclave.", "uses 2 Coffers.", maybe again: "uses 1 Coffers."
		g.repay(p)
		g.useCoffers(p)                                                        // or after the debt: "repays 1 debt.", "uses 1 Coffers.", "buys Piazza."
		if pl.Buys == 0 && pl.Actions > 0 && g.CameTrue("Flourishing Trade") { // an Action play used as a Buy, unlogged
			pl.Actions--
			pl.Buys++
		}
		if pl.Buys == 0 {
			break
		}
		opts := g.buyOptions(p)
		got := g.ask(&Decision{Player: p, Kind: Buy, Options: opts, Min: 0, Max: 1})
		if len(got) == 0 {
			break
		}
		g.buy(p, got[0].Card, got[0].Mode == 1)
		if g.T2.Back {
			return
		}
	}
}

// repay offers to pay off debt with $ now; what is left at the end of the turn
// pays it after the Clean-up draw: "draws 5 cards.", "loses 4 Coins."
func (g *Game) repay(p int) bool {
	pl := &g.P[p]
	n := min(pl.Debt, pl.Coins)
	if n == 0 {
		return false
	}
	t := "REPAYS_DEBT"
	if pl.Debt > n {
		t = "REPAYS_SOME_DEBT"
	}
	if g.ctx(p, -1, 0).Mode1(p, Mode{Label: "not now"}, Mode{Label: "repay", Line: t}) != 1 { // bots take the last mode
		return false
	}
	pl.Debt -= n
	pl.Coins -= n
	if pl.Debt > 0 {
		g.Log.add(Entry{Template: "REPAYS_SOME_DEBT", Args: []any{Seat(p), n, pl.Debt}})
	} else {
		g.Log.add(Entry{Template: "REPAYS_DEBT", Args: []any{Seat(p), n}})
	}
	return true
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
		if r := g.T.Defs[c].Rules; r != nil && r.PayWith != nil && g.P[p].Debt == 0 && g.allowed(p, c) && len(g.options(p, Hand, r.PayWith)) > 0 {
			out = append(out, Option{Card: c, Inst: -1, Mode: 1, Label: "trash a card instead"})
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
	return g.CanAfford(p, g.CostOf(c)) && g.allowed(p, c)
}

// allowed: the rules let player p buy c, whatever it costs.
func (g *Game) allowed(p int, c CardID) bool {
	d := g.T.Defs[c]
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
	return !(g.T2.Deluded && p == g.Cur && d.Action() && !d.Landscape())
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

// buy buys a card, Event or Project; instead, by trashing what its PayWith
// takes rather than paying its cost.
func (g *Game) buy(p int, c CardID, instead bool) {
	pl := &g.P[p]
	d := g.T.Defs[c]
	cost := g.CostOf(c)
	if instead {
		cost = Cost{}
	}
	pl.Buys--
	pl.Coins -= cost.Coins
	pl.Potions -= cost.Potion
	g.T2.Bought = append(g.T2.Bought, c)
	if d.Landscape() {
		if d.Rules == nil {
			panic(MissingRules{d.Name})
		}
		g.Log.add(Entry{Template: "BUY", Args: []any{Seat(p), []CardID{c}}})
		g.P[g.gainer(p)].Debt += cost.Debt
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
	over, coffers := 0, 0
	if d.Rules != nil && d.Rules.Overpay != nil && pl.Coffers > 0 { // to overpay with: "buys a Farrier.", "uses 1 Coffers. (+$1)", "gets +$1.", "loses 2 Coins."
		var opts []Option
		for n := 0; n <= pl.Coffers; n++ {
			opts = append(opts, Option{Mode: n, Label: fmt.Sprint(n), Inst: -1})
		}
		coffers = g.ask(&Decision{Player: p, Kind: UseCoffers, Source: c, Options: opts, Min: 1, Max: 1, Level: 1})[0].Mode
		pl.Coffers -= coffers
		pl.Coins += coffers
	}
	if d.Rules != nil && d.Rules.Overpay != nil && pl.Coins > 0 {
		over = g.ctx(p, x, 1).Amount(p, 0, pl.Coins, "LOSES_COIN", "LOSES_COINS")
	}
	if over == 0 { // no overpay to use them on
		pl.Coffers += coffers
		pl.Coins -= coffers
		coffers = 0
	}
	switch {
	case over > 0: // "buys an Infirmary.", "loses 2 Coins.", "gains an Infirmary."
		g.Log.add(Entry{Template: "BUY", Args: []any{Seat(p), []CardID{c}}})
		if coffers > 0 {
			g.Log.add(Entry{Level: 1, Template: plural(coffers, "USES_COFFER", "USES_COFFERS"), Args: []any{Seat(p), coffers}})
			g.Log.add(Entry{Level: 1, Template: plural(coffers, "GETS_COIN", "GETS_COINS"), Args: []any{Seat(p), coffers}})
		}
		pl.Coins -= over
		g.Log.add(Entry{Level: 1, Template: plural(over, "LOSES_COIN", "LOSES_COINS"), Args: []any{Seat(p), over}})
	case instead: // "buys an Animal Fair.", "trashes a Jester.", "gains an Animal Fair."
		g.Log.add(Entry{Template: "BUY", Args: []any{Seat(p), []CardID{c}}})
		cx := g.ctx(p, -1, 1)
		cx.Src = c
		cx.Trash(p, Cards(cx.Choose(p, ToTrash, cx.Options(p, Hand, d.Rules.PayWith), 1, 1)))
	case g.gainer(p) != p: // a Possession turn: "buys a Gold.", and the possessor "gains a Gold."
		g.Log.add(Entry{Template: "BUY", Args: []any{Seat(p), []CardID{c}}})
	default:
		g.Log.add(Entry{Template: "BUY_AND_GAIN", Args: []any{Seat(p), []CardID{c}}})
	}
	line := g.Log.last()
	gp := g.gainer(p)
	g.P[gp].Debt += cost.Debt
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
	if over > 0 || instead || gp != p {
		g.Log.add(Entry{Template: "GAIN", Args: []any{Seat(gp), []CardID{c}}})
	}
	g.gainX(gp, x, Discard, 1, 0, true)
	if over > 0 {
		d.Rules.Overpay(g.ctx(p, x, 1), over)
	}
	g.fire(&Ev{Kind: EvBought, P: p, X: x, C: c, Level: 1})
}

// useCoffers lets a player spend Coffers in the buy phase, as many times as they like.
func (g *Game) useCoffers(p int) {
	pl := &g.P[p]
	for pl.Coffers > 0 && pl.Buys > 0 {
		var opts []Option
		for n := 0; n <= pl.Coffers; n++ {
			opts = append(opts, Option{Mode: n, Label: fmt.Sprint(n), Inst: -1})
		}
		n := g.ask(&Decision{Player: p, Kind: UseCoffers, Options: opts, Min: 1, Max: 1})[0].Mode
		if n == 0 {
			return
		}
		pl.Coffers -= n
		g.Log.add(Entry{Template: plural(n, "USES_COFFER", "USES_COFFERS"), Args: []any{Seat(p), n}})
		pl.Coins += n
		g.Log.add(Entry{Template: plural(n, "GETS_COIN", "GETS_COINS"), Args: []any{Seat(p), n}})
	}
}

func (g *Game) takeDebt(p, n, level int) {
	p = g.gainer(p)
	if n > 0 {
		g.P[p].Debt += n
		g.Log.add(Entry{Level: level, Template: "TAKES_DEBT", Args: []any{Seat(p), n}})
	}
}

func (g *Game) nightPhase(p int) {
	for {
		opts := g.options(p, Hand, func(d *Def) bool { return d.Is(TNight) })
		if len(g.handPlays(p)) == 0 || len(opts) == 0 {
			return
		}
		got := g.ask(&Decision{Player: p, Kind: PlayNight, Options: opts, Min: 0, Max: 1})
		if len(got) == 0 {
			return
		}
		g.play(p, got[0].Inst, 0, playHow{})
	}
}

// offerWay lets a player use a Way on an Action another card plays: "plays a
// Bridge Troll. (Way of the Turtle)", then "uses Way of the Turtle." a level down.
func (g *Game) offerWay(p int, x Inst, level int, line string) CardID {
	ms := []Mode{{Label: "no"}}
	var ways []CardID
	for _, c := range g.Land {
		if g.T.Defs[c].Is(TWay) {
			ms = append(ms, Mode{Label: "use " + g.T.Defs[c].Name, Line: line, Card: c})
			ways = append(ways, c)
		}
	}
	if len(ways) == 0 {
		return 0
	}
	k := g.ctx(p, x, level).Mode1(p, ms...)
	if k == 0 {
		return 0
	}
	if line == "USES_WAY" {
		g.Log.add(Entry{Level: level, Template: "USES_WAY", Args: []any{Seat(p), Name(ways[k-1])}})
	}
	return ways[k-1]
}

// cleanup discards the hand and the cards in play that don't stay, and draws the next hand.
func (g *Game) cleanup(p int) {
	pl := &g.P[p]
	stay := map[Inst]bool{}
	for _, d := range g.durs {
		if !d.Done && d.X >= 0 && g.loc[d.X].Zone == InPlay { // not the Counterfeit whose Astrolabe it trashed
			stay[d.X] = true
			for k := len(d.Hold) - 1; k >= 0 && g.loc[d.Hold[k]].Zone == InPlay; k-- { // nor a Procession whose Throne Room it trashed
				stay[d.Hold[k]] = true
			}
		}
	}
	// the other player's cards too: what they played on this turn (Guard Dog) leaves with it
	for _, q := range [2]int{p, 1 - p} {
		if q == p && g.T2.KeepPlay {
			pl.Kept = append([]Inst{}, pl.Z[InPlay]...)
			continue
		}
		var xs []Inst
		for _, x := range g.P[q].Z[InPlay] {
			if !stay[x] && !(q != p && contains(g.P[q].Kept, x)) {
				xs = append(xs, x)
			}
		}
		if q != p && len(xs) > 0 { // "discards a Cutthroat." for the other player's
			g.Log.addAside(Entry{Template: "DISCARD", Args: []any{Seat(q), g.ids(xs)}})
		}
		for _, x := range xs {
			if g.loc[x].Zone != InPlay {
				continue
			}
			g.move(x, InPlay, Loc{Discard, int16(q)})
			g.fire(&Ev{Kind: EvDiscard, P: q, X: x, C: g.cards[x], From: InPlay, Cleanup: true, Level: 1})
		}
	}
	// the Travellers not exchanged yet, once more: the player picks the order ("returns a Treasure Hunter." before "returns a Page.")
	for again := g.T2.Unswapped; len(again) > 0; again = nil {
		g.T2.Unswapped = nil
		for _, x := range again {
			if g.loc[x].Zone == Discard {
				g.Def(x).Rules.OnDiscard(g.ctx(p, x, 1), &Ev{Kind: EvDiscard, P: p, X: x, C: g.cards[x], From: InPlay, Cleanup: true, Again: true, Level: 1})
			}
		}
	}
	if !g.T2.KeepPlay {
		pl.Kept = nil
	}
	for _, x := range slices.Clone(pl.Z[Hand]) {
		if !contains(g.T2.KeepHand, x) {
			g.move(x, Hand, Loc{Discard, int16(p)})
		}
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
	left := pl.Coins
	pl.Actions, pl.Buys, pl.Coins, pl.Potions = 0, 0, 0, 0
	if n, k, d := g.T2.HandSize, g.T2.LateDraw, len(pl.Z[Deck]); k > 0 && d >= n && d < n+k {
		// Farrier's cards after the hand, on one line with it unless a shuffle comes between:
		// "draws 5 cards.", "shuffles their deck.", "draws 3 cards."
		g.draw(p, n, 0, 0)
		g.draw(p, k, 0, 0)
	} else {
		g.draw(p, n+k, 0, 0)
	}
	if n := min(left, pl.Debt); n > 0 { // the $ left pays the debt: "draws 5 cards.", "loses 2 Coins."
		pl.Debt -= n
		g.Log.add(Entry{Template: plural(n, "LOSES_COIN", "LOSES_COINS"), Args: []any{Seat(p), n}})
	}
}

// playHow says how a card is played: again by a Throne Room, with a Way, by what.
type playHow struct {
	times   int    // 0 the first play, 1 again, 2 a third time
	line    string // the play line's template instead of PLAY: REACTS_PLAYING
	way     CardID // the Way used instead of the card's effect
	by      CardID // the card that plays it outside its own turn's flow ("(Prince)"), for PLAY_WITH
	blank   bool   // PLAY_WITH naming nothing: Patient's "plays a Nobles. (|)"
	holders []Inst // cards that played it: they stay in play while it does
	inPlace bool   // played where it is, and left there: Inheritance's set-aside card
	self    Inst   // with inPlace: the Estate or Overlord that plays it, which stays in play for a Duration
	merge   bool   // a Treasure played by an Action, on one line with the others it plays (Storyteller)
	asked   bool   // the Way was offered before the play line
}

// play puts a card into play from where it is (or plays it where it is, again)
// and runs its rules. level is the play line's log nesting.
func (g *Game) play(p int, x Inst, level int, how playHow) {
	d := g.Def(x)
	if d.Rules == nil {
		panic(MissingRules{d.Name})
	}
	if how.way == 0 && how.times == 0 && how.by == 0 && !how.blank && how.line == "" && level > 0 && d.Action() && g.loc[x].Zone == Hand {
		how.way, how.asked = g.offerWay(p, x, level, "PLAY_USING_WAY"), true // one from the hand on its line: Shop's "plays a Ruined Village using Way of the Ox."
	}
	if z := g.loc[x].Zone; how.times == 0 && !how.inPlace && z != InPlay && z != Supply { // a Trail played from the trash comes into play
		g.move(x, z, Loc{InPlay, int16(p)})
		if z == Hand && p == g.Cur {
			g.T2.FromHand++
		}
	}
	coins := g.coinsOf(p, x) // before it counts as played this turn (Fool's Gold)
	c := g.ctx(p, x, level+1)
	c.holders = how.holders
	if how.inPlace {
		c.Me, c.InPlace = how.self, true
	}
	card := []CardID{g.cards[x]}
	null := g.T2.Nullify && p == g.Cur && how.times == 0 && g.isTreasure(x)
	if null { // Highwayman: "plays a Copper. (+$1)", then "plays Copper (nullified by Highwayman)." on a line of its own
		g.T2.Nullify = false
		g.Log.flush()
	}
	switch {
	case how.way != 0:
		if g.T.Defs[how.way].Rules == nil {
			panic(MissingRules{g.T.Defs[how.way].Name})
		}
		g.Log.holdWay(Entry{Level: level, Template: "PLAY_USING_WAY", Args: []any{Seat(p), card, Name(how.way)}})
	case how.by != 0 || how.blank: // each of Mastermind's three plays too
		g.Log.add(Entry{Level: level, Template: "PLAY_WITH", Args: []any{Seat(p), card, Name(how.by)}})
	case how.line != "":
		g.Log.add(Entry{Level: level, Template: how.line, Args: []any{Seat(p), card}})
	case g.isTreasure(x) && !d.Action() && !d.Rules.Alone && (level == 0 && how.times == 0 && g.T2.Phase == BuyPhase || how.merge):
		g.Log.treasure(p, g.cards[x], coins, level, d.Rules.Coins == 0 && d.Rules.Worth == nil && coins == 0) // a Relic has a line of its own
	case g.isTreasure(x) && !d.Action() && !d.Rules.Alone && (d.Rules.Coins != 0 || d.Rules.Worth != nil || coins != 0): // played by another card: "plays a Copper again. (+$1)"; "plays a Potion."
		t := [...]string{"PLAY_TREASURES_FOR", "PLAY_TREASURES_AGAIN_FOR", "PLAY_TREASURES_THIRD_FOR"}[min(how.times, 2)]
		g.Log.add(Entry{Level: level, Template: t, Args: []any{Seat(p), card, coins}})
	case g.isTreasure(x) && !d.Action() && level == 0 && how.times == 0 && g.T2.Phase == BuyPhase: // a Treasure with a choice
		g.Log.playAlone(Entry{Level: level, Template: "PLAY", Args: []any{Seat(p), card}})
	case how.times == 1:
		g.Log.add(Entry{Level: level, Template: "PLAY_AGAIN", Args: []any{Seat(p), card}})
	case how.times >= 2: // a fourth Infirmary is "a third time" too
		g.Log.add(Entry{Level: level, Template: "PLAY_THIRD", Args: []any{Seat(p), card}})
	default:
		g.Log.add(Entry{Level: level, Template: "PLAY", Args: []any{Seat(p), card}})
	}
	if null {
		g.Log.flush()
		g.Log.add(Entry{Level: 3, Template: "PLAY_NULLIFIED", Args: []any{Seat(p), Name(g.cards[x])}})
		g.T2.Played = append(g.T2.Played, x)
		return
	}
	g.T2.Played = append(g.T2.Played, x)
	if d.Action() {
		g.T2.ActionsIn++
		g.tokenBonus(p, x, level+1)
	}
	playing := &Ev{Kind: EvPlaying, P: p, X: x, C: g.cards[x], Level: level + 1}
	g.fire(playing)
	if !playing.Skip && how.way == 0 && !how.asked && d.Action() && (level > 0 || how.times > 0 || how.by != 0 || how.blank) {
		how.way = g.offerWay(p, x, level+1, "USES_WAY")
	}
	if playing.Skip {
		g.Log.releaseWay() // nothing of its own
	} else if g.T2.Phase == ActionPhase && p == g.Cur && how.times == 0 && how.way == 0 && g.CameTrue("Enlightenment") && g.isTreasure(x) && !d.Action() {
		// instead of its instructions, two levels down: "Copper plays Enlightenment for +1 Card +1 Action"
		g.Log.add(Entry{Level: level + 2, Template: "PLAY_ENLIGHTENED", Args: []any{Name(g.cards[x]), Name(g.Prophecy)}})
		e := g.ctx(p, x, level+3)
		e.Draw(1)
		e.PlusActions(1)
	} else if how.way != 0 {
		if d.Attack() { // still an Attack played: "plays an Urchin.", "trashes an Urchin.", then "uses Way of the Owl."
			g.fire(&Ev{Kind: EvAttack, P: p, X: x, C: g.cards[x], Victim: 1 - p, Level: level + 1})
		}
		if g.Log.releaseWay() {
			g.Log.add(Entry{Level: level + 1, Template: "USES_WAY", Args: []any{Seat(p), Name(how.way)}})
		}
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
		if coins > 0 && g.P[p].MinusCoin != 0 { // "plays a Copper. (+$1)", "returns -Coin token set by Bridge Troll. (-$1)"
			g.Log.add(Entry{Level: level + 1, Template: "RETURNS_MINUS_COIN_TOKEN", Args: []any{Seat(p), "-Coin token", Name(g.P[p].MinusCoin)}})
			g.P[p].MinusCoin = 0
			coins--
		}
		g.P[p].Coins += coins
		reckless := g.T.Defs[g.TraitOf(g.cards[x])].Name == "Reckless"
		if reckless { // its instructions twice, a level down: "plays a Laboratory.", "draws 2 Coppers." at 2
			c.level++
			g.P[p].Coins += coins
		}
		if d.Rules.Play != nil {
			d.Rules.Play(c)
			if reckless {
				d.Rules.Play(c)
			}
		}
		if g.inherits(p, x) { // "plays an Estate.", "plays an Alley."
			g.play(p, g.P[p].Inherited, level+1, playHow{inPlace: true, self: x, holders: how.holders})
		}
	}
	g.fire(&Ev{Kind: EvPlay, P: p, X: x, C: g.cards[x], By: how.by, Level: level + 1})
}

// IsAction: x is an Action card, or an Estate of a player with the Estate token (Inheritance).
func (g *Game) IsAction(x Inst) bool {
	if g.Def(x).Action() {
		return true
	}
	p := g.Owner(x)
	return g.cards[x] == g.estate && p >= 0 && g.P[p].Inherited >= 0
}

// inherits: x is an Estate that plays the card with p's Estate token on p's turn.
func (g *Game) inherits(p int, x Inst) bool {
	return g.cards[x] == g.estate && g.P[p].Inherited >= 0 && g.Cur == p
}

// tokenNames are how the log shows the Adventures tokens, by Tok constant.
var tokenNames = [nTokens]string{"+Card token", "+Action token", "+Buy token", "+Coin token", "-Cost token", "Trashing token", "Estate token"}

// tokenBonus gives what the player's tokens on the played card's pile give first:
// "gets +1 Action (from Lost Arts)", "draws a Copper (from Teacher)".
func (g *Game) tokenBonus(p int, x Inst, level int) {
	pl := &g.P[p]
	i := g.PileOf(g.cards[x])
	if i < 0 {
		return
	}
	for k := TokCard; k <= TokCoin; k++ {
		if pl.Tokens[k] != i {
			continue
		}
		by := Name(pl.TokenBy[k])
		switch k {
		case TokCard:
			g.drawAs(p, 1, level, "DRAWS_FROM", pl.TokenBy[k])
		case TokAction:
			pl.Actions++
			g.Log.add(Entry{Level: level, Template: "PLUS_ACTION_FROM", Args: []any{Seat(p), by}})
		case TokBuy:
			pl.Buys++
			g.Log.add(Entry{Level: level, Template: "PLUS_BUY_FROM", Args: []any{Seat(p), by}})
		case TokCoin:
			if pl.MinusCoin != 0 {
				g.Log.add(Entry{Level: level, Template: "RETURNS_MINUS_COIN_TOKEN", Args: []any{Seat(p), "-Coin token", Name(pl.MinusCoin)}})
				pl.MinusCoin = 0
				continue
			}
			pl.Coins++
			g.Log.add(Entry{Level: level, Template: "PLUS_COIN_FROM", Args: []any{Seat(p), by}})
		}
	}
}

// alsoTreasure: a Project makes the card a Treasure for player p (Capitalism).
func (g *Game) alsoTreasure(p int, d *Def) bool {
	for _, c := range g.Land {
		if r := g.T.Defs[c].Rules; r != nil && r.AlsoTreasure != nil && r.AlsoTreasure(g, p, d) {
			return true
		}
	}
	return false
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
	if n > 1 && g.T2.Envious && p == g.Cur && (g.cards[x] == g.silver || g.cards[x] == g.gold) {
		return 1
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
	if d.Action() {
		off += g.T2.ReduceAct
	}
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
