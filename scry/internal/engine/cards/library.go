package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Library: Draw until you have 7 cards in hand, skipping any Action cards you choose to; set those aside, discarding them afterwards.
func init() {
	engine.Register("Library", engine.Rules{Play: func(c *engine.Ctx) {
		var aside []engine.Inst
		for len(c.Hand(c.P)) < 7 {
			x := c.DrawOne(c.P)
			if x < 0 {
				break
			}
			c.LookAt(c.P, []engine.Inst{x}) // the client logs each card Library draws as looked at
			if c.G.Def(x).Action() && len(c.Choose(c.P, engine.ToSetAside, c.OptionsOf([]engine.Inst{x}), 0, 1)) == 1 {
				c.SetAsideWith(c.P, x)
				aside = append(aside, x)
			}
		}
		c.Discard(c.P, aside)
	}})
}
