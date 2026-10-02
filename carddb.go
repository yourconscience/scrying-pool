package main

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Card facts from cards/cards.json (regenerate with: go run ./cmd/carddb).

type Card struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"` // kingdom, basic, other, or a landscape: event, landmark, project, way, ally, trait, prophecy
	Types  []string `json:"types"`
	Cost   string   `json:"cost,omitempty"`
	Potion int      `json:"potion,omitempty"`
	Debt   int      `json:"debt,omitempty"`
	Set    string   `json:"set"`
	Art    string   `json:"art"`
}

func (c Card) Landscape() bool {
	switch c.Kind {
	case "event", "landmark", "project", "way", "ally", "trait", "prophecy":
		return true
	}
	return false
}

//go:embed cards/cards.json
var cardsJSON []byte

var cardIndex = func() map[string]Card {
	var doc struct {
		Cards []Card `json:"cards"`
	}
	if err := json.Unmarshal(cardsJSON, &doc); err != nil {
		panic(err)
	}
	m := make(map[string]Card, len(doc.Cards))
	for _, c := range doc.Cards {
		m[cardKey(c.Name)] = c
	}
	return m
}()

func cardKey(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "’", "'"))
}

// lookupCard finds a card as DomBot names it. Split piles ("Encampment/Plunder")
// resolve to their top card, keep the pile name, and count as kingdom piles.
func lookupCard(name string) (Card, bool) {
	if c, ok := cardIndex[cardKey(name)]; ok {
		return c, true
	}
	top, _, split := strings.Cut(name, "/")
	if c, ok := cardIndex[cardKey(top)]; split && ok {
		c.Name = name
		c.Kind = "kingdom"
		return c, true
	}
	return Card{}, false
}

// Kingdom is one game's supply as DomBot lists it, sorted the way the
// dominion.games client lays it out: kingdom piles by cost, then landscapes.
type Kingdom struct {
	Cards      []Card   `json:"cards"`
	Landscapes []Card   `json:"landscapes"`
	Unknown    []string `json:"unknown"`
}

func classify(names []string) Kingdom {
	k := Kingdom{Cards: []Card{}, Landscapes: []Card{}, Unknown: []string{}}
	for _, n := range names {
		c, ok := lookupCard(n)
		switch {
		case !ok:
			k.Unknown = append(k.Unknown, n)
		case c.Landscape():
			k.Landscapes = append(k.Landscapes, c)
		default:
			k.Cards = append(k.Cards, c)
		}
	}
	sort.SliceStable(k.Cards, func(i, j int) bool { return costLess(k.Cards[i], k.Cards[j]) })
	sort.SliceStable(k.Landscapes, func(i, j int) bool {
		a, b := k.Landscapes[i], k.Landscapes[j]
		if a.Kind != b.Kind {
			return landscapeOrder(a.Kind) < landscapeOrder(b.Kind)
		}
		return costLess(a, b)
	})
	return k
}

func coins(c Card) int {
	n, _ := strconv.Atoi(strings.TrimRight(c.Cost, "*+"))
	return n
}

func costLess(a, b Card) bool {
	if coins(a) != coins(b) {
		return coins(a) < coins(b)
	}
	if a.Debt != b.Debt {
		return a.Debt < b.Debt
	}
	if a.Potion != b.Potion {
		return a.Potion < b.Potion
	}
	return a.Name < b.Name
}

func landscapeOrder(kind string) int {
	return strings.Index("event project landmark way ally trait prophecy", kind)
}

// costLabel renders a cost the way players write it: $5, $3P, 6D, $8*.
func costLabel(c Card) string {
	var s string
	if c.Cost != "" {
		s = "$" + c.Cost
	}
	if c.Potion > 0 {
		s += "P"
	}
	if c.Debt > 0 {
		if s != "" {
			s += " "
		}
		s += strconv.Itoa(c.Debt) + "D"
	}
	return s
}
