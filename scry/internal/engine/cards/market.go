package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Market: +1 Card, +1 Action, +1 Buy, +$1.
func init() {
	engine.Register("Market", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusBuys(1)
		c.PlusCoins(1)
	}})
}
