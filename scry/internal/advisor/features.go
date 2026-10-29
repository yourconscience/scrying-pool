package advisor

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

// Features describe one game from one player's side.
type Features struct {
	ID       string `json:"id"`
	Date     string `json:"date"` // "2026-Oct-09", from the row
	Me       string `json:"me"`
	Opponent string `json:"opponent"`
	// when the game was played and both players' rating levels then, if a
	// profile listed the game when it was loaded
	Played   string  `json:"played,omitempty"`
	MyLevel  float64 `json:"my_level,omitempty"`
	OppLevel float64 `json:"opp_level,omitempty"`

	First bool `json:"first"` // Me took the first turn

	Result float64 `json:"result"` // 1 win, 0.5 tie, 0 loss: the row's scores, else the computed VP
	MyVP   int     `json:"my_vp"`
	OppVP  int     `json:"opp_vp"`
	Final  []int   `json:"final,omitempty"` // the row's scores, Me's first, when it has them
	// the computed VP equal the row's scores; when they don't, usually the game's
	// last decision (often the final buy) is missing from the loaded log
	Complete bool   `json:"complete"`
	Ending   string `json:"ending"` // provinces | piles | resign, by the rule below
	Turns    int    `json:"turns"`  // Me's turns

	Kingdom    []string        `json:"kingdom"`
	Landscapes []string        `json:"landscapes"`
	Roles      map[string]bool `json:"roles"` // the kingdom has a card filling the role
	Took       map[string]bool `json:"took"`  // kingdom cards Me owned at some point

	Opening       string   `json:"opening"`        // Coppers in hand on turns 1 and 2, by openingSplit: "4/3", "2/5", "other", "" unknown
	OpeningBuys   []string `json:"opening_buys"`   // cards bought or gained on turns 1-2
	FirstGold     int      `json:"first_gold"`     // Me's turn of the first Gold, 0 if none
	FirstProvince int      `json:"first_province"` // Me's turn of the first Province, 0 if none
	Coins         []int    `json:"coins"`          // Me's $ on each of their turns
}

// GameFeatures computes a saved two-player game's features for both players.
func GameFeatures(client *cartographer.Client, s *Saved, profiles map[string]*Profile) []*Features {
	g := s.Game
	if len(g.Players) != 2 {
		return nil
	}
	turns := g.Turns()
	if len(turns) == 0 {
		return nil
	}
	vp := client.Score(g, cartographer.ScoreInput{Landscapes: client.Landscapes(s.Score)})
	end := ending(g.Supply(client))
	roles := map[string]bool{}
	for _, card := range s.Kingdom {
		var types []string
		if c := client.Cards[card]; c != nil {
			types = c.Types
		}
		for _, r := range CardRoles(card, profiles, types) {
			roles[r] = true
		}
	}
	kingdomPile := map[string]string{} // pile -> kingdom card
	for _, card := range s.Kingdom {
		kingdomPile[client.PileOf(card)] = card
	}
	date, _, _ := strings.Cut(s.Row, " | ")
	played, levels := Header(s.Score, "played"), Levels(s.Score)

	var out []*Features
	for k, me := range g.Players {
		opp := g.Players[1-k]
		f := &Features{ID: s.ID, Date: date, Me: me, Opponent: opp, Played: played, MyLevel: levels[me], OppLevel: levels[opp],
			Kingdom: s.Kingdom, Landscapes: s.Landscapes,
			Roles: roles, Took: map[string]bool{}, Ending: end, MyVP: vp[me].Total(), OppVP: vp[opp].Total()}
		if len(s.Original) > 0 {
			f.First = s.Original[0] == me // seat 1 is the first original player's
		}
		mine, theirs := f.MyVP, f.OppVP
		if a, ok := s.Scores[me]; ok {
			if b, ok := s.Scores[opp]; ok {
				f.Complete = a == f.MyVP && b == f.OppVP
				f.Final = []int{a, b}
				mine, theirs = a, b
			}
		}
		switch {
		case mine > theirs:
			f.Result = 1
		case mine == theirs:
			f.Result = 0.5
		}
		for _, t := range turns {
			for card, n := range t.Decks[me] {
				if kc, ok := kingdomPile[client.PileOf(card)]; ok && n > 0 {
					f.Took[kc] = true
				}
			}
			if t.Player != me || t.Extra != "" {
				continue
			}
			f.Turns++
			if f.Turns == 1 {
				f.Opening = openingSplit(g, t, me, s.Kingdom)
			}
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
		// a log from the other player's side hides the hand; the $ played tells
		// the split when both turns played all 7 Coppers
		if f.Opening == "" && len(f.Coins) >= 2 && f.Coins[0]+f.Coins[1] == 7 {
			f.Opening = fmt.Sprintf("%d/%d", f.Coins[0], f.Coins[1])
		}
		sort.Strings(f.OpeningBuys)
		out = append(out, f)
	}
	return out
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

// startEstates are the cards that can fill a starting deck's 3 Estate slots.
var startEstates = map[string]bool{"Estate": true, "Hovel": true, "Necropolis": true, "Overgrown Estate": true}

// openingSplit is how the 7 starting Coppers fell between the hands of a
// player's turns 1 and 2, in turn order: "5/2", "4/3", "3/4" or "2/5". It
// counts the Coppers the player draws at the end of turn 1 (t), which the log
// names for the player whose log it is, rather than the $ played: a player who
// plays 3 of 4 Coppers for a Silver would look like 3/3. A start other than 7
// Coppers and 3 Estates or Shelters (an Heirloom) or with Coffers (Baker) is
// "other"; a hand the log hides is "" (GameFeatures then tries the $ played).
func openingSplit(g *cartographer.Game, t cartographer.Turn, me string, kingdom []string) string {
	start := map[string]int{} // the deck before turn 1: after it, without what it gained and with what it trashed
	for card, n := range t.Decks[me] {
		start[card] = n
	}
	for _, c := range append(append([]cartographer.CardN{}, t.Bought...), t.Gained...) {
		if c.Card != nil {
			start[c.Card.Name] -= c.N
		}
	}
	for _, c := range t.Trashed {
		if c.Card != nil {
			start[c.Card.Name] += c.N
		}
	}
	estates := 0
	for card, n := range start {
		switch {
		case n <= 0 || card == "Copper": // events bought on turn 1 go below zero
		case startEstates[card]:
			estates += n
		default:
			return "other"
		}
	}
	if start["Copper"] != 7 || estates != 3 || slices.Contains(kingdom, "Baker") {
		return "other"
	}
	drawn, coppers := 0, 0
	for _, e := range g.Events {
		if e.Template != "DRAW" || e.Player != me || e.TurnOf != me || e.Turn != 1 || e.Level != 0 {
			continue
		}
		for _, c := range e.Args[1].Cards {
			if c.Card == nil {
				return ""
			}
			drawn += c.N
			if c.Card.Name == "Copper" {
				coppers += c.N
			}
		}
	}
	if drawn != 5 {
		return ""
	}
	return fmt.Sprintf("%d/%d", 7-coppers, coppers)
}
