package advisor

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/yourconscience/scrying-pool/cartographer"
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
	logs, _ := filepath.Glob("../testdata/logs/*.log")
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
