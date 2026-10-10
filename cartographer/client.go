// Package cartographer turns dominion.games game logs into events and game state.
//
// It reads the log grammar and the card list from the dominion.games web client
// itself: the client's 244 log templates ("Argument0 buys and gains Argument1.")
// and its card table with singular, plural and noun forms. Those tables belong to
// 2nd Shuffle, so they are not stored in this repo; scry exports them from the
// player's own browser into the user config folder.
package cartographer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Card is one card or landscape as the client knows it.
type Card struct {
	Key       string // the client's English key, e.g. "The Winds Gift"
	Name      string // as shown in logs, e.g. "The Wind's Gift"
	Plural    string // "Hostelries"
	Noun      string // "a Hostelry"
	Expansion string
	Coin      int
	Potion    int
	Debt      int
	Types     []string
}

// notInDeck lists types that are never part of a player's deck: Boons and Hexes
// are received and returned, States and Artifacts are taken, the rest are landscapes.
var notInDeck = map[string]bool{
	"Boon": true, "Hex": true, "State": true, "Artifact": true, "Prophecy": true, "Event": true,
	"Project": true, "Way": true, "Landmark": true, "Ally": true, "Trait": true,
}

// InDeck reports whether the card can be part of a player's deck.
func (c *Card) InDeck() bool {
	for _, t := range c.Types {
		if notInDeck[t] {
			return false
		}
	}
	return true
}

// Client holds the tables read from the dominion.games web client.
type Client struct {
	Version   string
	Templates []*Template
	Cards     map[string]*Card // by Name
	forms     map[string][]form
}

// Template is one log line template, e.g. BUY_AND_GAIN "Argument0 buys and gains Argument1.".
type Template struct {
	Name    string
	English string
	re      *regexp.Regexp
	args    []int // argument numbers in the order they appear
	literal int   // length of the fixed text, to try specific templates first
}

type clientFile struct {
	Source    string `json:"source"`
	Templates []struct {
		Name    string `json:"name"`
		English string `json:"english"`
	} `json:"templates"`
	Names []struct {
		Key      string `json:"key"`
		Singular string `json:"singular"`
		Plural   string `json:"plural"`
		Noun     string `json:"noun"`
	} `json:"names"`
	Cards []struct {
		Key       string `json:"key"`
		Name      string `json:"name"`
		Expansion string `json:"expansion"`
		Cost      *struct {
			Coin   int `json:"coin"`
			Potion int `json:"potion"`
			Debt   int `json:"debt"`
		} `json:"cost"`
		Types []string `json:"types"`
	} `json:"cards"`
}

// DefaultClientPath is where scry keeps the exported client tables:
// $SCRY_CLIENT, or the newest webclient-*.json in <user config>/scrying-pool/client.
func DefaultClientPath() string {
	if p := os.Getenv("SCRY_CLIENT"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	files, _ := filepath.Glob(filepath.Join(dir, "scrying-pool", "client", "webclient-*.json"))
	sort.Strings(files)
	if len(files) == 0 {
		return ""
	}
	return files[len(files)-1]
}

// LoadClient reads exported client tables.
func LoadClient(path string) (*Client, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f clientFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c := &Client{Version: f.Source, Cards: map[string]*Card{}, forms: map[string][]form{}}

	byKey := map[string]*Card{}
	for _, n := range f.Names {
		if n.Singular == "" {
			continue
		}
		card := &Card{Key: n.Key, Name: n.Singular, Plural: n.Plural, Noun: n.Noun}
		byKey[n.Key] = card
		c.Cards[card.Name] = card
	}
	for _, x := range f.Cards {
		card := byKey[x.Name]
		if card == nil {
			continue
		}
		card.Expansion, card.Types = x.Expansion, x.Types
		if x.Cost != nil {
			card.Coin, card.Potion, card.Debt = x.Cost.Coin, x.Cost.Potion, x.Cost.Debt
		}
	}
	for _, card := range c.Cards {
		c.addForm(card.Name, card, false)
		c.addForm(card.Noun, card, false)
		c.addForm(card.Plural, card, true)
	}
	for _, fs := range c.forms {
		sort.Slice(fs, func(i, j int) bool { return len(fs[i].text) > len(fs[j].text) })
	}

	for _, t := range f.Templates {
		if t.English == "" {
			continue
		}
		tm, err := compileTemplate(t.Name, t.English, byKey)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", t.Name, err)
		}
		c.Templates = append(c.Templates, tm)
	}
	sort.SliceStable(c.Templates, func(i, j int) bool { return c.Templates[i].literal > c.Templates[j].literal })
	return c, nil
}

var (
	reArg     = regexp.MustCompile(`Argument(\d+)`)
	reCardRef = regexp.MustCompile(`\[([^\]]+)\]`)
	reSpaces  = regexp.MustCompile(`\s+`)
)

// compileTemplate turns "Argument0 gains Argument1. (Argument2)" into a regexp.
// "[The Skys Gift]" is a card key the client prints as the card's name. Log pieces
// were trimmed when copied, so spaces between words are optional.
func compileTemplate(name, english string, byKey map[string]*Card) (*Template, error) {
	t := &Template{Name: name, English: english}
	var b strings.Builder
	b.WriteString("^")
	rest := english
	for rest != "" {
		loc := reArg.FindStringIndex(rest)
		ref := reCardRef.FindStringIndex(rest)
		next := len(rest)
		if loc != nil {
			next = loc[0]
		}
		if ref != nil && ref[0] < next {
			next = ref[0]
		}
		lit := rest[:next]
		t.literal += len(strings.TrimSpace(lit))
		b.WriteString(reSpaces.ReplaceAllString(regexp.QuoteMeta(lit), `\s*`))
		rest = rest[next:]
		switch {
		case rest == "":
		case ref != nil && ref[0] == next && (loc == nil || ref[0] < loc[0]):
			key := rest[1 : ref[1]-ref[0]-1]
			shown := key
			if c := byKey[key]; c != nil {
				shown = c.Name
			}
			t.literal += len(shown)
			b.WriteString(reSpaces.ReplaceAllString(regexp.QuoteMeta(shown), `\s*`))
			rest = rest[ref[1]-ref[0]:]
		default:
			m := reArg.FindStringSubmatch(rest)
			var n int
			fmt.Sscan(m[1], &n)
			t.args = append(t.args, n)
			b.WriteString(`(.+?)`)
			rest = rest[len(m[0]):]
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, err
	}
	t.re = re
	return t, nil
}

// generic templates are only "Argument0" or "Argument0.", which match anything;
// they are used only where the line kind says so (turn lines, the game header).
func (t *Template) generic() bool { return t.literal <= 1 }

// match returns the template's arguments by number, or nil.
func (t *Template) match(text string) map[int]string {
	m := t.re.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	args := map[int]string{}
	for i, n := range t.args {
		if _, seen := args[n]; !seen {
			args[n] = strings.TrimSpace(m[i+1])
		}
	}
	return args
}

// byKeyName finds a card by the client's key ("The Skys Gift").
func (c *Client) byKeyName(key string) *Card {
	for _, card := range c.Cards {
		if card.Key == key {
			return card
		}
	}
	return nil
}
