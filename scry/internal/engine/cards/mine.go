package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Mine: You may trash a Treasure from your hand. Gain a Treasure to your hand costing up to $3 more than it.
func init() {
	engine.Register("Mine", engine.Rules{Play: func(c *engine.Ctx) {
		got := c.Choose(c.P, engine.ToTrash, c.Options(c.P, engine.Hand, treasure), 0, 1)
		if len(got) == 0 {
			return
		}
		c.Trash(c.P, engine.Cards(got))
		upTo := c.CostsUpTo(c.G.Cost(got[0].Card) + 3)
		c.ChooseGain(c.P, func(d *engine.Def) bool { return d.Treasure() && upTo(d) }, engine.Hand)
	}})
}
