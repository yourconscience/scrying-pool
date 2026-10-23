package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

// Gardens: worth 1 VP per 10 cards you have (round down).
func init() {
	engine.Register("Gardens", engine.Rules{VP: func(g *engine.Game, p int) int { return len(g.Owned(p)) / 10 }})
}
