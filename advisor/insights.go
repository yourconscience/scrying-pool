package advisor

import (
	"math"
	"sort"
)

// Rate is a share of games won, ties counting half, with a 95% Wilson interval.
type Rate struct {
	Games int     `json:"games"`
	Wins  float64 `json:"wins"`
	Rate  float64 `json:"rate"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
}

func rate(results []float64) Rate {
	r := Rate{Games: len(results)}
	for _, x := range results {
		r.Wins += x
	}
	if r.Games == 0 {
		return r
	}
	n, p, z := float64(r.Games), r.Wins/float64(r.Games), 1.96
	r.Rate = p
	mid := (p + z*z/(2*n)) / (1 + z*z/n)
	half := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / (1 + z*z/n)
	r.Low, r.High = mid-half, mid+half
	return r
}

// Split compares games with and without something.
type Split struct {
	Name    string `json:"name"`
	Desc    string `json:"desc,omitempty"`
	With    Rate   `json:"with"`
	Without Rate   `json:"without"`
}

// CardRow is a kingdom card's record: games with it in the kingdom, and among
// those, games where the player took it or skipped it. Shrunk pulls the
// win rate with the card toward the overall rate, as if 10 average games were
// added, so cards seen a few times don't top the list by luck.
type CardRow struct {
	Card    string  `json:"card"`
	Present Rate    `json:"present"`
	Took    Rate    `json:"took"`
	Skipped Rate    `json:"skipped"`
	Shrunk  float64 `json:"shrunk"`
}

// Tempo is the player's average $ on each of their turns, in wins and in losses.
type Tempo struct {
	Turn   []int     `json:"turn"`
	Wins   []float64 `json:"wins"`
	Losses []float64 `json:"losses"`
	NWins  []int     `json:"n_wins"`
	NLoss  []int     `json:"n_losses"`
}

// Insights is everything the "My games" page shows.
type Insights struct {
	Me       string          `json:"me"`
	Overall  Rate            `json:"overall"`
	Complete int             `json:"complete"` // games whose computed VP match the row
	Seat     []Split         `json:"seat"`
	Endings  map[string]Rate `json:"endings"`
	Roles    []Split         `json:"roles"`
	Openings map[string]Rate `json:"openings"`
	Cards    []CardRow       `json:"cards"`
	Tempo    Tempo           `json:"tempo"`
	MinGames int             `json:"min_games"` // cards seen in fewer games are left out
	Games    []*Features     `json:"games"`
}

const (
	prior       = 10 // pseudo-games of shrinkage toward the overall rate
	minCardGame = 8
	tempoTurns  = 15
)

// Summarize computes the page's statistics from a player's game features.
func Summarize(me string, games []*Features) *Insights {
	in := &Insights{Me: me, Endings: map[string]Rate{}, Openings: map[string]Rate{}, MinGames: minCardGame, Games: games}
	var all []float64
	byEnd, byOpen := map[string][]float64{}, map[string][]float64{}
	var first, second []float64
	for _, f := range games {
		all = append(all, f.Result)
		byEnd[f.Ending] = append(byEnd[f.Ending], f.Result)
		if f.Opening != "" {
			byOpen[f.Opening] = append(byOpen[f.Opening], f.Result)
		}
		if f.First {
			first = append(first, f.Result)
		} else {
			second = append(second, f.Result)
		}
		if f.Complete {
			in.Complete++
		}
	}
	in.Overall = rate(all)
	in.Seat = []Split{{Name: "first player", With: rate(first), Without: rate(second)}}
	for k, v := range byEnd {
		in.Endings[k] = rate(v)
	}
	for k, v := range byOpen {
		if len(v) >= 3 {
			in.Openings[k] = rate(v)
		}
	}

	descs := RoleDescriptions()
	for _, role := range RoleNames() {
		var with, without []float64
		for _, f := range games {
			if len(f.Kingdom) == 0 {
				continue // no kingdom, no roles
			}
			if f.Roles[role] {
				with = append(with, f.Result)
			} else {
				without = append(without, f.Result)
			}
		}
		in.Roles = append(in.Roles, Split{Name: role, Desc: descs[role], With: rate(with), Without: rate(without)})
	}

	type acc struct{ present, took, skipped []float64 }
	cards := map[string]*acc{}
	for _, f := range games {
		for _, k := range f.Kingdom {
			a := cards[k]
			if a == nil {
				a = &acc{}
				cards[k] = a
			}
			a.present = append(a.present, f.Result)
			if f.Took[k] {
				a.took = append(a.took, f.Result)
			} else {
				a.skipped = append(a.skipped, f.Result)
			}
		}
	}
	base := in.Overall.Rate
	for k, a := range cards {
		if len(a.present) < minCardGame {
			continue
		}
		p := rate(a.present)
		in.Cards = append(in.Cards, CardRow{Card: k, Present: p, Took: rate(a.took), Skipped: rate(a.skipped),
			Shrunk: (p.Wins + prior*base) / (float64(p.Games) + prior)})
	}
	sort.Slice(in.Cards, func(i, j int) bool { return in.Cards[i].Shrunk < in.Cards[j].Shrunk })

	t := Tempo{}
	for turn := 1; turn <= tempoTurns; turn++ {
		var w, l, nw, nl float64
		for _, f := range games {
			if turn > len(f.Coins) {
				continue
			}
			switch f.Result {
			case 1:
				w += float64(f.Coins[turn-1])
				nw++
			case 0:
				l += float64(f.Coins[turn-1])
				nl++
			}
		}
		t.Turn = append(t.Turn, turn)
		t.Wins = append(t.Wins, avg(w, nw))
		t.Losses = append(t.Losses, avg(l, nl))
		t.NWins = append(t.NWins, int(nw))
		t.NLoss = append(t.NLoss, int(nl))
	}
	in.Tempo = t
	return in
}

func avg(sum, n float64) float64 {
	if n == 0 {
		return 0
	}
	return sum / n
}
