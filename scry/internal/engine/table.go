// Package engine is a Dominion rules engine for two players. It plays whole
// games and asks an Agent for every choice, so bots can play each other; the
// design is in docs/simulator.md. Card rules live in the cards package, one file
// per card, and register themselves here; names, types and costs come from
// scry/cards/cards.json.
package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CardID is a card kind: an index into the Table.
type CardID uint16

// Def is one card: its facts from the card table and its rules, when written.
type Def struct {
	ID     CardID
	Name   string
	Kind   string // kingdom, basic, other, or a landscape: event, landmark, project, way, ally, trait, prophecy
	Types  []string
	Cost   int
	Potion int
	Debt   int
	Rules  *Rules // nil until the card's rules are written

	action, treasure, victory, attack bool
}

func (d *Def) Action() bool   { return d.action }
func (d *Def) Treasure() bool { return d.treasure }
func (d *Def) Victory() bool  { return d.victory }
func (d *Def) Attack() bool   { return d.attack }

// Is reports whether the card has a type, e.g. "Duration".
func (d *Def) Is(t string) bool {
	for _, x := range d.Types {
		if x == t {
			return true
		}
	}
	return false
}

// Rules are what a card does. Effects run in the order of the card's text.
type Rules struct {
	Play  func(c *Ctx)               // an Action's or Treasure's effect
	Coins int                        // a Treasure's fixed $, added before Play runs
	VP    func(g *Game, p int) int   // worth at the end of the game
	React func(c *Ctx, me Inst) bool // a Reaction in hand when another player plays an Attack; true if it makes the player unaffected
}

var registered = map[string]*Rules{}

// Register adds a card's rules; the cards package calls it from init.
func Register(name string, r Rules) {
	if registered[name] != nil {
		panic("engine: " + name + " registered twice")
	}
	registered[name] = &r
}

// Table is every card, with rules where they are written.
type Table struct {
	Defs   []*Def
	byName map[string]*Def
}

// NewTable reads scry/cards/cards.json and attaches the registered rules.
func NewTable(cardsJSON []byte) (*Table, error) {
	var doc struct {
		Cards []struct {
			Name   string   `json:"name"`
			Kind   string   `json:"kind"`
			Types  []string `json:"types"`
			Cost   string   `json:"cost"`
			Potion int      `json:"potion"`
			Debt   int      `json:"debt"`
		} `json:"cards"`
	}
	if err := json.Unmarshal(cardsJSON, &doc); err != nil {
		return nil, err
	}
	t := &Table{byName: map[string]*Def{}}
	for _, c := range doc.Cards {
		// "4", "3*", "8+": the number is the cost; the marks mean it can change
		cost, _ := strconv.Atoi(strings.TrimRight(c.Cost, "*+"))
		d := &Def{ID: CardID(len(t.Defs)), Name: c.Name, Kind: c.Kind, Types: c.Types, Cost: cost, Potion: c.Potion, Debt: c.Debt}
		d.action, d.treasure, d.victory = d.Is("Action"), d.Is("Treasure"), d.Is("Victory")
		d.attack = d.Is("Attack")
		d.Rules = registered[c.Name]
		t.Defs = append(t.Defs, d)
		t.byName[c.Name] = d
	}
	for name := range registered {
		if t.byName[name] == nil {
			return nil, fmt.Errorf("rules for %q, which isn't in the card table", name)
		}
	}
	return t, nil
}

// Card finds a card by name.
func (t *Table) Card(name string) (*Def, bool) {
	d, ok := t.byName[name]
	return d, ok
}

// MustCard finds a card the engine itself needs, such as Copper.
func (t *Table) MustCard(name string) CardID {
	d, ok := t.byName[name]
	if !ok {
		panic("engine: no card " + name)
	}
	return d.ID
}

// Playable lists the kingdom cards whose rules are written, by name.
func (t *Table) Playable() []CardID {
	var out []CardID
	for _, d := range t.Defs {
		if d.Kind == "kingdom" && d.Rules != nil {
			out = append(out, d.ID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return t.Defs[out[i]].Name < t.Defs[out[j]].Name })
	return out
}
