package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Merchant", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(1)
		c.PlusActions(1)
		for _, x := range c.G.P[c.P].Z[engine.InPlay] {
			if c.G.Def(x).Name == "Silver" {
				return // the first Silver was already played
			}
		}
		c.WhenPlay(func(c *engine.Ctx, x engine.Inst) bool {
			if c.G.Def(x).Name != "Silver" {
				return false
			}
			c.CoinsFrom(1)
			return true
		})
	}})
}
