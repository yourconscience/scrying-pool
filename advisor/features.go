package advisor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yourconscience/scrying-pool/cartographer"
)

// Features describe one game from one player's side.
type Features struct {
	ID       string `json:"id"`
	Date     string `json:"date"` // "2026-Oct-09", from the row
	Me       string `json:"me"`
	Opponent string `json:"opponent"`
	First    bool   `json:"first"` // Me took the first turn

	Result float64 `json:"result"` // 1 win, 0.5 tie, 0 loss: the row's scores, else the computed VP
	MyVP   int     `json:"my_vp"`
	OppVP  int     `json:"opp_vp"`
	// the computed VP equal the row's scores; when they don't, usually the game's
	// last decision (often the final buy) is missing from the loaded log
	Complete bool   `json:"complete"`
	Ending   string `json:"ending"` // provinces | piles | resign, by the rule below
	Turns    int    `json:"turns"`  // Me's turns

	Kingdom    []string        `json:"kingdom"`
	Landscapes []string        `json:"landscapes"`
	Roles      map[string]bool `json:"roles"` // the kingdom has a card filling the role
	Took       map[string]bool `json:"took"`  // kingdom cards Me owned at some point

	Opening       string   `json:"opening"`        // $ of turns 1 and 2, larger first: "4/3"
	OpeningBuys   []string `json:"opening_buys"`   // cards bought or gained on turns 1-2
	FirstGold     int      `json:"first_gold"`     // Me's turn of the first Gold, 0 if none
	FirstProvince int      `json:"first_province"` // Me's turn of the first Province, 0 if none
	Coins         []int    `json:"coins"`          // Me's $ on each of their turns
}

// Feature computes a saved game's features for player me. It returns nil when
// me didn't play the game or it has no turns.
func Feature(client *cartographer.Client, s *Saved, me string, profiles map[string]*Profile) *Features {
	g := s.Game
	if len(g.Players) != 2 || (g.Players[0] != me && g.Players[1] != me) {
		return nil
	}
	opp := g.Players[0]
	if opp == me {
		opp = g.Players[1]
	}
	turns := g.Turns()
	if len(turns) == 0 {
		return nil
	}
	f := &Features{ID: s.ID, Me: me, Opponent: opp, Kingdom: s.Kingdom, Landscapes: s.Landscapes,
		Roles: map[string]bool{}, Took: map[string]bool{}}
	if len(s.Original) > 0 {
		f.First = s.Original[0] == me // seat 1 is the first original player's
	}
	if d, _, ok := strings.Cut(s.Row, " | "); ok {
		f.Date = d
	}

	vp := client.Score(g, cartographer.ScoreInput{Landscapes: client.Landscapes(s.Score)})
	f.MyVP, f.OppVP = vp[me].Total(), vp[opp].Total()
	mine, theirs := f.MyVP, f.OppVP
	if sc := s.Scores; sc != nil {
		if a, ok := sc[me]; ok {
			if b, ok := sc[opp]; ok {
				f.Complete = a == f.MyVP && b == f.OppVP
				mine, theirs = a, b
			}
		}
	}
	switch {
	case mine > theirs:
		f.Result = 1
	case mine == theirs:
		f.Result = 0.5
	}
	f.Ending = ending(g.Supply(client))

	for _, card := range s.Kingdom {
		var types []string
		if c := client.Cards[card]; c != nil {
			types = c.Types
		}
		for _, r := range CardRoles(card, profiles, types) {
			f.Roles[r] = true
		}
	}
	kingdomPile := map[string]string{} // pile -> kingdom card
	for _, card := range s.Kingdom {
		kingdomPile[client.PileOf(card)] = card
	}
	for _, t := range turns {
		for card, n := range t.Decks[me] {
			if k, ok := kingdomPile[client.PileOf(card)]; ok && n > 0 {
				f.Took[k] = true
			}
		}
		if t.Player != me || t.Extra != "" {
			continue
		}
		f.Turns++
		f.Coins = append(f.Coins, t.Coins)
		for _, c := range append(append([]cartographer.CardN{}, t.Bought...), t.Gained...) {
			if c.Card == nil {
				continue
			}
			if f.Turns <= 2 {
				f.OpeningBuys = append(f.OpeningBuys, c.Card.Name)
			}
			if c.Card.Name == "Gold" && f.FirstGold == 0 {
				f.FirstGold = f.Turns
			}
			if c.Card.Name == "Province" && f.FirstProvince == 0 {
				f.FirstProvince = f.Turns
			}
		}
	}
	if len(f.Coins) >= 2 {
		a, b := f.Coins[0], f.Coins[1]
		if b > a {
			a, b = b, a
		}
		f.Opening = fmt.Sprintf("%d/%d", a, b)
	}
	sort.Strings(f.OpeningBuys)
	return f
}

// ending classifies how a game ended, as markus did for logs that miss the last
// decision: at most one Province (or Colony) left is a Province ending; at most
// one card away from three empty Supply piles is a three-pile ending; anything
// else was a resignation.
func ending(s cartographer.SupplyState) string {
	for _, p := range []string{"Province", "Colony"} {
		if _, ok := s.Initial[p]; ok && s.Left[p] <= 1 {
			return "provinces"
		}
	}
	empty, nearly := 0, 0
	for p := range s.Initial {
		switch {
		case s.Empty(p):
			empty++
		case s.InSupply(p) && s.Left[p] == 1:
			nearly++
		}
	}
	if empty >= 3 || empty == 2 && nearly > 0 {
		return "piles"
	}
	return "resign"
}
