package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Market", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusBuys(1)
		c.PlusCoins(1)
	}})
}
