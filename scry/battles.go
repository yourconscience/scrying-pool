//go:build !js

package main

import (
	"encoding/json"
	"net/url"
	"os"
	"slices"
	"sort"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

// scoutBattles is the Scout page's Battles panel: a scouted player's listed
// games against anyone, newest first (docs/scout-battles.md).
type scoutBattles struct {
	Player string            `json:"player"` // as dominion.games spells it
	Me     string            `json:"me"`
	Games  []battle          `json:"games"`
	Images map[string]string `json:"images"` // kingdom cards and landscapes that have an image
}

// battle is one listed game from the scouted player's side. What the list
// doesn't carry comes from the saved log, nil when there is none.
type battle struct {
	ID       string     `json:"id"`
	Time     string     `json:"time"`
	Opponent string     `json:"opponent"`
	Level    float64    `json:"level,omitempty"`
	OppLevel float64    `json:"opp_level,omitempty"`
	Score    int        `json:"score"`
	OppScore int        `json:"opp_score"`
	Result   float64    `json:"result"` // 1 won, 0.5 tied, 0 lost
	Log      *battleLog `json:"log,omitempty"`
}

type battleLog struct {
	First      bool       `json:"first"`    // the scouted player went first
	Turns      int        `json:"turns"`    // the larger of the two players' turns
	Ending     string     `json:"ending"`   // provinces | colonies | piles | resign
	Complete   bool       `json:"complete"` // the log's VP equal the listed scores, so the decks are whole
	Kingdom    []string   `json:"kingdom"`
	Landscapes []string   `json:"landscapes"`
	Deck       []deckCard `json:"deck"`
	OppDeck    []deckCard `json:"opp_deck"`
}

// deckCard is a card of a final deck, in the order the page shows them.
type deckCard struct {
	Card string `json:"card"`
	N    int    `json:"n"`
	Kind string `json:"kind"` // victory | treasure | action | curse
}

var kindOrder = []string{"victory", "treasure", "action", "curse"}

func deckCards(client *cartographer.Client, d cartographer.Deck) []deckCard {
	out := []deckCard{}
	for card, n := range d {
		if n <= 0 {
			continue
		}
		kind := "action"
		if c := client.Cards[card]; c != nil {
			switch {
			case slices.Contains(c.Types, "Curse"):
				kind = "curse"
			case slices.Contains(c.Types, "Victory"):
				kind = "victory"
			case slices.Contains(c.Types, "Treasure"):
				kind = "treasure"
			}
		}
		out = append(out, deckCard{card, n, kind})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Kind != b.Kind {
			return slices.Index(kindOrder, a.Kind) < slices.Index(kindOrder, b.Kind)
		}
		if a.N != b.N {
			return a.N > b.N
		}
		return a.Card < b.Card
	})
	return out
}

// battlesOf lists the games in the player's scout file, nil without one.
func battlesOf(client *cartographer.Client, dir string, features []*advisor.Features, player string) (*scoutBattles, error) {
	b, err := os.ReadFile(scoutFile(dir, player))
	if err != nil {
		return nil, nil
	}
	var d scoutData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	listed := map[string]bool{}
	for _, g := range d.Games {
		listed[g.ID] = true
	}
	mine, theirs := map[string]*advisor.Features{}, map[string]*advisor.Features{}
	var ids []string
	for _, f := range features {
		switch {
		case !listed[f.ID]:
		case f.Me == d.Player:
			mine[f.ID] = f
			ids = append(ids, f.ID)
		default:
			theirs[f.ID] = f
		}
	}
	saved, err := advisor.LoadGames(client, dir, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]*advisor.Saved{}
	for _, s := range saved {
		byID[s.ID] = s
	}

	out := &scoutBattles{Player: d.Player, Me: d.Me, Games: []battle{}, Images: map[string]string{}}
	for _, g := range d.Games {
		bt := battle{ID: g.ID, Time: g.Time, Level: g.Levels[d.Player], Score: g.Scores[d.Player]}
		for _, p := range g.Players {
			if p != d.Player {
				bt.Opponent, bt.OppLevel, bt.OppScore = p, g.Levels[p], g.Scores[p]
			}
		}
		switch {
		case bt.Score > bt.OppScore:
			bt.Result = 1
		case bt.Score == bt.OppScore:
			bt.Result = 0.5
		}
		if f, s := mine[g.ID], byID[g.ID]; f != nil && s != nil {
			l := &battleLog{First: f.First, Turns: f.Turns, Ending: f.Ending, Complete: f.Complete, Kingdom: append([]string{}, f.Kingdom...), Landscapes: append([]string{}, f.Landscapes...)}
			if t := theirs[g.ID]; t != nil {
				l.Turns = max(l.Turns, t.Turns)
			}
			// Province or Colony: both count as "provinces" in Features.Ending
			if sup := s.Game.Supply(client); l.Ending == "provinces" && sup.Left["Colony"] <= 1 && sup.Initial["Colony"] > 0 && (sup.Initial["Province"] == 0 || sup.Left["Province"] > 1) {
				l.Ending = "colonies"
			}
			decks := s.Game.FinalDecks()
			l.Deck, l.OppDeck = deckCards(client, decks[f.Me]), deckCards(client, decks[f.Opponent])
			for _, name := range append(slices.Clone(l.Kingdom), l.Landscapes...) {
				if _, ok := cardImage(name); ok {
					out.Images[name] = "/card-image/" + url.PathEscape(name)
				}
			}
			bt.Log = l
		}
		out.Games = append(out.Games, bt)
	}
	return out, nil
}
