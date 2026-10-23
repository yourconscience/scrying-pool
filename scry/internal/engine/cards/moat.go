package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Moat", engine.Rules{
		Play:  func(c *engine.Ctx) { c.Draw(2) },
		React: func(*engine.Ctx, engine.Inst) bool { return true },
	})
}
