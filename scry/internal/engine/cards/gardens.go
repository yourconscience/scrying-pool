package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Gardens", engine.Rules{VP: func(g *engine.Game, p int) int { return len(g.Owned(p)) / 10 }})
}
