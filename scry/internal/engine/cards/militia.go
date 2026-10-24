package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Militia", engine.Rules{Play: func(c *engine.Ctx) {
		c.PlusCoins(2)
		c.Attack(func(v int) {
			if n := len(c.Hand(v)) - 3; n > 0 {
				c.Discard(v, engine.Cards(c.Choose(v, engine.ToDiscard, c.Options(v, engine.Hand, nil), n, n)))
			}
		})
	}})
}
