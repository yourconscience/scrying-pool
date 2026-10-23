package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Moat: +2 Cards. When another player plays an Attack card, you may first
// reveal this from your hand, to be unaffected by it.
func init() {
	engine.Register("Moat", engine.Rules{
		Play:  func(c *engine.Ctx) { c.Draw(2) },
		React: func(*engine.Ctx, engine.Inst) bool { return true },
	})
}
