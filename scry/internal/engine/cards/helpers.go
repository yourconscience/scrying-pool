// Package cards holds the card rules, one file per set. Each file registers
// its cards with the engine when the package is imported.
package cards

import (
	"fmt"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

type (
	ctx   = engine.Ctx
	ev    = engine.Ev
	inst  = engine.Inst
	def   = engine.Def
	rules = engine.Rules
)

const (
	deck     = engine.Deck
	hand     = engine.Hand
	discard  = engine.Discard
	inPlay   = engine.InPlay
	aside    = engine.Aside
	revealed = engine.Revealed
)

func reg(name string, r rules) { engine.Register(name, r) }

func worth(n int) func(*engine.Game, int) int {
	return func(*engine.Game, int) int { return n }
}

func treasure(d *def) bool { return d.Treasure() }
func action(d *def) bool   { return d.Action() }
func victory(d *def) bool  { return d.Victory() }
func attack(d *def) bool   { return d.Attack() }
func duration(d *def) bool { return d.Is(engine.TDuration) }
func any(*def) bool        { return true }

func not(f func(*def) bool) func(*def) bool { return func(d *def) bool { return !f(d) } }

func and(fs ...func(*def) bool) func(*def) bool {
	return func(d *def) bool {
		for _, f := range fs {
			if !f(d) {
				return false
			}
		}
		return true
	}
}

func named(n string) func(*def) bool { return func(d *def) bool { return d.Name == n } }

func cards(opts []engine.Option) []inst { return engine.Cards(opts) }

func one(x inst) []inst {
	if x < 0 {
		return nil
	}
	return []inst{x}
}

// pick asks for up to n cards of a zone that pass a test (at least min).
func pick(c *ctx, p int, k engine.Kind, z engine.Zone, ok func(*def) bool, min, max int) []inst {
	return cards(c.Choose(p, k, c.Options(p, z, ok), min, max))
}

// pickOf asks for between min and max of some cards.
func pickOf(c *ctx, p int, k engine.Kind, xs []inst, min, max int) []inst {
	return cards(c.Choose(p, k, c.OptionsOf(xs), min, max))
}

// pick1 asks for one card of a zone; may makes it optional. -1 for none.
func pick1(c *ctx, p int, k engine.Kind, z engine.Zone, ok func(*def) bool, may bool) inst {
	min := 1
	if may {
		min = 0
	}
	got := pick(c, p, k, z, ok, min, 1)
	if len(got) == 0 {
		return -1
	}
	return got[0]
}

// attacked: the event is an Attack on this card's owner.
func attacked(c *ctx, e *ev) bool { return e.Kind == engine.EvAttack && e.Victim == c.P }

// moat makes the owner unaffected by the Attack, revealing this card.
func moat(c *ctx, e *ev) bool {
	c.Log("REACTS_WITH", engine.Seat(c.P), []engine.CardID{c.Src})
	e.Unaffected = true
	return true
}

// order asks a player to put cards back on their deck in any order.
func topdeckAny(c *ctx, p int, xs []inst) {
	if len(xs) > 1 {
		xs = pickOf(c, p, engine.ToOrder, xs, len(xs), len(xs))
		for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
			xs[i], xs[j] = xs[j], xs[i] // the first picked goes on top, so it moves last
		}
	}
	c.Topdeck(p, xs)
}

func costUp(c *ctx, x inst, n int) func(*def) bool {
	return c.CostsAtMost(c.CostOf(c.G.CardOf(x)).Plus(n))
}

func costExactly(c *ctx, x inst, n int) func(*def) bool {
	want := c.CostOf(c.G.CardOf(x)).Plus(n)
	return func(d *def) bool { return c.CostOf(d.ID) == want }
}

func itoa(n int) string { return fmt.Sprint(n) }

// mode is one of a card's modes and the template of the line it writes first.
func mode(label, line string) engine.Mode { return engine.Mode{Label: label, Line: line} }
