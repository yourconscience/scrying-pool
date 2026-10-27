package engine

import (
	"fmt"
	"slices"
)

// Ctx is a card's effect as it runs: whose card it is, and how deep its log
// lines nest. Card code is straight-line Go over these calls; each move logs
// the line dominion.games would.
type Ctx struct {
	G     *Game
	P     int    // the player whose card it is
	Me    Inst   // the card, -1 for a landscape, Boon or State
	Src   CardID // the card whose rules run: Me's card, a Way, a landscape
	level int    // log nesting of the effect's lines

	// InPlace: the card was played where it is and left there (Overlord, Inheritance), so
	// moving "this" fails; Me is the card that played it, which stays in play for a Duration
	InPlace bool

	from       bool    // the effect happens away from the card's play (a Duration's next turn): lines name the card
	name       CardID  // with from, the card +Cards, +Actions, +Buys and +$ lines name instead of Src
	swap       bool    // Way of the Chameleon: its player's +Cards give +$ instead, and +$ give +Cards
	holders    []Inst  // the cards that played this one (Throne Room), which stay in play with it
	unaffected [2]bool // players a Reaction protected from this Attack
	moved      []Zone  // where the cards the last moveAll moved came from
}

func (c *Ctx) Other() int { return 1 - c.P }

// Level is the log nesting of the effect's lines.
func (c *Ctx) Level() int { return c.level }

// Deeper is the same effect one log level down, e.g. for what a gain causes.
func (c *Ctx) Deeper() *Ctx {
	d := *c
	d.level++
	return &d
}

// At is the same effect at another log level: a Royal Carriage called on the line of the play it follows.
func (c *Ctx) At(level int) *Ctx {
	d := *c
	d.level = level
	return &d
}

// As is the effect for another player, e.g. an Attack's victim gaining a Curse
// that then triggers their own cards.
func (c *Ctx) As(p int) *Ctx {
	d := *c
	d.P = p
	return &d
}

// Here is the same effect with lines that don't name the card: Urchin's "trashes an Urchin" when another Attack is played.
func (c *Ctx) Here() *Ctx {
	d := *c
	d.from = false
	return &d
}

// Away is the same effect named on its lines, as away from the card's play: "gets 2 VP from Collection".
func (c *Ctx) Away() *Ctx {
	d := *c
	d.from = true
	return &d
}

// Hand is a player's hand.
func (c *Ctx) Hand(p int) []Inst { return c.G.P[p].Z[Hand] }

// Zone is a player's cards in a zone.
func (c *Ctx) Zone(p int, z Zone) []Inst { return c.G.P[p].Z[z] }

// Pl is a player's state.
func (c *Ctx) Pl(p int) *Player { return &c.G.P[p] }

// Name is the card the effect is from.
func (c *Ctx) Name() string { return c.G.T.Defs[c.Src].Name }

// Card finds a card kind by name.
func (c *Ctx) Card(name string) CardID { return c.G.T.MustCard(name) }

// Log writes a line at the effect's level.
func (c *Ctx) Log(template string, args ...any) {
	c.G.Log.add(Entry{Level: c.level, Template: template, Args: args})
}

// from picks a template's "(Card)" variant when the effect is away from the card's play.
func (c *Ctx) withFrom(template string, args []any) (string, []any) {
	if c.from {
		return template + "_FROM", append(args, Name(c.named()))
	}
	return template, args
}

// named is the card +Cards and the like name away from the card's play.
func (c *Ctx) named() CardID {
	if c.name != 0 {
		return c.name
	}
	return c.Src
}

// Receiving is the effect of a Boon or Hex b that this card's effect gives player p.
// Away from the card's play (at the start of a turn) gains and most moves name the
// Boon, but +Cards, +Buys and +$ name the card: "gets +1 Buy. (Blessed Village)",
// "gains a Silver. (The Mountain's Gift)".
func (c *Ctx) Receiving(p int, b CardID) *Ctx {
	d := *c
	d.P, d.Me, d.Src, d.InPlace = p, -1, b, false
	if d.from && d.name == 0 {
		d.name = c.Src
	}
	return &d
}

// Swapped is the effect with +Cards and +$ traded, for Way of the Chameleon.
func (c *Ctx) Swapped() *Ctx {
	d := *c
	d.swap = true
	return &d
}

// Draw: +Cards.
func (c *Ctx) Draw(n int) []Inst { return c.DrawFor(c.P, n) }

// DrawFor draws cards for any player, e.g. Council Room's other player.
func (c *Ctx) DrawFor(p, n int) []Inst {
	if c.swap && p == c.P {
		d := *c
		d.swap = false
		d.PlusCoins(n)
		return nil
	}
	var from CardID
	if c.from {
		from = c.named()
	}
	return c.G.draw(p, n, c.level, from)
}

func (c *Ctx) PlusActions(n int) {
	if c.G.T2.NoActions && c.P == c.G.Cur { // Snowy Village: silently
		return
	}
	c.G.P[c.P].Actions += n
	t, a := c.withFrom(plural(n, "GETS_ACTION", "GETS_ACTIONS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

func (c *Ctx) PlusBuys(n int) {
	c.G.P[c.P].Buys += n
	t, a := c.withFrom(plural(n, "GETS_BUY", "GETS_BUYS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

func (c *Ctx) PlusCoins(n int) {
	if c.swap {
		d := *c
		d.swap = false
		d.Draw(n)
		return
	}
	pl := &c.G.P[c.P]
	if n > 0 && pl.MinusCoin != 0 { // "returns -Coin token set by Bridge Troll. (-$1)"
		c.Log("RETURNS_MINUS_COIN_TOKEN", Seat(c.P), "-Coin token", Name(pl.MinusCoin))
		pl.MinusCoin = 0
		n--
		if n == 0 {
			return
		}
	}
	pl.Coins += n
	t, a := c.withFrom(plural(n, "GETS_COIN", "GETS_COINS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

// CoinsFromMerged is CoinsFrom added to the line this card just wrote at this level: two Priests'
// +$2 for one trash, or one Priest's for "trashes 2 Coppers", are "gets +$4. (Priest)".
func (c *Ctx) CoinsFromMerged(n int) {
	l, pl := c.G.Log, &c.G.P[c.P]
	if l != nil && l.pending == nil && l.tr == nil && len(l.after) == 0 && len(l.Entries) > 0 && pl.MinusCoin == 0 {
		e := &l.Entries[len(l.Entries)-1]
		if (e.Template == "GETS_COIN_FROM" || e.Template == "GETS_COINS_FROM") && e.Level == c.level && e.Args[0] == Seat(c.P) && e.Args[2] == Name(c.Src) {
			pl.Coins += n
			e.Args[1] = e.Args[1].(int) + n
			e.Template = plural(e.Args[1].(int), "GETS_COIN", "GETS_COINS") + "_FROM"
			l.changed(len(l.Entries) - 1)
			return
		}
	}
	c.CoinsFrom(n)
}

// CoinsFrom is +$ that a card gives later, e.g. Merchant's for the first Silver.
func (c *Ctx) CoinsFrom(n int) {
	d := *c
	d.from = true
	d.PlusCoins(n)
}

func (c *Ctx) PlusPotions(n int) { c.G.P[c.P].Potions += n }

func (c *Ctx) PlusCoffers(n int) {
	c.G.P[c.P].Coffers += n
	t, a := c.withFrom(plural(n, "GETS_COFFER", "GETS_COFFERS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

func (c *Ctx) PlusVillagers(n int) {
	c.G.P[c.P].Villagers += n
	t, a := c.withFrom(plural(n, "GETS_VILLAGER", "GETS_VILLAGERS"), []any{Seat(c.P), n})
	c.Log(t, a...)
}

// PlusFavors: +Favors, whose line always names the card: "gets +1 Favor. (Underling)".
func (c *Ctx) PlusFavors(n int) {
	c.G.P[c.P].Favors += n
	c.Log(plural(n, "GETS_FAVOR", "GETS_FAVORS")+"_FROM", Seat(c.P), n, Name(c.named()))
}

// SpendFavors spends Favors: "spends 2 Favors."
func (c *Ctx) SpendFavors(n int) {
	c.G.P[c.P].Favors -= n
	c.Log(plural(n, "SPENDS_FAVOR", "SPENDS_FAVORS"), Seat(c.P), n)
}

// Rotate moves the cards of a pile's top card to its bottom: "rotates 2 Town Criers."
func (c *Ctx) Rotate(i int) {
	s := c.G.Piles[i].Cards
	k := len(s)
	for k > 0 && c.G.cards[s[k-1]] == c.G.cards[s[len(s)-1]] {
		k--
	}
	top := slices.Clone(s[k:])
	copy(s, append(top, s[:k]...))
	c.Log("ROTATES", Seat(c.P), c.G.ids(top))
}

// PlusVP gives VP tokens.
func (c *Ctx) PlusVP(n int) {
	c.G.P[c.P].VP += n
	if c.from {
		c.Log("PLAYER_GETS_VP_FROM", Seat(c.P), n, Name(c.Src))
	} else {
		c.Log("PLAYER_GETS_VP", Seat(c.P), n)
	}
}

// TakeDebt takes debt tokens.
func (c *Ctx) TakeDebt(n int) { c.G.takeDebt(c.P, n, c.level) }

// AddVPTo puts VP tokens on a pile: "adds 1 VP to the Farmers' Market pile".
func (c *Ctx) AddVPTo(pile, n int) {
	c.G.Piles[pile].VP += n
	c.Log("ADDS_VP_TO", Seat(c.P), n, PileName(c.G.Piles[pile].Card))
}

// TakeVPFrom takes the VP tokens on a pile: "takes 4 VP from the Farmers' Market pile".
func (c *Ctx) TakeVPFrom(pile int) int {
	n := c.G.Piles[pile].VP
	if n > 0 {
		c.G.Piles[pile].VP = 0
		c.G.P[c.P].VP += n
		c.Log("TAKE_VP_FROM", Seat(c.P), n, PileName(c.G.Piles[pile].Card))
	}
	return n
}

// TakeLandVP takes up to n VP tokens from this Landmark: "takes 2 VP from Arena".
func (c *Ctx) TakeLandVP(n int) {
	if n = min(n, c.G.LandVP[c.Src]); n > 0 {
		c.G.LandVP[c.Src] -= n
		c.G.P[c.P].VP += n
		c.Log("TAKE_VP_FROM", Seat(c.P), n, Name(c.Src))
	}
}

// MoveVPToLand moves a VP token from a pile to this Landmark (Aqueduct, Defiled Shrine).
func (c *Ctx) MoveVPToLand(pile int) {
	if c.G.Piles[pile].VP > 0 {
		c.G.Piles[pile].VP--
		c.G.LandVP[c.Src]++
		c.Log("MOVE_VP_FROM_TO", Seat(c.P), 1, PileName(c.G.Piles[pile].Card), Name(c.Src))
	}
}

// Quietly writes what fn logs without first writing the waiting lazy line:
// Capital's debt in Clean-up comes without "begins their Cleanup phase".
func (c *Ctx) Quietly(fn func()) {
	l := c.G.Log
	if l == nil {
		fn()
		return
	}
	p := l.pending
	l.pending = nil
	fn()
	if l.pending == nil {
		l.pending = p
	}
}

// ShuffleLine writes "shuffles their deck." with nothing to shuffle, as the
// client does before Donate shuffles a hand into the deck.
func (c *Ctx) ShuffleLine(p int) {
	c.G.Log.add(Entry{Level: c.level, Kind: 'S', Template: "SHUFFLES", Args: []any{Seat(p)}})
}

// PlayHere plays a card where it is, leaving it there, with this card as its
// "this": Overlord's card from the Supply.
func (c *Ctx) PlayHere(x Inst) {
	c.G.play(c.P, x, c.level, playHow{inPlace: true, self: c.Me, holders: c.holders})
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Choose asks player p to pick between min and max of the options.
func (c *Ctx) Choose(p int, k Kind, opts []Option, min, max int) []Option {
	return c.G.ask(&Decision{Player: p, Kind: k, Source: c.Src, Options: opts, Min: min, Max: max, Level: c.level})
}

// ChooseOne asks for one option, or none when may is true; nil when there is none.
func (c *Ctx) ChooseOne(p int, k Kind, opts []Option, may bool) *Option {
	min := 1
	if may {
		min = 0
	}
	got := c.Choose(p, k, opts, min, 1)
	if len(got) == 0 {
		return nil
	}
	return &got[0]
}

// Mode is one way a card can be used: its label, and the template of the
// first log line it writes, so a replay can tell which one a log chose.
type Mode struct {
	Label string
	Line  string
	Count int // how many cards the line draws, when modes share a line (Blacksmith); 0 for any
}

// Modes asks player p to pick between min and max of a card's modes; it returns their indexes.
func (c *Ctx) Modes(p, min, max int, ms ...Mode) []int {
	opts := make([]Option, len(ms))
	for i, m := range ms {
		opts[i] = Option{Inst: -1, Mode: i, Label: m.Label, Line: m.Line, Count: m.Count}
	}
	got := c.Choose(p, ToChoose, opts, min, max)
	out := make([]int, len(got))
	for i, o := range got {
		out[i] = o.Mode
	}
	return out
}

// Mode1 asks for one mode.
func (c *Ctx) Mode1(p int, ms ...Mode) int { return c.Modes(p, 1, 1, ms...)[0] }

// Number asks player p for a number from lo to hi.
func (c *Ctx) Number(p, lo, hi int, line string) int {
	var opts []Option
	for n := lo; n <= hi; n++ {
		opts = append(opts, Option{Inst: -1, Mode: n, Label: fmt.Sprint(n), Line: line})
	}
	return c.Choose(p, ToChoose, opts, 1, 1)[0].Mode
}

// Amount asks player p how much to spend, from lo to hi: none writes nothing,
// one writes the line one ("loses 1 Coin."), more the line many.
func (c *Ctx) Amount(p, lo, hi int, one, many string) int {
	var opts []Option
	for n := lo; n <= hi; n++ {
		line := many
		switch n {
		case 0:
			line = ""
		case 1:
			line = one
		}
		opts = append(opts, Option{Inst: -1, Mode: n, Label: fmt.Sprint(n), Line: line})
	}
	return c.Choose(p, ToChoose, opts, 1, 1)[0].Mode
}

// Yes asks player p whether to do something optional; line is what doing it writes first.
func (c *Ctx) Yes(p int, label, line string) bool {
	return c.Mode1(p, Mode{Label: "no"}, Mode{Label: label, Line: line}) == 1
}

// NameCard asks player p to name a card, from all the cards that could be in
// a deck; the caller logs it (NAMES, or Wishing Well's own line).
func (c *Ctx) NameCard(p int) CardID {
	var opts []Option
	for _, pile := range c.G.Piles {
		for _, id := range pile.kinds(c.G) {
			opts = append(opts, Option{Card: id, Inst: -1})
		}
	}
	return c.Choose(p, ToName, opts, 1, 1)[0].Card
}

// Options lists a player's cards in a zone that pass a test (nil: all).
func (c *Ctx) Options(p int, z Zone, ok func(*Def) bool) []Option { return c.G.options(p, z, ok) }

// Cards turns options into the instances they are.
func Cards(opts []Option) []Inst {
	out := make([]Inst, len(opts))
	for i, o := range opts {
		out[i] = o.Inst
	}
	return out
}

// OptionsOf makes options of instances.
func (c *Ctx) OptionsOf(xs []Inst) []Option {
	out := make([]Option, len(xs))
	for i, x := range xs {
		out[i] = Option{Card: c.G.cards[x], Inst: x}
	}
	return out
}

// In keeps the cards that are still in a zone.
func (c *Ctx) In(z Zone, xs []Inst) []Inst {
	var out []Inst
	for _, x := range xs {
		if c.G.loc[x].Zone == z {
			out = append(out, x)
		}
	}
	return out
}

// SupplyOptions lists the non-empty Supply piles whose top card passes a test.
func (c *Ctx) SupplyOptions(ok func(*Def) bool) []Option {
	var out []Option
	for _, p := range c.G.Piles {
		if p.Supply && len(p.Cards) > 0 {
			top := c.G.cards[p.Cards[len(p.Cards)-1]]
			if ok == nil || ok(c.G.T.Defs[top]) {
				out = append(out, Option{Card: top, Inst: -1})
			}
		}
	}
	return out
}

// CostsUpTo tests for a card costing at most $n (no Potion, no debt).
func (c *Ctx) CostsUpTo(n int) func(*Def) bool { return c.CostsAtMost(Upto(n)) }

// CostsAtMost tests for a card costing at most a cost.
func (c *Ctx) CostsAtMost(m Cost) func(*Def) bool {
	return func(d *Def) bool { return c.G.CostOf(d.ID).LE(m) }
}

// CostOf is a card kind's cost now.
func (c *Ctx) CostOf(id CardID) Cost { return c.G.CostOf(id) }

// Gain gains a card from its pile to a zone, logged as "gains"; -1 if it can't.
func (c *Ctx) Gain(p int, card CardID, to Zone) Inst {
	return c.GainAs(p, card, to, "GAIN")
}

// GainAs gains a card with a template of its own, e.g. GAIN_TO_HAND.
func (c *Ctx) GainAs(p int, card CardID, to Zone, template string) Inst {
	i := c.G.PileOf(card)
	if i < 0 || c.G.Top(i) != card {
		return -1
	}
	s := c.G.Piles[i].Cards
	x := s[len(s)-1]
	c.logGain(p, x, template)
	c.G.gainX(p, x, to, c.level+1, c.Src, false)
	return x
}

func (c *Ctx) logGain(p int, x Inst, template string) {
	args := []any{Seat(p), []CardID{c.G.cards[x]}}
	if c.from && template == "GAIN" {
		c.Log("GAIN_WITH", append(args, Name(c.Src))...)
		return
	}
	c.Log(template, args...)
}

// GainN gains up to n cards from a pile, logged as one line unless what a gain
// causes comes between: "gains 4 Golds onto their drawpile". A mixed pile's
// card (Ruins) gains whatever is on top: "gains a Ruined Library and a Survivors".
func (c *Ctx) GainN(p int, card CardID, n int, to Zone, template string) []Inst {
	i := c.G.PileOf(card)
	var xs []Inst
	seen := -1
	for i >= 0 && len(xs) < n { // the top card each time: a Duplicate's Silver between them comes off the pile too
		s := c.G.Piles[i].Cards
		if len(s) == 0 || c.G.cards[s[len(s)-1]] != card && c.G.Piles[i].Card != card {
			break
		}
		x := s[len(s)-1]
		if c.G.Log.size() == seen { // nothing logged since the last gain: "gains 2 Silvers"
			c.G.Log.extendLast(c.G.cards[x])
		} else { // a Footpad's draw between them: "gains a Silver", "draws a card", "gains a Silver"
			c.Log(template, Seat(p), []CardID{c.G.cards[x]})
		}
		seen = c.G.Log.size()
		xs = append(xs, x)
		c.G.gainX(p, x, to, c.level+1, c.Src, false)
	}
	return xs
}

// GainInst gains a particular card, e.g. one from the trash.
func (c *Ctx) GainInst(p int, x Inst, to Zone, template string) {
	c.logGain(p, x, template)
	c.G.gainX(p, x, to, c.level+1, c.Src, false)
}

// ChooseGain asks player p to gain a card from the Supply that passes a test.
func (c *Ctx) ChooseGain(p int, ok func(*Def) bool, to Zone) Inst {
	o := c.ChooseOne(p, ToGain, c.SupplyOptions(ok), false)
	if o == nil {
		return -1
	}
	return c.Gain(p, o.Card, to)
}

// gainX moves a card to a player's zone as a gain and runs what the gain
// causes; the caller logged it.
func (g *Game) gainX(p int, x Inst, to Zone, level int, by CardID, bought bool) {
	from := g.loc[x]
	if !g.move(x, from.Zone, Loc{to, int16(p)}) {
		return
	}
	if from.Zone == Supply {
		if pile := &g.Piles[from.Owner]; pile.Trade {
			pile.Trade = false
			g.TradeRoute++
		}
	}
	if p == g.Cur {
		g.T2.Gained = append(g.T2.Gained, g.cards[x])
		if g.T2.Phase == BuyPhase {
			g.T2.BuyGained = append(g.T2.BuyGained, g.cards[x])
		}
	}
	if from.Zone == Supply && g.P[p].Tokens[TokTrash] == int(from.Owner) { // Plan's Trashing token: "buys and gains a Fortress.", "trashes a Copper."
		cx := g.ctx(p, -1, level)
		cx.Trash(p, Cards(cx.Choose(p, ToTrash, cx.Options(p, Hand, nil), 0, 1)))
	}
	if ex := g.copies(p, Exile, g.cards[x]); len(ex) > 0 {
		c := g.ctx(p, x, level)
		if c.Yes(p, "discard from Exile", "DISCARD_FROM_EXILE") {
			for _, y := range ex {
				g.move(y, Exile, Loc{Discard, int16(p)})
			}
			c.Log("DISCARD_FROM_EXILE", Seat(p), g.ids(ex))
		}
	}
	g.fire(&Ev{Kind: EvGain, P: p, X: x, C: g.cards[x], To: to, By: by, Bought: bought, Level: level})
}

func (g *Game) copies(p int, z Zone, c CardID) []Inst {
	var out []Inst
	for _, y := range g.P[p].Z[z] {
		if g.cards[y] == c {
			out = append(out, y)
		}
	}
	return out
}

// moveAll moves a player's cards from their zones and logs one line for those
// that moved; a card that has left the player's zones (trashed) stays where it is.
func (c *Ctx) moveAll(p int, xs []Inst, to Loc, template string, args ...any) []Inst {
	var moved []Inst
	var from []Zone
	for _, x := range xs {
		l := c.G.loc[x]
		if c.InPlace && x == c.Me {
			continue
		}
		if l.Zone < nPlayerZones && int(l.Owner) == p && (l.Zone != to.Zone || to.Zone == Deck) && c.G.move(x, l.Zone, to) {
			moved = append(moved, x)
			from = append(from, l.Zone)
		}
	}
	if len(moved) > 0 && template != "" {
		c.Log(template, append([]any{Seat(p), c.G.ids(moved)}, args...)...)
	}
	c.moved = from
	return moved
}

// Trash trashes a player's cards and runs what trashing them causes.
func (c *Ctx) Trash(p int, xs []Inst) []Inst {
	t, a := c.with("TRASH")
	moved := c.moveAll(p, xs, Loc{Trash, 0}, t, a...)
	from := c.moved
	if p == c.G.Cur {
		c.G.T2.Trashed += len(moved)
	}
	for i, x := range moved {
		c.G.fire(&Ev{Kind: EvTrash, P: p, X: x, C: c.G.cards[x], From: from[i], By: c.Src, Level: c.level + 1})
	}
	return moved
}

// Discard discards a player's cards (not in Clean-up).
func (c *Ctx) Discard(p int, xs []Inst) []Inst {
	t, a := c.with("DISCARD")
	return c.DiscardAs(p, xs, t, a...)
}

// DiscardAs discards cards with a template of its own, e.g. DISCARD_REVEALED.
func (c *Ctx) DiscardAs(p int, xs []Inst, template string, args ...any) []Inst {
	moved := c.moveAll(p, xs, Loc{Discard, int16(p)}, template, args...)
	from := c.moved
	for i, x := range moved {
		c.G.fire(&Ev{Kind: EvDiscard, P: p, X: x, C: c.G.cards[x], From: from[i], By: c.Src, Level: c.level + 1})
	}
	return moved
}

// with is a move template's "(Card)" variant away from the card's play.
func (c *Ctx) with(template string) (string, []any) {
	if c.from {
		return template + "_WITH", []any{Name(c.Src)}
	}
	return template, nil
}

// Topdeck puts cards onto the deck; the last one ends on top.
func (c *Ctx) Topdeck(p int, xs []Inst) []Inst {
	return c.moveAll(p, xs, Loc{Deck, int16(p)}, "TOPDECK")
}

// TopdeckNamed topdecks cards "with" the card, away from its play: The Sun's Gift at the start of a turn.
func (c *Ctx) TopdeckNamed(p int, xs []Inst) []Inst {
	t, a := c.with("TOPDECK")
	return c.moveAll(p, xs, Loc{Deck, int16(p)}, t, a...)
}

// Bottomdeck puts cards on the bottom of the deck.
func (c *Ctx) Bottomdeck(p int, xs []Inst) { c.BottomdeckAs(p, xs, "BOTTOMDECKS") }

// BottomdeckAs puts cards on the bottom of the deck with a template of its own:
// "puts a Jewels on the bottom of Draw Pile" (PUT_ON_BOTTOM_OF).
func (c *Ctx) BottomdeckAs(p int, xs []Inst, template string, args ...any) {
	var moved []Inst
	for _, x := range xs {
		if c.G.Owner(x) == p && !(c.InPlace && x == c.Me) {
			c.G.bottom(x, p)
			moved = append(moved, x)
		}
	}
	if len(moved) > 0 {
		c.Log(template, append([]any{Seat(p), c.G.ids(moved)}, args...)...)
	}
}

// ToHand puts cards into a player's hand with a template, e.g. PUT_IN_HAND.
func (c *Ctx) ToHand(p int, xs []Inst, template string, args ...any) []Inst {
	return c.moveAll(p, xs, Loc{Hand, int16(p)}, template, args...)
}

// MoveTo moves a player's cards to one of their zones with a template ("" logs nothing).
func (c *Ctx) MoveTo(p int, xs []Inst, z Zone, template string, args ...any) []Inst {
	return c.moveAll(p, xs, Loc{z, int16(p)}, template, args...)
}

// SetAside sets cards aside, logged as "sets X aside".
func (c *Ctx) SetAside(p int, xs []Inst) []Inst { return c.MoveTo(p, xs, Aside, "SET_ASIDE") }

// SetAsideWith sets a card aside with this card, e.g. Library's skipped Actions.
func (c *Ctx) SetAsideWith(p int, xs ...Inst) []Inst {
	return c.MoveTo(p, xs, Aside, "SET_ASIDE_WITH", Name(c.Src))
}

// PutOnMat puts cards on one of a player's mats: "puts an Island on their Island mat".
func (c *Ctx) PutOnMat(p int, xs []Inst, z Zone) []Inst {
	return c.MoveTo(p, xs, z, "PUT_ON_MAT", z.String())
}

// ReturnTo puts a card back on its pile.
func (c *Ctx) ReturnTo(p int, x Inst) bool {
	i := c.G.PileOf(c.G.cards[x])
	l := c.G.loc[x]
	if i < 0 || l.Zone == Supply || c.InPlace && x == c.Me {
		return false
	}
	if !c.G.move(x, l.Zone, Loc{Supply, int16(i)}) {
		return false
	}
	c.Log("RETURN_TO", Seat(p), []CardID{c.G.cards[x]}, PileName(c.G.Piles[i].Card))
	return true
}

// Exchange returns a card to its pile and puts the top card of another pile in
// the discard pile, which isn't gaining it: "returns a Hermit.", "receives a Madman."
func (c *Ctx) Exchange(x Inst, card CardID) bool {
	i, j := c.G.PileOf(c.G.cards[x]), c.G.PileOf(card)
	l := c.G.loc[x]
	if i < 0 || j < 0 || len(c.G.Piles[j].Cards) == 0 || l.Zone >= nPlayerZones || int(l.Owner) != c.P || c.InPlace && x == c.Me {
		return false
	}
	c.G.move(x, l.Zone, Loc{Supply, int16(i)})
	c.Log("EXCHANGE_RETURN", Seat(c.P), []CardID{c.G.cards[x]})
	s := c.G.Piles[j].Cards
	y := s[len(s)-1]
	c.G.move(y, Supply, Loc{Discard, int16(c.P)})
	c.Log("EXCHANGE_RECEIVE", Seat(c.P), []CardID{c.G.cards[y]})
	return true
}

// Call moves a Reserve card from the Tavern mat into play: "calls a Ratcatcher".
func (c *Ctx) Call(x Inst) bool {
	if c.G.loc[x] != (Loc{Tavern, int16(c.P)}) {
		return false
	}
	c.G.move(x, Tavern, Loc{InPlay, int16(c.P)})
	c.Log("CALL", Seat(c.P), []CardID{c.G.cards[x]})
	return true
}

// FlipJourney turns the player's Journey token over; it reports whether it is now face up.
func (c *Ctx) FlipJourney() bool {
	pl := &c.G.P[c.P]
	pl.JourneyDown = !pl.JourneyDown
	t := "PLAYER_FLIPS_TOKEN_UP"
	if pl.JourneyDown {
		t = "PLAYER_FLIPS_TOKEN_DOWN"
	}
	c.Log(t, Seat(c.P), "Journey token")
	return !pl.JourneyDown
}

// MoveToken moves one of the player's tokens (TokCard, ...) to a pile: "moves +Action token to the Torturer pile".
func (c *Ctx) MoveToken(k, pile int) {
	pl := &c.G.P[c.P]
	pl.Tokens[k], pl.TokenBy[k] = pile, c.Src
	c.Log("PLAYER_MOVES_TOKEN_TO", Seat(c.P), tokenNames[k], PileName(c.G.Piles[pile].Card))
}

// MinusCardOn puts player p's -1 Card token on their deck, unless it is there: "moves -Card token to Draw Pile".
func (c *Ctx) MinusCardOn(p int) bool {
	if c.G.P[p].MinusCard != 0 {
		return false
	}
	c.G.P[p].MinusCard = c.Src
	c.Log("PLAYER_MOVES_TOKEN_TO", Seat(p), "-Card token", "Draw Pile")
	return true
}

// Emit writes the waiting "begins their Cleanup phase" line now, as dominion.games does when it asks something then.
func (c *Ctx) Emit() { c.G.Log.emit() }

// ShuffleInto shuffles cards into a player's deck: "shuffles an Inn into their deck".
func (c *Ctx) ShuffleInto(p int, xs []Inst) {
	moved := c.moveAll(p, xs, Loc{Deck, int16(p)}, "SHUFFLE_INTO_DECK")
	if len(moved) > 0 {
		d := c.G.P[p].Z[Deck]
		c.G.rng.Shuffle(len(d), func(i, j int) { d[i], d[j] = d[j], d[i] })
	}
}

// LoseCoins takes $ away: "loses 6 Coins".
func (c *Ctx) LoseCoins(n int) {
	n = min(n, c.G.P[c.P].Coins)
	if n > 0 {
		c.G.P[c.P].Coins -= n
		c.Log(plural(n, "LOSES_COIN", "LOSES_COINS"), Seat(c.P), n)
	}
}

// LookAt logs cards a player looks at without moving them.
func (c *Ctx) LookAt(p int, xs []Inst) {
	if len(xs) > 0 {
		c.Log("LOOK_AT", Seat(p), c.G.ids(xs))
	}
}

// Reveal logs cards a player reveals.
func (c *Ctx) Reveal(p int, xs []Inst) {
	if len(xs) > 0 {
		c.Log("REVEAL", Seat(p), c.G.ids(xs))
		c.revealed(p, xs)
	}
}

func (c *Ctx) RevealHand(p int) {
	if xs := c.G.P[p].Z[Hand]; len(xs) > 0 {
		c.Log("REVEAL_HAND", Seat(p), c.G.ids(xs))
		c.revealed(p, slices.Clone(xs))
	}
}

// revealed runs what revealing the cards causes, below the reveal line: "gets a Coffers from Patron."
func (c *Ctx) revealed(p int, xs []Inst) {
	for _, x := range xs {
		c.G.fire(&Ev{Kind: EvReveal, P: p, X: x, C: c.G.cards[x], Level: c.level + 1})
	}
}

// FromTop takes up to n cards off the top of a deck into Revealed, shuffling
// if needed, and logs them with a template (REVEAL, LOOK_AT; "" for none).
func (c *Ctx) FromTop(p, n int, template string) []Inst {
	var out []Inst
	for range n {
		x := c.G.top(p, Revealed, c.level)
		if x < 0 {
			break
		}
		out = append(out, x)
	}
	if len(out) > 0 && template != "" {
		c.Log(template, Seat(p), c.G.ids(out))
		if template == "REVEAL" {
			c.revealed(p, out)
		}
	}
	return out
}

// RevealUntil reveals cards from the top of a deck until one passes a test or
// the deck and discard run out; it returns the revealed cards, the found one last (or -1).
func (c *Ctx) RevealUntil(p int, ok func(*Def) bool, n int) ([]Inst, []Inst) {
	var all, found []Inst
	for len(found) < n {
		x := c.G.top(p, Revealed, c.level)
		if x < 0 {
			break
		}
		all = append(all, x)
		if ok(c.G.Def(x)) {
			found = append(found, x)
		}
	}
	if len(all) > 0 {
		c.Log("REVEAL", Seat(p), c.G.ids(all))
		c.revealed(p, all)
	}
	return all, found
}

// DrawOne draws a single card without logging it; -1 when there is none.
func (c *Ctx) DrawOne(p int) Inst { return c.G.top(p, Hand, c.level) }

// TopCard takes a deck's top card into Revealed without logging; -1 when there is none.
func (c *Ctx) TopCard(p int) Inst { return c.G.top(p, Revealed, c.level) }

// DiscardTop discards a deck's top card; -1 when there is none.
func (c *Ctx) DiscardTop(p int) Inst {
	x := c.G.top(p, Revealed, c.level)
	if x >= 0 {
		c.Discard(p, []Inst{x})
	}
	return x
}

// PlayCard plays a card from where it is for a card that plays it more than
// once (Throne Room), which stays in play with a Duration; times > 0 plays it again where it is.
func (c *Ctx) PlayCard(x Inst, times int) {
	c.G.play(c.P, x, c.level, playHow{times: times, holders: append(append([]Inst{}, c.holders...), c.Me)})
}

// PlayCardWith is PlayCard with this card named on each play line: "plays a Mining Village. (Mastermind)".
func (c *Ctx) PlayCardWith(x Inst, times int) {
	c.G.play(c.P, x, c.level, playHow{times: times, by: c.Src, holders: append(append([]Inst{}, c.holders...), c.Me)})
}

// PlayFrom plays a card with this card named on the play line ("(Prince)").
func (c *Ctx) PlayFrom(x Inst) {
	c.G.play(c.P, x, c.level-1, playHow{by: c.Src, holders: append(append([]Inst{}, c.holders...), c.Me)})
}

// PlayNow plays a card from where it is, e.g. Vassal's discarded Action or a Reaction that plays itself.
func (c *Ctx) PlayNow(x Inst) { c.G.play(c.P, x, c.level, playHow{}) }

// PlayAgain plays a card again where it is, with nothing holding it: Citadel's "plays a Settlers again."
func (c *Ctx) PlayAgain(x Inst) { c.G.play(c.P, x, c.level, playHow{times: 1}) }

// PlayAs plays a card with its own play line, e.g. REACTS_PLAYING ("reacts with a Clerk, playing it.").
func (c *Ctx) PlayAs(x Inst, template string) { c.G.play(c.P, x, c.level, playHow{line: template}) }

// PlayMerged plays a Treasure on one line with the others this card plays: "plays 2 Coppers. (+$2)" (Storyteller).
func (c *Ctx) PlayMerged(x Inst) { c.G.play(c.P, x, c.level, playHow{merge: true}) }

// PlayWith plays a card with this card named on the line ("(Sailor)").
func (c *Ctx) PlayWith(x Inst) { c.G.play(c.P, x, c.level, playHow{by: c.Src}) }

// PlayBlank plays a card with an empty name on the line: Patient's "plays a Nobles. (|)".
func (c *Ctx) PlayBlank(x Inst) { c.G.play(c.P, x, c.level, playHow{blank: true}) }

// Attack runs an Attack's effect on each other player a Reaction didn't protect.
func (c *Ctx) Attack(fn func(victim int)) {
	v := c.Other()
	if !c.unaffected[v] {
		fn(v)
	}
}

// Stay keeps this card in play for a later turn: its Next runs then.
func (c *Ctx) Stay() *Dur {
	d := &Dur{P: c.P, X: c.Me, C: c.Src, Hold: append([]Inst{}, c.holders...), Made: c.G.turns}
	c.G.durs = append(c.G.durs, d)
	return d
}

// Unaffected: a Reaction protected player p from this Attack.
func (c *Ctx) Unaffected(p int) bool { return c.unaffected[p] }

// Shuffle shuffles a player's discard pile into their deck.
func (c *Ctx) Shuffle(p int) {
	if len(c.G.P[p].Z[Discard]) > 0 {
		c.G.shuffle(p, c.level)
	}
}

// Durs are this card's Durations still waiting.
func (c *Ctx) Durs() []*Dur {
	var out []*Dur
	for _, d := range c.G.durs {
		if d.X == c.Me && c.Me >= 0 && !d.Done {
			out = append(out, d)
		}
	}
	return out
}

// Later runs a card's Next at the start of the player's next turn without the
// card staying in play (an Event's, a Boon's).
func (c *Ctx) Later() *Dur {
	d := &Dur{P: c.P, X: -1, C: c.Src, Made: c.G.turns}
	c.G.durs = append(c.G.durs, d)
	return d
}

// ExtraTurn gives the player an extra turn after this one.
func (c *Ctx) ExtraTurn() { c.G.extra = append(c.G.extra, c.Src) }

// Turn is a player's Ctx for a whole-turn fact: what they played and gained.
func (c *Ctx) T2() *turnState { return &c.G.T2 }

// Count counts a player's cards in a zone that pass a test.
func (c *Ctx) Count(p int, z Zone, ok func(*Def) bool) int {
	n := 0
	for _, x := range c.G.P[p].Z[z] {
		if ok == nil || ok(c.G.Def(x)) {
			n++
		}
	}
	return n
}

// Is tests a card kind by name.
func Is(name string) func(*Def) bool { return func(d *Def) bool { return d.Name == name } }

// Of tests for any of the types.
func Of(t Types) func(*Def) bool { return func(d *Def) bool { return d.Is(t) } }

// TrashFromSupply trashes the top card of a Supply pile, e.g. Lurker's.
func (c *Ctx) TrashFromSupply(p int, card CardID) Inst {
	i := c.G.PileOf(card)
	if i < 0 || c.G.Top(i) != card {
		return -1
	}
	s := c.G.Piles[i].Cards
	x := s[len(s)-1]
	c.G.move(x, Supply, Loc{Trash, 0})
	c.Log("TRASH", Seat(p), []CardID{card})
	c.G.fire(&Ev{Kind: EvTrash, P: p, X: x, C: card, From: Supply, By: c.Src, Level: c.level + 1})
	return x
}

// Pass moves a card from one player's hand to another's.
func (c *Ctx) Pass(from, to int, x Inst) {
	if c.G.move(x, Hand, Loc{Hand, int16(to)}) {
		c.Log("PASS", Seat(from), []CardID{c.G.cards[x]}, Seat(to))
	}
}

// Insert puts a card into a player's deck with n cards above it (0: on top).
func (c *Ctx) Insert(p int, x Inst, n int) {
	if n == 0 {
		c.Topdeck(p, []Inst{x})
		return
	}
	l := c.G.loc[x]
	if !c.G.move(x, l.Zone, Loc{Deck, int16(p)}) {
		return
	}
	d := c.G.P[p].Z[Deck]
	at := len(d) - 1 - n
	if at < 0 {
		at = 0
	}
	copy(d[at+1:], d[at:len(d)-1])
	d[at] = x
	c.Log("INSERT_IN_DECK", Seat(p), []CardID{c.G.cards[x]})
}
