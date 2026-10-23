package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Moneylender: You may trash a Copper from your hand for +$3.
func init() {
	engine.Register("Moneylender", engine.Rules{Play: func(c *engine.Ctx) {
		coppers := c.Options(c.P, engine.Hand, func(d *engine.Def) bool { return d.Name == "Copper" })
		if len(coppers) > 1 {
			coppers = coppers[:1]
		}
		if got := c.Choose(c.P, engine.ToTrash, coppers, 0, 1); len(got) == 1 {
			c.Trash(c.P, engine.Cards(got))
			c.PlusCoins(3)
		}
	}})
}
