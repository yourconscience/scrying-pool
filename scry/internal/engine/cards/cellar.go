package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Cellar: +1 Action. Discard any number of cards, then draw that many.
func init() {
	engine.Register("Cellar", engine.Rules{Play: func(c *engine.Ctx) {
		c.PlusActions(1)
		got := c.Choose(c.P, engine.ToDiscard, c.Options(c.P, engine.Hand, nil), 0, len(c.Hand(c.P)))
		c.Discard(c.P, engine.Cards(got))
		c.Draw(len(got))
	}})
}
