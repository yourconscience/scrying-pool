package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Festival: +2 Actions, +1 Buy, +$2.
func init() {
	engine.Register("Festival", engine.Rules{Play: func(c *engine.Ctx) {
		c.PlusActions(2)
		c.PlusBuys(1)
		c.PlusCoins(2)
	}})
}
