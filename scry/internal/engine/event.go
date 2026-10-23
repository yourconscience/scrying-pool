package engine

// EvKind is something that happens that cards may respond to.
type EvKind uint8

const (
	EvGain      EvKind = iota // P gains X, to To
	EvBuy                     // P buys C (X: the card about to be gained, or -1 for an Event or Project)
	EvTrash                   // P's card X is trashed, from From
	EvDiscard                 // P discards X from From, other than in Clean-up; or from play in Clean-up (Cleanup)
	EvPlay                    // P played X; its effect is done
	EvAttack                  // P plays the Attack X; ev.Victim may react
	EvTurnStart               // the start of P's turn
	EvBuyStart                // the start of P's Buy phase
	EvBuyEnd                  // the end of P's Buy phase
	EvCleanup                 // the start of P's Clean-up
	EvTurnEnd                 // the end of P's turn
	EvShuffle                 // P shuffled
	EvReveal                  // P reveals X
	nEvKinds
)

// EvMask is a set of EvKinds.
type EvMask uint32

func On(ks ...EvKind) EvMask {
	var m EvMask
	for _, k := range ks {
		m |= 1 << k
	}
	return m
}

func (m EvMask) has(k EvKind) bool { return m&(1<<k) != 0 }

// Ev is one event. Listeners may change it: a gain's To, an Attack's Unaffected.
type Ev struct {
	Kind    EvKind
	P       int    // the player it happens to
	X       Inst   // the card, -1 for none
	C       CardID // its card kind
	From    Zone   // where a trashed or discarded card came from
	To      Zone   // where a gained card went
	By      CardID // the card that caused it; 0 for a buy or the turn itself
	Bought  bool   // the gain of a bought card
	Victim  int    // EvAttack: the player attacked
	Level   int    // the nesting of lines its responses write
	Cleanup bool   // EvDiscard in Clean-up

	Unaffected bool // EvAttack: a Reaction protects the victim
	Done       bool // the card was moved already: a later listener can't move it
}

// Watch is an effect a card leaves for later: Merchant's +$1 for the next
// Silver, Priest's +$2 for each trash. Its Rules.Watch runs for the events in On.
type Watch struct {
	P    int
	Src  Inst   // the card that made it, -1 for none
	C    CardID // whose Rules.Watch runs
	On   EvMask
	N    int  // what it counts
	Long bool // it lasts past this turn's Clean-up, until used up
}

// Watches are the effects waiting for later.
func (g *Game) Watches() []Watch { return g.watches }

// AddWatch leaves an effect for later this turn.
func (c *Ctx) AddWatch(on EvMask, n int) *Watch {
	c.G.watches = append(c.G.watches, Watch{P: c.P, Src: c.Me, C: c.Src, On: on, N: n})
	c.G.hears |= on
	return &c.G.watches[len(c.G.watches)-1]
}

// listenZones are where a card hears events: not in hand (that's React), the deck or the discard pile.
var listenZones = []Zone{InPlay, Aside, Tavern, Exile, Island, NativeVillage}

// fire runs everything that responds to an event: the card's own "when you
// ..." rule, Reactions in hand, cards that hear it, landscapes, and watches.
func (g *Game) fire(ev *Ev) {
	if ev.X >= 0 {
		if r := g.Def(ev.X).Rules; r != nil {
			var h func(*Ctx, *Ev)
			switch ev.Kind {
			case EvGain:
				h = r.OnGain
			case EvBuy:
				h = r.OnBuy
			case EvTrash:
				h = r.OnTrash
			case EvDiscard:
				h = r.OnDiscard
			}
			if h != nil {
				h(g.ctx(ev.P, ev.X, ev.Level), ev)
			}
		}
	}
	if !g.hears.has(ev.Kind) {
		return
	}
	order := [2]int{g.Cur, 1 - g.Cur}
	for _, q := range order {
		g.react(q, ev)
	}
	for _, q := range order {
		for _, z := range listenZones {
			zone := g.P[q].Z[z]
			for i := 0; i < len(zone); i++ {
				x := zone[i]
				r := g.Def(x).Rules
				if r == nil || r.Hear == nil || !r.Hears.has(ev.Kind) {
					continue
				}
				cx := g.ctx(q, x, ev.Level)
				cx.from = true // what a card does away from its play names it: "draws a card. (Monkey)"
				r.Hear(cx, ev)
				zone = g.P[q].Z[z]
			}
		}
	}
	for _, c := range g.Land {
		if r := g.T.Defs[c].Rules; r != nil && r.Hear != nil && r.Hears.has(ev.Kind) {
			cx := g.ctx(ev.P, -1, ev.Level)
			cx.Src = c
			r.Hear(cx, ev)
		}
	}
	for _, h := range g.Held {
		if r := g.T.Defs[h.C].Rules; r != nil && r.Hear != nil && r.Hears.has(ev.Kind) {
			cx := g.ctx(h.P, -1, ev.Level)
			cx.Src = h.C
			r.Hear(cx, ev)
		}
	}
	for i := 0; i < len(g.watches); i++ {
		w := g.watches[i]
		if !w.On.has(ev.Kind) {
			continue
		}
		cx := g.ctx(w.P, w.Src, ev.Level)
		cx.Src = w.C
		if g.T.Defs[w.C].Rules.Watch(cx, ev, &g.watches[i]) {
			g.watches = append(g.watches[:i], g.watches[i+1:]...)
			i--
		}
	}
}

// react lets player q reveal Reactions from hand to an event, one at a time.
func (g *Game) react(q int, ev *Ev) {
	var used []Inst
	for {
		var opts []Option
		for _, x := range g.P[q].Z[Hand] {
			r := g.Def(x).Rules
			if r == nil || r.React == nil || !r.Reacts.has(ev.Kind) || contains(used, x) {
				continue
			}
			if r.CanReact != nil && !r.CanReact(g.ctx(q, x, ev.Level), ev) {
				continue
			}
			opts = append(opts, Option{Card: g.cards[x], Inst: x})
		}
		if len(opts) == 0 {
			return
		}
		got := g.ask(&Decision{Player: q, Kind: ToReveal, Source: ev.C, Options: opts, Min: 0, Max: 1, Level: ev.Level})
		if len(got) == 0 {
			return
		}
		x := got[0].Inst
		used = append(used, x)
		g.Def(x).Rules.React(g.ctx(q, x, ev.Level), ev)
	}
}

func contains(xs []Inst, x Inst) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// listening collects which events any card in the game responds to, so
// fire can skip the rest.
func (g *Game) listening() {
	g.hears = 0
	seen := map[CardID]bool{}
	for _, c := range g.cards {
		if seen[c] {
			continue
		}
		seen[c] = true
		if r := g.T.Defs[c].Rules; r != nil {
			g.hears |= r.Hears | r.Reacts
		}
	}
	for _, c := range g.Land {
		if r := g.T.Defs[c].Rules; r != nil {
			g.hears |= r.Hears
		}
	}
	for _, d := range g.T.Defs {
		if d.Is(TState|TArtifact) && d.Rules != nil {
			g.hears |= d.Rules.Hears
		}
	}
}

// ctx makes a card's effect context.
func (g *Game) ctx(p int, x Inst, level int) *Ctx {
	c := &Ctx{G: g, P: p, Me: x, level: level}
	if x >= 0 {
		c.Src = g.cards[x]
	}
	return c
}
