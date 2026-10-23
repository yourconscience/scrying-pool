package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Chapel", engine.Rules{Play: func(c *engine.Ctx) {
		got := c.Choose(c.P, engine.ToTrash, c.Options(c.P, engine.Hand, nil), 0, 4)
		c.Trash(c.P, engine.Cards(got))
	}})
}
