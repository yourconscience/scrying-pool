package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Sentry", engine.Rules{Play: func(c *engine.Ctx) {
		c.Draw(1)
		c.PlusActions(1)
		top := c.FromTop(c.P, 2, "LOOK_AT")
		c.Trash(c.P, engine.Cards(c.Choose(c.P, engine.ToTrash, c.OptionsOf(top), 0, len(top))))
		rest := c.In(engine.Revealed, top)
		c.Discard(c.P, engine.Cards(c.Choose(c.P, engine.ToDiscard, c.OptionsOf(rest), 0, len(rest))))
		rest = c.In(engine.Revealed, rest)
		if len(rest) > 1 {
			rest = engine.Cards(c.Choose(c.P, engine.ToOrder, c.OptionsOf(rest), len(rest), len(rest)))
			rest[0], rest[1] = rest[1], rest[0] // the first picked goes on top, so it moves last
		}
		c.Topdeck(c.P, rest)
	}})
}
