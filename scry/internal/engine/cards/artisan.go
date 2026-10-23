package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Artisan: Gain a card to your hand costing up to $5. Put a card from your hand onto your deck.
func init() {
	engine.Register("Artisan", engine.Rules{Play: func(c *engine.Ctx) {
		c.ChooseGain(c.P, c.CostsUpTo(5), engine.Hand)
		c.Topdeck(c.P, engine.Cards(c.Choose(c.P, engine.ToTopdeck, c.Options(c.P, engine.Hand, nil), 1, 1)))
	}})
}
