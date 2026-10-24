package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Bureaucrat", engine.Rules{Play: func(c *engine.Ctx) {
		c.Gain(c.P, c.G.T.MustCard("Silver"), engine.Deck)
		c.Attack(func(v int) {
			vic := c.Options(v, engine.Hand, func(d *engine.Def) bool { return d.Victory() })
			if len(vic) == 0 {
				c.RevealHand(v)
				return
			}
			c.Topdeck(v, engine.Cards(c.Choose(v, engine.ToTopdeck, vic, 1, 1)))
		})
	}})
}
