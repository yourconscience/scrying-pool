package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Laboratory", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(2)
		c.PlusActions(1)
	}})
}
