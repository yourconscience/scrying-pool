package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Artisan", engine.Rules{Play: func(c *engine.Ctx) {
		c.ChooseGain(c.P, c.CostsUpTo(5), engine.Hand)
		c.Topdeck(c.P, engine.Cards(c.Choose(c.P, engine.ToTopdeck, c.Options(c.P, engine.Hand, nil), 1, 1)))
	}})
}
