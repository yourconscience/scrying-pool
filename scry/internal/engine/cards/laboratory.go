package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Laboratory: +2 Cards, +1 Action.
func init() {
	engine.Register("Laboratory", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(2)
		c.PlusActions(1)
	}})
}
