package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Workshop: Gain a card costing up to $4.
func init() {
	engine.Register("Workshop", engine.Rules{Play: func(c *engine.Ctx) {
		c.ChooseGain(c.P, c.CostsUpTo(4), engine.Discard)
	}})
}
