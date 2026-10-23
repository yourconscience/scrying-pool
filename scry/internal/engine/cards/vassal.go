package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Vassal: +$2. Discard the top card of your deck. If it's an Action card, you may play it.
func init() {
	engine.Register("Vassal", engine.Rules{Play: func(c *engine.Ctx) {
		c.PlusCoins(2)
		x := c.DiscardTop(c.P)
		if x >= 0 && c.G.Def(x).Action() && len(c.Choose(c.P, engine.ToPlay, c.OptionsOf([]engine.Inst{x}), 0, 1)) == 1 {
			c.PlayCard(x, false)
		}
	}})
}
