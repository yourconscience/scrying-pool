package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Council Room: +4 Cards, +1 Buy. Each other player draws a card.
func init() {
	engine.Register("Council Room", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(4)
		c.PlusBuys(1)
		c.DrawFor(c.Other(), 1)
	}})
}
