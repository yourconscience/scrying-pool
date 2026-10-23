package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Bureaucrat: Gain a Silver onto your deck. Each other player reveals a Victory card from their hand and puts it onto their deck (or reveals a hand with no Victory cards).
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
