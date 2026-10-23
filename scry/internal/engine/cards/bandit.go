package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Bandit: Gain a Gold. Each other player reveals the top 2 cards of their deck, trashes a revealed Treasure other than Copper, and discards the rest.
func init() {
	engine.Register("Bandit", engine.Rules{Play: func(c *engine.Ctx) {
		c.Gain(c.P, c.G.T.MustCard("Gold"), engine.Discard)
		c.Attack(func(v int) {
			top := c.FromTop(v, 2, "REVEAL")
			var loot []engine.Inst
			for _, x := range top {
				if d := c.G.Def(x); d.Treasure() && d.Name != "Copper" {
					loot = append(loot, x)
				}
			}
			c.Trash(v, engine.Cards(c.Choose(v, engine.ToTrash, c.OptionsOf(loot), 1, 1)))
			c.Discard(v, c.In(engine.Revealed, top))
		})
	}})
}
