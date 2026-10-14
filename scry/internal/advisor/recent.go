package advisor

import (
	"math"
	"sort"
)

// Recent is a player's latest saved games with what DomBot's !results shows for
// them: the score against the score the rating levels predicted, and how strong
// the opponents were. Wins and losses are compared on a few tempo measures.
type Recent struct {
	Me    string       `json:"me"`
	Games []RecentGame `json:"games"` // newest first
	Score float64      `json:"score"` // wins plus half the ties

	// over the games whose profile gave both levels
	Leveled    int     `json:"leveled"`
	LevelScore float64 `json:"level_score"`
	Expected   float64 `json:"expected"` // the sum of their win chances
	OppLow     float64 `json:"opp_low"`  // the weakest and strongest opponent's level
	OppHigh    float64 `json:"opp_high"`
	Opponents  int     `json:"opponents"` // different opponents, over all the games

	Wins   Side `json:"wins"`
	Losses Side `json:"losses"`
}

// RecentGame is one game of Recent.
type RecentGame struct {
	*Features
	Chance float64 `json:"chance,omitempty"` // the win chance both levels give; 0 without them
}

// Side averages a few measures over the games won, or lost.
type Side struct {
	Games         int     `json:"games"`
	Turns         float64 `json:"turns"`          // the player's turns
	Coins         float64 `json:"coins"`          // $ per turn over turns 1-10
	FirstProvince float64 `json:"first_province"` // turn of the first Province, over the games with one
	NoProvince    int     `json:"no_province"`    // games without a Province
}

// WinChance is the chance to win that two rating levels give. A dominion.games
// level is 50 + 7.5 × (μ − 2φ) (as DomBot's !results shows: 55.82 for μ 1.05,
// φ 0.137), so with similar φ the level difference over 7.5 is the μ difference,
// and the win chance is 1 / (1 + e^−Δμ), the model markus's statistics used.
// It's an estimate: φ differs between players.
func WinChance(myLevel, oppLevel float64) float64 {
	return 1 / (1 + math.Exp(-(myLevel-oppLevel)/7.5))
}

// LatestGames summarizes a player's n latest saved games. Game numbers grow
// with time, so the latest games are the highest numbers.
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
	opps := map[string]bool{}
	var wins, losses []*Features
	for _, f := range mine {
		g := RecentGame{Features: f}
		r.Score += f.Result
		opps[f.Opponent] = true
		if f.MyLevel > 0 && f.OppLevel > 0 {
			g.Chance = WinChance(f.MyLevel, f.OppLevel)
			if r.Leveled == 0 || f.OppLevel < r.OppLow {
				r.OppLow = f.OppLevel
			}
			if r.Leveled == 0 || f.OppLevel > r.OppHigh {
				r.OppHigh = f.OppLevel
			}
			r.Leveled++
			r.LevelScore += f.Result
			r.Expected += g.Chance
		}
		switch f.Result {
		case 1:
			wins = append(wins, f)
		case 0:
			losses = append(losses, f)
		}
		r.Games = append(r.Games, g)
	}
	r.Opponents = len(opps)
	r.Wins, r.Losses = side(wins), side(losses)
	return r
}

func side(fs []*Features) Side {
	s := Side{Games: len(fs)}
	var coins, turns, provinces float64
	var nCoins, nProvinces int
	for _, f := range fs {
		turns += float64(f.Turns)
		for i, c := range f.Coins {
			if i == 10 {
				break
			}
			coins += float64(c)
			nCoins++
		}
		if f.FirstProvince > 0 {
			provinces += float64(f.FirstProvince)
			nProvinces++
		} else {
			s.NoProvince++
		}
	}
	if len(fs) > 0 {
		s.Turns = turns / float64(len(fs))
	}
	if nCoins > 0 {
		s.Coins = coins / float64(nCoins)
	}
	if nProvinces > 0 {
		s.FirstProvince = provinces / float64(nProvinces)
	}
	return s
}
