package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Witch", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(2)
		c.Attack(func(v int) { c.Gain(v, c.G.T.MustCard("Curse"), engine.Discard) })
	}})
}
