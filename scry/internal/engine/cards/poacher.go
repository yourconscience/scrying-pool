package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Poacher", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(1)
		c.PlusActions(1)
		c.PlusCoins(1)
		if n := c.G.EmptyPiles(); n > 0 {
			c.Discard(c.P, engine.Cards(c.Choose(c.P, engine.ToDiscard, c.Options(c.P, engine.Hand, nil), n, n)))
		}
	}})
}
