package advisor

import (
	"math"
	"sort"
)

// Recent is a player's latest saved games, newest first. The Recent games page
// sums them up the way DomBot's !results does (scry/web/recent.js).
type Recent struct {
	Me    string       `json:"me"`
	Games []RecentGame `json:"games"`
}

// RecentGame is one game of Recent.
type RecentGame struct {
	*Features
	Chance float64 `json:"chance,omitempty"` // the win chance both levels give; 0 without them
}

// WinChance is the chance to win that two rating levels give. A dominion.games
// level is 50 + 7.5 × (μ − 2φ) (as DomBot's !results shows: 55.82 for μ 1.05,
// φ 0.137), so with similar φ the level difference over 7.5 is the μ difference,
// and the win chance is 1 / (1 + e^−Δμ), the model markus's statistics used.
// It's an estimate: φ differs between players. recent.js uses the same formula
// for games it knows only from a profile.
func WinChance(myLevel, oppLevel float64) float64 {
	return 1 / (1 + math.Exp(-(myLevel-oppLevel)/7.5))
}

// LatestGames returns a player's n latest saved games. Game numbers grow with
// time, so the latest games are the highest numbers.
func LatestGames(me string, features []*Features, n int) *Recent {
	mine := Of(me, features)
	sort.Slice(mine, func(i, j int) bool {
		a, b := mine[i].ID, mine[j].ID
		if len(a) != len(b) {
			return len(a) > len(b)
		}
		return a > b
	})
	if len(mine) > n {
		mine = mine[:n]
	}
	r := &Recent{Me: me, Games: []RecentGame{}}
	for _, f := range mine {
		g := RecentGame{Features: f}
		if f.MyLevel > 0 && f.OppLevel > 0 {
			g.Chance = WinChance(f.MyLevel, f.OppLevel)
		}
		r.Games = append(r.Games, g)
	}
	return r
}
