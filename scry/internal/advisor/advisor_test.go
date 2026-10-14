package advisor

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestRateWilson(t *testing.T) {
	r := rate([]float64{1, 1, 1, 1, 1, 0, 0, 0, 0, 0})
	if r.Games != 10 || r.Rate != 0.5 || !near(r.Low, 0.237, 0.001) || !near(r.High, 0.763, 0.001) {
		t.Fatalf("5 of 10: %+v", r)
	}
	if r := rate([]float64{1, 0.5, 0}); r.Wins != 1.5 || r.Rate != 0.5 {
		t.Fatalf("a tie counts half: %+v", r)
	}
	if r := rate(nil); r.Games != 0 || r.Rate != 0 {
		t.Fatalf("no games: %+v", r)
	}
}

// Endings follow markus's rule for logs that miss the last decision.
func TestEnding(t *testing.T) {
	s := func(left map[string]int) cartographer.SupplyState {
		st := cartographer.SupplyState{Initial: map[string]int{}, Left: left}
		for p := range left {
			st.Initial[p] = 10
		}
		st.Initial["Province"] = 8
		return st
	}
	for _, c := range []struct {
		left map[string]int
		want string
	}{
		{map[string]int{"Province": 1, "Smithy": 4}, "provinces"},
		{map[string]int{"Province": 3, "Smithy": 0, "Village": 0, "Market": 1}, "piles"},
		{map[string]int{"Province": 3, "Smithy": 0, "Village": 0, "Market": 0}, "piles"},
		{map[string]int{"Province": 3, "Smithy": 0, "Village": 2}, "resign"},
		{map[string]int{"Province": 3, "Smithy": 0, "Village": 0, "Horse": 1}, "resign"}, // Horses aren't a Supply pile
	} {
		if got := ending(s(c.left)); got != c.want {
			t.Errorf("%v: %s, want %s", c.left, got, c.want)
		}
	}
}

// Profiles measured from the test logs match what the cards say, for cards
// played often enough there.
func TestProfilesMatchCardText(t *testing.T) {
	p := cartographer.DefaultClientPath()
	if p == "" {
		t.Skip("no exported dominion.games client tables")
	}
	c, err := cartographer.LoadClient(p)
	if err != nil {
		t.Fatal(err)
	}
	logs, _ := filepath.Glob("../../testdata/logs/*.log")
	var games []*cartographer.Game
	for _, f := range logs {
		b, _ := os.ReadFile(f)
		if g, err := c.Parse(string(b), nil); err == nil {
			games = append(games, g)
		}
	}
	ps := Profiles(games)
	for card, want := range map[string]Profile{
		"Copper":        {Coins: 1},
		"Gold":          {Coins: 3},
		"Horse":         {Actions: 1, Cards: 2},
		"Market Square": {Actions: 1, Buys: 1, Cards: 1},
		"Herald":        {Actions: 1, Cards: 1},
		"Spices":        {Buys: 1, Coins: 2},
	} {
		got := ps[card]
		if got == nil {
			t.Errorf("%s: not played in the test logs", card)
			continue
		}
		if !near(got.Actions, want.Actions, 0.05) || !near(got.Buys, want.Buys, 0.05) || !near(got.Cards, want.Cards, 0.05) || !near(got.Coins, want.Coins, 0.05) {
			t.Errorf("%s: %+v, want %+v", card, *got, want)
		}
	}
	if r := CardRoles("Market Square", ps, nil); len(r) != 1 || r[0] != "plus_buy" {
		t.Errorf("Market Square roles %v", r)
	}
}

func TestSplitAndPlayers(t *testing.T) {
	s := split("x", "", []float64{1, 1, 1, 0}, []float64{1, 0, 0, 0})
	if !near(s.Diff, 0.5, 1e-9) || s.DiffLow >= s.Diff || s.DiffHigh <= s.Diff {
		t.Fatalf("split %+v", s)
	}
	fs := []*Features{{Me: "a"}, {Me: "b"}, {Me: "a"}, {Me: "c"}, {Me: "a"}, {Me: "b"}}
	got := Players(fs, 2)
	if len(got) != 2 || got[0] != (PlayerCount{"a", 3}) || got[1] != (PlayerCount{"b", 2}) {
		t.Fatalf("players %v", got)
	}
	if n := len(Of("b", fs)); n != 2 {
		t.Fatalf("Of(b) = %d games", n)
	}
	in := Summarize("a", Of("a", fs))
	if in.Form == nil || in.Cards == nil {
		t.Fatal("empty lists must be [] in JSON, not null")
	}
}

func TestLatestGames(t *testing.T) {
	score := "# row: 2026-Oct-11 | FLOYDWG: 31, gamrevinu: 47\n# played: 2026-10-11T05:45:07.121Z\n# levels: FLOYDWG 48.25 / gamrevinu 55.50\n"
	if l := Levels(score); len(l) != 2 || l["FLOYDWG"] != 48.25 || l["gamrevinu"] != 55.5 {
		t.Fatalf("levels %v", l)
	}
	if len(Levels("# row: x\n")) != 0 {
		t.Fatal("no levels line, no levels")
	}
	if c := WinChance(55.5, 55.5); c != 0.5 {
		t.Fatalf("equal levels: %v", c)
	}
	if c := WinChance(57.5, 50); !near(c, 1/(1+math.Exp(-1)), 1e-9) {
		t.Fatalf("7.5 levels apart is one μ: %v", c)
	}
	fs := []*Features{
		{ID: "99", Me: "a", Opponent: "x", Result: 1, MyLevel: 55, OppLevel: 55, Turns: 14, FirstProvince: 9, Coins: []int{4, 3, 5, 5, 6, 6, 7, 8, 8, 9, 9, 9}},
		{ID: "101", Me: "a", Opponent: "y", Result: 0, Turns: 16, Coins: []int{5, 2}},
		{ID: "100", Me: "a", Opponent: "x", Result: 0.5, MyLevel: 55, OppLevel: 62.5, Turns: 15, FirstProvince: 11},
		{ID: "102", Me: "b", Opponent: "a", Result: 1},
	}
	r := LatestGames("a", fs, 2)
	if len(r.Games) != 2 || r.Games[0].ID != "101" || r.Games[1].ID != "100" {
		t.Fatalf("latest first, by game number: %v, %v", r.Games[0].ID, r.Games[1].ID)
	}
	if r.Score != 0.5 || r.Leveled != 1 || r.LevelScore != 0.5 || !near(r.Expected, WinChance(55, 62.5), 1e-9) || r.Opponents != 2 {
		t.Fatalf("summary %+v", r)
	}
	all := LatestGames("a", fs, 25)
	if all.OppLow != 55 || all.OppHigh != 62.5 || all.Wins.Games != 1 || all.Losses.Games != 1 {
		t.Fatalf("all: %+v", all)
	}
	if w := all.Wins; w.FirstProvince != 9 || w.Turns != 14 || !near(w.Coins, 6.1, 1e-9) {
		t.Fatalf("wins: %+v (turns 1-10 only)", w)
	}
	if l := all.Losses; l.NoProvince != 1 || l.FirstProvince != 0 || l.Coins != 3.5 {
		t.Fatalf("losses: %+v", l)
	}
}
