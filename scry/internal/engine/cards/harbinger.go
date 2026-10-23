package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Harbinger", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(1)
		c.PlusActions(1)
		pile := c.Options(c.P, engine.Discard, nil)
		c.LookAt(c.P, engine.Cards(pile))
		got := c.Choose(c.P, engine.ToTopdeck, pile, 0, 1)
		c.Topdeck(c.P, engine.Cards(got))
	}})
}
