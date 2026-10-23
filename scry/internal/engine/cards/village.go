package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Village", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(1)
		c.PlusActions(2)
	}})
}
