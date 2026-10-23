package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Remodel: Trash a card from your hand. Gain a card costing up to $2 more than it.
func init() {
	engine.Register("Remodel", engine.Rules{Play: func(c *engine.Ctx) {
		got := c.Choose(c.P, engine.ToTrash, c.Options(c.P, engine.Hand, nil), 1, 1)
		if len(got) == 0 {
			return
		}
		c.Trash(c.P, engine.Cards(got))
		c.ChooseGain(c.P, c.CostsUpTo(c.G.Cost(got[0].Card)+2), engine.Discard)
	}})
}
