package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Smithy", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(3)
	}})
}
