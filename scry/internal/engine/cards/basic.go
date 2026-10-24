// Package cards holds the card rules, one file per kingdom card. Each file
// registers its card with the engine when the package is imported.
package cards

import "github.com/yourconscience/scrying-pool/scry/internal/engine"

func init() {
	engine.Register("Copper", engine.Rules{Coins: 1})
	engine.Register("Silver", engine.Rules{Coins: 2})
	engine.Register("Gold", engine.Rules{Coins: 3})
	engine.Register("Estate", engine.Rules{VP: worth(1)})
	engine.Register("Duchy", engine.Rules{VP: worth(3)})
	engine.Register("Province", engine.Rules{VP: worth(6)})
	engine.Register("Curse", engine.Rules{VP: worth(-1)})
}

func worth(n int) func(*engine.Game, int) int {
	return func(*engine.Game, int) int { return n }
}

func treasure(d *engine.Def) bool { return d.Treasure() }
func action(d *engine.Def) bool   { return d.Action() }
