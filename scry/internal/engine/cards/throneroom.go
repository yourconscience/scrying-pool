package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Throne Room: You may play an Action card from your hand twice.
func init() {
	engine.Register("Throne Room", engine.Rules{Play: func(c *engine.Ctx) {
		got := c.Choose(c.P, engine.ToPlay, c.Options(c.P, engine.Hand, action), 0, 1)
		if len(got) == 0 {
			return
		}
		c.PlayCard(got[0].Inst, false)
		c.PlayCard(got[0].Inst, true)
	}})
}
