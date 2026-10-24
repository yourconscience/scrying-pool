// Package engine is a Dominion rules engine for two players. It plays whole
// games and asks an Agent for every choice, so bots can play each other; the
// design is in docs/simulator.md. Card rules live in the cards package, one file
// per set, and register themselves here; names, types and costs come from
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

// Types is a card's types as bits.
type Types uint64

const (
	TAction Types = 1 << iota
	TTreasure
	TVictory
	TCurse
	TAttack
	TReaction
	TDuration
	TNight
	TCommand
	TKnight
	TRuins
	TShelter
	TLooter
	TPrize
	TReward
	TReserve
	TTraveller
	TGathering
	TCastle
	TDoom
	TFate
	THeirloom
	TSpirit
	TZombie
	TLiaison
	TAugur
	TClash
	TFort
	TOdyssey
	TTownsfolk
	TWizard
	TLoot
	TOmen
	TShadow
	TEvent
	TLandmark
	TProject
	TWay
	TAlly
	TTrait
	TProphecy
	TBoon
	THex
	TState
	TArtifact
)

var typeBits = map[string]Types{
	"Action": TAction, "Treasure": TTreasure, "Victory": TVictory, "Curse": TCurse, "Attack": TAttack,
	"Reaction": TReaction, "Duration": TDuration, "Night": TNight, "Command": TCommand, "Knight": TKnight,
	"Ruins": TRuins, "Shelter": TShelter, "Looter": TLooter, "Prize": TPrize, "Reward": TReward,
	"Reserve": TReserve, "Traveller": TTraveller, "Gathering": TGathering, "Castle": TCastle, "Doom": TDoom,
	"Fate": TFate, "Heirloom": THeirloom, "Spirit": TSpirit, "Zombie": TZombie, "Liaison": TLiaison,
	"Augur": TAugur, "Clash": TClash, "Fort": TFort, "Odyssey": TOdyssey, "Townsfolk": TTownsfolk,
	"Wizard": TWizard, "Loot": TLoot, "Omen": TOmen, "Shadow": TShadow, "Event": TEvent,
	"Landmark": TLandmark, "Project": TProject, "Way": TWay, "Ally": TAlly, "Trait": TTrait,
	"Prophecy": TProphecy, "Boon": TBoon, "Hex": THex, "State": TState, "Artifact": TArtifact,
}

// Cost is what a card costs: coins, Potions and debt.
type Cost struct{ Coins, Potion, Debt int }

// LE: a costs at most b ("costs up to").
func (a Cost) LE(b Cost) bool { return a.Coins <= b.Coins && a.Potion <= b.Potion && a.Debt <= b.Debt }

// Less: a costs less than b.
func (a Cost) Less(b Cost) bool { return a.LE(b) && a != b }

func (a Cost) Plus(n int) Cost { a.Coins += n; return a }

// Upto is a cost of $n.
func Upto(n int) Cost { return Cost{Coins: n} }

// Def is one card: its facts from the card table and its rules, when written.
type Def struct {
	ID    CardID
	Name  string
	Kind  string // kingdom, basic, other, or a landscape: event, landmark, project, way, ally, trait, prophecy
	Types []string
	T     Types
	Cost  Cost
	Rules *Rules // nil until the card's rules are written
}

func (d *Def) Action() bool   { return d.T&TAction != 0 }
func (d *Def) Treasure() bool { return d.T&TTreasure != 0 }
func (d *Def) Victory() bool  { return d.T&TVictory != 0 }
func (d *Def) Attack() bool   { return d.T&TAttack != 0 }

// Is reports whether the card has any of the types.
func (d *Def) Is(t Types) bool { return d.T&t != 0 }

// NTypes counts the card's types.
func (d *Def) NTypes() int { return len(d.Types) }

// Landscape: an Event, Landmark, Project, Way, Ally, Trait or Prophecy.
func (d *Def) Landscape() bool {
	return d.Is(TEvent | TLandmark | TProject | TWay | TAlly | TTrait | TProphecy)
}

// Rules are what a card does. Every func is static: what a card remembers
// between calls is data in the Game (a Dur, a Watch, a counter), so games can be copied.
type Rules struct {
	Play  func(c *Ctx)             // an Action's, Treasure's or Night card's effect; a Way's, a Boon's, a Hex's
	Coins int                      // a Treasure's fixed $, added before Play runs
	Worth func(g *Game, p int) int // a Treasure's $ when it depends on the game (Bank), instead of Coins
	// Alone is a Treasure with a choice: its play line stands alone, without its $ ("plays an Anvil.")
	Alone bool
	VP    func(g *Game, p int) int  // worth at the end of the game; a Landmark's for each player
	VPAt  func(g *Game, x Inst) int // worth by where this copy is (Distant Lands on the Tavern mat)

	// Next is a Duration's effect at the start of a later turn; it returns true
	// when the card is done and leaves play in that turn's Clean-up.
	Next func(c *Ctx, d *Dur) bool

	// "When you gain/buy/trash this", "when you discard this from play".
	OnGain    func(c *Ctx, ev *Ev)
	OnBuy     func(c *Ctx, ev *Ev)
	OnTrash   func(c *Ctx, ev *Ev)
	OnDiscard func(c *Ctx, ev *Ev) // discarded from play in Clean-up, or discarded other than in Clean-up (ev.From)

	// React is a card in hand its owner may reveal (or play) when an event in
	// Reacts happens and CanReact allows it. It returns true when it did.
	Reacts   EvMask
	CanReact func(c *Ctx, ev *Ev) bool
	React    func(c *Ctx, ev *Ev) bool

	// Hear is a card in play, set aside, on a mat, in Exile, or a landscape, a
	// State or an Artifact, that sees the events in Hears.
	Hears EvMask
	Hear  func(c *Ctx, ev *Ev)

	// Watch is an effect a card left for later in the turn, e.g. Merchant's +$1
	// for the first Silver; it returns true when it is used up.
	Watch func(c *Ctx, ev *Ev, w *Watch) bool

	// SelfCost changes the card's own cost (Peddler); Reduce makes other cards
	// cheaper while this one is in play (Highway).
	SelfCost func(g *Game, p int, cost *Cost)
	Reduce   func(g *Game, me Inst, c CardID) int
	// CanBuy forbids buying the card (Grand Market with a Copper in play).
	CanBuy func(g *Game, p int) bool

	// Overpay is what paying $n more than a card's cost when buying it does,
	// after the card is gained (Masterpiece's Silvers).
	Overpay func(c *Ctx, n int)

	// Buy is an Event's effect, or a Project's when bought.
	Buy func(c *Ctx)
	// Setup prepares the game for the card: piles it needs, tokens. Start
	// runs when the game starts, before the starting decks, for what the log shows then.
	Setup func(g *Game)
	Start func(g *Game, me CardID)
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

type cardFacts struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	Types  []string `json:"types"`
	Cost   string   `json:"cost"`
	Potion int      `json:"potion"`
	Debt   int      `json:"debt"`
}

// NewTable reads scry/cards/cards.json, adds the cards it lacks (pile members
// such as the Knights), and attaches the registered rules.
func NewTable(cardsJSON []byte) (*Table, error) {
	var doc struct {
		Cards []cardFacts `json:"cards"`
	}
	if err := json.Unmarshal(cardsJSON, &doc); err != nil {
		return nil, err
	}
	t := &Table{byName: map[string]*Def{}}
	t.add(cardFacts{Name: "", Kind: "none"}) // CardID 0 is no card
	for _, c := range append(doc.Cards, extraCards...) {
		if t.byName[c.Name] == nil {
			t.add(c)
		}
	}
	for name := range registered {
		if t.byName[name] == nil {
			return nil, fmt.Errorf("rules for %q, which isn't in the card table", name)
		}
	}
	return t, nil
}

func (t *Table) add(c cardFacts) {
	// "4", "3*", "8+": the number is the cost; the marks mean it can change
	cost, _ := strconv.Atoi(strings.TrimRight(c.Cost, "*+"))
	d := &Def{ID: CardID(len(t.Defs)), Name: c.Name, Kind: c.Kind, Types: c.Types, Cost: Cost{cost, c.Potion, c.Debt}}
	for _, ty := range c.Types {
		d.T |= typeBits[ty]
	}
	d.Rules = registered[c.Name]
	t.Defs = append(t.Defs, d)
	t.byName[c.Name] = d
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

// Playable lists the kingdom piles and landscapes whose rules are all written, by name.
func (t *Table) Playable() []CardID {
	var out []CardID
	for _, d := range t.Defs {
		if (d.Kind == "kingdom" || d.Landscape()) && t.HasRules(d.ID) {
			out = append(out, d.ID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return t.Defs[out[i]].Name < t.Defs[out[j]].Name })
	return out
}

// HasRules: the card, and every card of its pile, has rules.
func (t *Table) HasRules(c CardID) bool {
	d := t.Defs[c]
	if m, ok := pileMembers[d.Name]; ok {
		for _, x := range m {
			if t.byName[x.name] == nil || t.byName[x.name].Rules == nil {
				return false
			}
		}
		return true
	}
	return d.Rules != nil
}
