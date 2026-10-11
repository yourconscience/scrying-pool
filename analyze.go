//go:build !js

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yourconscience/scrying-pool/cartographer"
)

// The game view is what the analysis page shows for one saved game: every turn
// with its log, the players' zones, VP and the Supply after it.

type logPart struct {
	Text   string   `json:"text"`
	Player string   `json:"player,omitempty"` // the piece is a player's initial
	Cards  []string `json:"cards,omitempty"`  // the piece is a card list
}

type logLine struct {
	Level int       `json:"level"`
	Parts []logPart `json:"parts"`
}

type turnView struct {
	cartographer.Turn
	VP     map[string]int `json:"vp"`     // every player's VP after the turn
	Supply map[string]int `json:"supply"` // cards left in each pile after the turn
	Log    []logLine      `json:"log"`
}

type gameView struct {
	ID       string                              `json:"id"`
	Row      string                              `json:"row"`     // "date | result", when the capture knew it
	Account  string                              `json:"account"` // the account that loaded the game
	Header   []string                            `json:"header"`
	Players  []string                            `json:"players"`  // the original players, in turn order
	Problems []string                            `json:"problems"` // where the log and the score screen disagree
	Setup    []logLine                           `json:"setup"`
	Turns    []turnView                          `json:"turns"`
	FinalVP  map[string]cartographer.VPBreakdown `json:"final_vp"`
	Piles    map[string]int                      `json:"piles"` // starting size of each pile the game touched
	Cards    map[string]Card                     `json:"cards"` // every card the view names
}

type savedGame struct {
	ID       string   `json:"id"`
	Row      string   `json:"row"`
	Original []string `json:"original"`
}

// listGames lists the saved games, newest first.
func listGames(dir string) ([]savedGame, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.score.txt"))
	if err != nil {
		return nil, err
	}
	out := []savedGame{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(f), ".score.txt")
		out = append(out, savedGame{ID: id, Row: scoreRow(string(b)), Original: cartographer.OriginalPlayers(string(b))})
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i].ID)
		b, _ := strconv.Atoi(out[j].ID)
		return a > b
	})
	return out, nil
}

// scoreRow reads "# row: ..." from a saved score screen.
func scoreRow(score string) string {
	for _, l := range strings.Split(score, "\n") {
		if v, ok := strings.CutPrefix(l, "# row: "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func loadGameView(client *cartographer.Client, dir, id string) (*gameView, error) {
	if !reGameID.MatchString(id) {
		return nil, os.ErrNotExist
	}
	base := filepath.Join(dir, id)
	text, err := os.ReadFile(base + ".log")
	if err != nil {
		return nil, err
	}
	score, _ := os.ReadFile(base + ".score.txt")
	bot, _ := os.ReadFile(base + ".bot.log") // the score screen counts the bot's moves
	g, err := client.Parse(string(text), nil)
	if err != nil {
		return nil, err
	}
	v := &gameView{ID: id, Row: scoreRow(string(score)), Header: g.Header, Cards: map[string]Card{}}
	if len(g.Players) > 0 {
		v.Account = g.Players[0]
	}
	v.Problems = client.Check(string(text)+string(bot), string(score)).Problems
	if seats := cartographer.SeatNames(g.Players, cartographer.OriginalPlayers(string(score))); seats != nil {
		if g, err = client.Parse(string(text), seats); err != nil {
			return nil, err
		}
	}
	v.Players = g.Players
	in := cartographer.ScoreInput{Landscapes: client.Landscapes(string(score))}
	byTurn, final := scoresByTurn(client, g, in)
	v.FinalVP = final
	setup, logs := turnLogs(client, g, string(text))
	v.Setup = setup
	ends := turnEnds(g)
	for i, t := range g.Turns() {
		tv := turnView{Turn: t, VP: map[string]int{}}
		if i < len(logs) {
			tv.Log = logs[i]
		}
		for p, b := range byTurn[i] {
			tv.VP[p] = b.Total()
		}
		prefix := *g
		prefix.Events = g.Events[:ends[i]]
		tv.Supply = prefix.Supply(client).Left
		v.Turns = append(v.Turns, tv)
	}
	v.Piles = g.Supply(client).Initial

	add := func(name string) {
		if _, ok := v.Cards[name]; ok || name == "" {
			return
		}
		if c, ok := lookupCard(name); ok {
			v.Cards[name] = c
		} else if c := client.Cards[name]; c != nil {
			v.Cards[name] = Card{Name: c.Name, Kind: "other", Types: c.Types, Cost: strconv.Itoa(c.Coin), Potion: c.Potion, Debt: c.Debt, Set: c.Expansion}
		}
	}
	for _, ls := range append([][]logLine{setup}, logs...) {
		for _, l := range ls {
			for _, p := range l.Parts {
				for _, c := range p.Cards {
					add(c)
				}
			}
		}
	}
	for pile := range v.Piles {
		add(pile)
	}
	for _, t := range v.Turns {
		for _, d := range t.Decks {
			for c := range d {
				add(c)
			}
		}
		for _, z := range t.Zones {
			for _, cs := range z {
				for c := range cs.Known {
					add(c)
				}
			}
		}
	}
	return v, nil
}

var reTurnLine = regexp.MustCompile(`^Turn \d+ - `)

// turnLogs splits a copied log into the lines before turn 1 and each turn's
// lines, with player initials and card lists marked for display.
func turnLogs(client *cartographer.Client, g *cartographer.Game, text string) (setup []logLine, turns [][]logLine) {
	cur := &setup
	for _, l := range strings.Split(text, "\n")[1:] {
		head, body, ok := strings.Cut(l, "\t")
		if !ok || strings.TrimSpace(body) == "" {
			continue
		}
		kind := strings.TrimLeft(head, "0123456789")
		level, _ := strconv.Atoi(strings.TrimSuffix(head, kind))
		if kind == "T" && reTurnLine.MatchString(body) {
			turns = append(turns, nil)
			cur = &turns[len(turns)-1]
			continue
		}
		pieces := strings.Split(body, "|")
		line := logLine{Level: level}
		for i, s := range pieces {
			p := logPart{Text: s}
			if name, ok := g.Initials[s]; ok && i == 0 && len(pieces) > 1 {
				p.Player = name
			} else if cs, ok := client.CardList(s); ok {
				for _, c := range cs {
					if c.Card != nil {
						p.Cards = append(p.Cards, c.Card.Name)
					}
				}
			}
			line.Parts = append(line.Parts, p)
		}
		*cur = append(*cur, line)
	}
	return setup, turns
}
