package advisor

import (
	"sort"
	"strings"
)

// Side is one player's record over a set of their games, made to set an
// opponent's beside your own: how they did, how they opened, what they built
// and in which kingdoms they won.
type Side struct {
	Name  string `json:"name"`
	Games int    `json:"games"`
	First string `json:"first_date"`
	Last  string `json:"last_date"`

	Record   Rate    `json:"record"`
	Margin   float64 `json:"margin"`    // average VP minus the opponent's
	Level    float64 `json:"level"`     // the rating level in the latest game that has one
	OppLevel float64 `json:"opp_level"` // the opponents' average level
	// wins (ties half) and what both players' levels predicted, over the games with levels
	Leveled  int     `json:"leveled"`
	Won      float64 `json:"won"`
	Expected float64 `json:"expected"`

	Turns         float64 `json:"turns"`          // the player's turns per game
	FirstGold     float64 `json:"first_gold"`     // average turn, over the games with one
	FirstProvince float64 `json:"first_province"` // the same
	NoProvince    int     `json:"no_province"`    // games without a Province

	Seat     Split           `json:"seat"`     // as the first player
	Stronger Split           `json:"stronger"` // against a higher-rated opponent
	Endings  map[string]Rate `json:"endings"`  // provinces, piles, resign
	Length   []Row           `json:"length"`   // by the player's turns

	Splits   map[string]Rate `json:"splits"`   // how the Coppers fell: "5/2", "4/3"
	Openings []Row           `json:"openings"` // the cards bought on turns 1-2
	Opened   []Row           `json:"opened"`   // the kingdom cards among them
	Styles   []Split         `json:"styles"`   // what the player built: games with and without
	Kingdoms []Split         `json:"kingdoms"` // what the kingdom offered: games with and without
	Took     []CardTake      `json:"took"`     // kingdom cards, most often taken first
	Skipped  []CardTake      `json:"skipped"`  // kingdom cards left alone most often
}

// Row is a share of games won among games with one thing in common.
type Row struct {
	Name string `json:"name"`
	Rate Rate   `json:"rate"`
}

// CardTake is a kingdom card: the games it was in the kingdom, those where the
// player took it, and their results then.
type CardTake struct {
	Card    string `json:"card"`
	Present int    `json:"present"`
	Took    Rate   `json:"took"`
	Skipped Rate   `json:"skipped"`
}

// Styles name what a player built, judged from the roles (CardRoles) of the
// kingdom cards they took: an engine has a village and draw, "draw, no village"
// is Big Money with a drawing card, and "no village or draw" is everything slower.
var styles = []struct {
	Name, Desc string
	Has        func(took map[string]bool) bool
}{
	{"engine", "took a village and a drawing card", func(t map[string]bool) bool { return t["village"] && t["draw"] }},
	{"draw, no village", "took a drawing card but no village (Big Money with a card)", func(t map[string]bool) bool { return t["draw"] && !t["village"] }},
	{"no village or draw", "took neither", func(t map[string]bool) bool { return !t["draw"] && !t["village"] }},
	{"trashing", "took a trasher", func(t map[string]bool) bool { return t["trasher"] }},
	{"attacking", "took an Attack", func(t map[string]bool) bool { return t["attack"] }},
	{"cursing", "took a card that gives Curses", func(t map[string]bool) bool { return t["curser"] }},
	{"+Buy", "took a card with +Buy", func(t map[string]bool) bool { return t["plus_buy"] }},
}

// Scout sums up a player's games for Side. roles says which roles a kingdom
// card fills (CardRoles with the card's types).
func Scout(name string, games []*Features, roles func(card string) []string) *Side {
	s := &Side{Name: name, Games: len(games), Endings: map[string]Rate{}, Splits: map[string]Rate{},
		Length: []Row{}, Openings: []Row{}, Opened: []Row{}, Styles: []Split{}, Kingdoms: []Split{}, Took: []CardTake{}, Skipped: []CardTake{}}
	if len(games) == 0 {
		return s
	}
	var all, first, second, stronger, weaker []float64
	byEnd, bySplit, byOpen, byOpened := map[string][]float64{}, map[string][]float64{}, map[string][]float64{}, map[string][]float64{}
	lengths := []struct {
		name string
		max  int
	}{{"up to 14 turns", 14}, {"15-19 turns", 19}, {"20 turns or more", 1 << 30}}
	byLen := make([][]float64, len(lengths))
	took := make([]map[string]bool, len(games)) // each game's roles the player took
	var gold, prov, oppLevels []float64
	latest := ""
	for i, f := range games {
		all = append(all, f.Result)
		m := f.MyVP - f.OppVP
		if len(f.Final) == 2 {
			m = f.Final[0] - f.Final[1] // the row's scores when there are any
		}
		s.Margin += float64(m)
		if f.First {
			first = append(first, f.Result)
		} else {
			second = append(second, f.Result)
		}
		if f.MyLevel > 0 && f.OppLevel > 0 {
			s.Leveled++
			s.Won += f.Result
			s.Expected += WinChance(f.MyLevel, f.OppLevel)
			oppLevels = append(oppLevels, f.OppLevel)
			if f.OppLevel > f.MyLevel {
				stronger = append(stronger, f.Result)
			} else {
				weaker = append(weaker, f.Result)
			}
			if f.Played >= latest {
				latest, s.Level = f.Played, f.MyLevel
			}
		}
		byEnd[f.Ending] = append(byEnd[f.Ending], f.Result)
		for k, l := range lengths {
			if f.Turns <= l.max {
				byLen[k] = append(byLen[k], f.Result)
				break
			}
		}
		s.Turns += float64(f.Turns)
		if f.FirstGold > 0 {
			gold = append(gold, float64(f.FirstGold))
		}
		if f.FirstProvince > 0 {
			prov = append(prov, float64(f.FirstProvince))
		} else {
			s.NoProvince++
		}
		if f.Opening != "" && f.Opening != "other" {
			bySplit[f.Opening] = append(bySplit[f.Opening], f.Result)
		}
		if len(f.OpeningBuys) > 0 {
			k := strings.Join(f.OpeningBuys, " + ")
			byOpen[k] = append(byOpen[k], f.Result)
		}
		for _, c := range f.Opened {
			byOpened[c] = append(byOpened[c], f.Result)
		}
		took[i] = map[string]bool{}
		for c := range f.Took {
			for _, r := range roles(c) {
				took[i][r] = true
			}
		}
	}
	s.Record = rate(all)
	s.Margin /= float64(len(games))
	s.Turns /= float64(len(games))
	s.FirstGold, s.FirstProvince = mean(gold), mean(prov)
	s.OppLevel = mean(oppLevels)
	s.Seat = split("first player", "", first, second)
	s.Stronger = split("against a higher-rated opponent", "", stronger, weaker)
	for k, v := range byEnd {
		s.Endings[k] = rate(v)
	}
	for k, v := range bySplit {
		s.Splits[k] = rate(v)
	}
	for k, l := range lengths {
		if len(byLen[k]) > 0 {
			s.Length = append(s.Length, Row{l.name, rate(byLen[k])})
		}
	}
	s.Openings, s.Opened = rows(byOpen), rows(byOpened)

	for _, st := range styles {
		var with, without []float64
		for i, f := range games {
			if st.Has(took[i]) {
				with = append(with, f.Result)
			} else {
				without = append(without, f.Result)
			}
		}
		s.Styles = append(s.Styles, split(st.Name, st.Desc, with, without))
	}
	descs := RoleDescriptions()
	for _, role := range RoleNames() {
		var with, without []float64
		for _, f := range games {
			if len(f.Kingdom) == 0 {
				continue
			}
			if f.Roles[role] {
				with = append(with, f.Result)
			} else {
				without = append(without, f.Result)
			}
		}
		s.Kingdoms = append(s.Kingdoms, split("a "+strings.ReplaceAll(role, "_", " ")+" in the kingdom", descs[role], with, without))
	}

	type acc struct{ took, skipped []float64 }
	cards := map[string]*acc{}
	for _, f := range games {
		for _, k := range f.Kingdom {
			a := cards[k]
			if a == nil {
				a = &acc{}
				cards[k] = a
			}
			if f.Took[k] {
				a.took = append(a.took, f.Result)
			} else {
				a.skipped = append(a.skipped, f.Result)
			}
		}
	}
	for k, a := range cards {
		c := CardTake{Card: k, Present: len(a.took) + len(a.skipped), Took: rate(a.took), Skipped: rate(a.skipped)}
		if c.Present < 2 {
			continue // seen once says nothing about a habit
		}
		s.Took = append(s.Took, c)
	}
	sort.Slice(s.Took, func(i, j int) bool {
		a, b := s.Took[i], s.Took[j]
		if a.Took.Games != b.Took.Games {
			return a.Took.Games > b.Took.Games
		}
		return a.Card < b.Card
	})
	for _, c := range s.Took {
		if c.Skipped.Games > c.Took.Games {
			s.Skipped = append(s.Skipped, c)
		}
	}
	sort.SliceStable(s.Skipped, func(i, j int) bool { return s.Skipped[i].Skipped.Games > s.Skipped[j].Skipped.Games })
	s.First, s.Last = games[0].Date, games[len(games)-1].Date
	return s
}

// rows turns grouped results into rows, most games first.
func rows(by map[string][]float64) []Row {
	out := []Row{}
	for k, v := range by {
		out = append(out, Row{k, rate(v)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rate.Games != out[j].Rate.Games {
			return out[i].Rate.Games > out[j].Rate.Games
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}
