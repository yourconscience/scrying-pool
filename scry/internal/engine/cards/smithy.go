package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Smithy: +3 Cards.
func init() {
	engine.Register("Smithy", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(3)
	}})
}
