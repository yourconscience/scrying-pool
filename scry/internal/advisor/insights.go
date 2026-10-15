package advisor

import (
	"math"
	"sort"
)

// z gives the ranges: 80% of them hold the true value. A narrower range than the
// usual 95% shows more with a few hundred games, at the price that about 1 in 5
// differences that are only chance looks clear.
const z = 1.2816

// Rate is a share of games won, ties counting half, with an 80% Wilson interval.
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
	n, p := float64(r.Games), r.Wins/float64(r.Games)
	r.Rate = p
	mid := (p + z*z/(2*n)) / (1 + z*z/n)
	half := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / (1 + z*z/n)
	r.Low, r.High = mid-half, mid+half
	return r
}

// Split compares games with and without something. Diff is With minus Without
// with an 80% range from the normal approximation.
type Split struct {
	Name     string  `json:"name"`
	Desc     string  `json:"desc,omitempty"`
	With     Rate    `json:"with"`
	Without  Rate    `json:"without"`
	Diff     float64 `json:"diff"`
	DiffLow  float64 `json:"diff_low"`
	DiffHigh float64 `json:"diff_high"`
}

func split(name, desc string, with, without []float64) Split {
	s := Split{Name: name, Desc: desc, With: rate(with), Without: rate(without)}
	a, b := s.With, s.Without
	if a.Games == 0 || b.Games == 0 {
		return s
	}
	s.Diff = a.Rate - b.Rate
	se := math.Sqrt(a.Rate*(1-a.Rate)/float64(a.Games) + b.Rate*(1-b.Rate)/float64(b.Games))
	s.DiffLow, s.DiffHigh = s.Diff-z*se, s.Diff+z*se
	return s
}

// FormPoint is the win rate over the window of games ending at game Index.
type FormPoint struct {
	Index int     `json:"index"`
	Date  string  `json:"date"`
	Rate  float64 `json:"rate"`
}

// Histogram counts games by a turn number, in wins and losses (ties left out).
type Histogram struct {
	Turn   []int  `json:"turn"`
	Wins   []int  `json:"wins"`
	Losses []int  `json:"losses"`
	Never  [2]int `json:"never"` // wins, losses without one
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

// Tempo is the player's average $ on each of their turns, in all games, wins and losses.
type Tempo struct {
	Turn   []int     `json:"turn"`
	All    []float64 `json:"all"`
	Wins   []float64 `json:"wins"`
	Losses []float64 `json:"losses"`
	NWins  []int     `json:"n_wins"`
	NLoss  []int     `json:"n_losses"`
}

// Insights is everything the "My games" page shows.
type Insights struct {
	Me       string          `json:"me"`
	Overall  Rate            `json:"overall"`
	AvgTurns float64         `json:"avg_turns"` // the player's turns per game
	Complete int             `json:"complete"`  // games whose computed VP match the row
	Seat     []Split         `json:"seat"`
	Endings  map[string]Rate `json:"endings"`
	Roles    []Split         `json:"roles"`
	Openings map[string]Rate `json:"openings"`
	Cards    []CardRow       `json:"cards"`
	Tempo    Tempo           `json:"tempo"`
	Form     []FormPoint     `json:"form"`           // win rate over the last formWindow games, in game order
	Province Histogram       `json:"first_province"` // turn of the first Province
	First    string          `json:"first_date"`
	Last     string          `json:"last_date"`
	MinGames int             `json:"min_games"` // cards seen in fewer games are left out
	// card -> image URL, filled by whoever serves the page (the local app or a static site)
	Images map[string]string `json:"images,omitempty"`
	Games  []*Features       `json:"games,omitempty"`
}

const (
	prior       = 10 // pseudo-games of shrinkage toward the overall rate
	minCardGame = 8
	tempoTurns  = 15
	formWindow  = 30
	maxTurn     = 20 // histogram turns past this go in the last bin
)

// Summarize computes the page's statistics from a player's game features.
func Summarize(me string, games []*Features) *Insights {
	in := &Insights{Me: me, Endings: map[string]Rate{}, Openings: map[string]Rate{}, MinGames: minCardGame, Games: games,
		Cards: []CardRow{}, Form: []FormPoint{}}
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
		in.AvgTurns += float64(f.Turns)
	}
	if len(games) > 0 {
		in.AvgTurns /= float64(len(games))
	}
	in.Overall = rate(all)
	in.Seat = []Split{split("first player", "", first, second)}
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
		in.Roles = append(in.Roles, split(role, descs[role], with, without))
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
	sort.Slice(in.Cards, func(i, j int) bool {
		if in.Cards[i].Shrunk != in.Cards[j].Shrunk {
			return in.Cards[i].Shrunk < in.Cards[j].Shrunk
		}
		return in.Cards[i].Card < in.Cards[j].Card // the same order on every run
	})

	t := Tempo{}
	for turn := 1; turn <= tempoTurns; turn++ {
		var w, l, nw, nl, a, na float64
		for _, f := range games {
			if turn > len(f.Coins) {
				continue
			}
			a += float64(f.Coins[turn-1])
			na++
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
		t.All = append(t.All, avg(a, na))
		t.Wins = append(t.Wins, avg(w, nw))
		t.Losses = append(t.Losses, avg(l, nl))
		t.NWins = append(t.NWins, int(nw))
		t.NLoss = append(t.NLoss, int(nl))
	}
	in.Tempo = t

	// games come in saved order, which is game-number order: oldest first
	for i := range games {
		if i+1 < formWindow {
			continue
		}
		var w []float64
		for _, f := range games[i+1-formWindow : i+1] {
			w = append(w, f.Result)
		}
		in.Form = append(in.Form, FormPoint{Index: i + 1, Date: games[i].Date, Rate: rate(w).Rate})
	}
	if len(games) > 0 {
		in.First, in.Last = games[0].Date, games[len(games)-1].Date
	}

	h := Histogram{}
	for turn := 1; turn <= maxTurn; turn++ {
		h.Turn = append(h.Turn, turn)
	}
	h.Wins, h.Losses = make([]int, maxTurn), make([]int, maxTurn)
	for _, f := range games {
		k := -1
		switch f.Result {
		case 1:
			k = 0
		case 0:
			k = 1
		}
		if k < 0 {
			continue
		}
		if f.FirstProvince == 0 {
			h.Never[k]++
			continue
		}
		i := min(f.FirstProvince, maxTurn) - 1
		if k == 0 {
			h.Wins[i]++
		} else {
			h.Losses[i]++
		}
	}
	in.Province = h
	return in
}

func avg(sum, n float64) float64 {
	if n == 0 {
		return 0
	}
	return sum / n
}
