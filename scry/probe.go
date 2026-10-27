//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yourconscience/scrying-pool/scry/internal/bots"
	"github.com/yourconscience/scrying-pool/scry/internal/engine"
	"github.com/yourconscience/scrying-pool/scry/internal/replay"
)

// The Engine page's data: in each kingdom of a player's latest saved games,
// Big Money with one kingdom card against plain Big Money, next to what the
// player bought there; and how far the engine replays the saved games.
type probeData struct {
	Player string      `json:"player"`
	Made   string      `json:"made"`  // YYYY-MM-DD
	Per    int         `json:"per"`   // games per card
	Games  []probeGame `json:"games"` // latest first
	Replay probeReplay `json:"replay"`
}

type probeGame struct {
	ID       string         `json:"id"`
	Date     string         `json:"date"`
	Opponent string         `json:"opponent"`
	Me       int            `json:"me"`
	Them     int            `json:"them"`
	Cards    []probeCard    `json:"cards"` // the strongest first
	Opening  []string       `json:"opening"`
	Bought   map[string]int `json:"bought"`
}

type probeCard struct {
	Card string  `json:"card"`
	Rate float64 `json:"rate"` // BM+card's share of the games, a tie counting half
}

type probeReplay struct {
	Logs    int `json:"logs"`
	Done    int `json:"done"`
	Turns   int `json:"turns"`
	Matched int `json:"matched"`
}

var reScore = regexp.MustCompile(`([^,|]+?): (-?\d+)`)

// simProbe runs "scry sim probe".
func simProbe(args []string) error {
	fs := flag.NewFlagSet("sim probe", flag.ExitOnError)
	player := fs.String("player", "", "whose saved games")
	last := fs.Int("games", 20, "how many of the latest saved games")
	per := fs.Int("n", 1000, "games of BM+card against BM per kingdom card")
	out := fs.String("o", "", "write the JSON to this file instead of stdout")
	fs.Parse(args)
	if *player == "" {
		return fmt.Errorf("sim probe: -player is required")
	}
	t, err := engine.NewTable(cardsJSON)
	if err != nil {
		return err
	}
	client, err := loadClient()
	if err != nil {
		return err
	}
	dir, err := gamesDir("")
	if err != nil {
		return err
	}
	scores, _ := filepath.Glob(filepath.Join(dir, "*.score.txt"))
	sort.Sort(sort.Reverse(sort.StringSlice(scores))) // game ids grow with time
	data := probeData{Player: *player, Made: time.Now().Format("2006-01-02"), Per: *per, Games: []probeGame{}}
	for _, f := range scores {
		if len(data.Games) == *last {
			break
		}
		id := strings.TrimSuffix(filepath.Base(f), ".score.txt")
		if strings.Contains(id, ".") {
			continue // a bot's copy
		}
		score, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		kingdom, lands, err := replay.KingdomOf(t, string(score))
		if err != nil {
			continue
		}
		g := probeGame{ID: id, Bought: map[string]int{}}
		for _, l := range strings.Split(string(score), "\n") {
			if rest, ok := strings.CutPrefix(l, "# row: "); ok {
				date, sc, _ := strings.Cut(rest, " | ")
				g.Date = date
				for _, m := range reScore.FindAllStringSubmatch(sc, -1) {
					v, _ := strconv.Atoi(m[2])
					if name := strings.TrimSpace(m[1]); name == *player {
						g.Me = v
					} else {
						g.Opponent, g.Them = name, v
					}
				}
			}
		}
		logText, err := os.ReadFile(strings.TrimSuffix(f, ".score.txt") + ".log")
		if err != nil {
			return err
		}
		rg, err := client.Parse(string(logText), nil)
		if err != nil {
			return err
		}
		for _, e := range rg.Events {
			if e.Template != "BUY_AND_GAIN" || e.Player != *player {
				continue
			}
			for _, cn := range e.Args[1].Cards {
				if cn.Card != nil {
					g.Bought[cn.Card.Name] += cn.N
					if e.Turn <= 2 && len(g.Opening) < 2 {
						g.Opening = append(g.Opening, cn.Card.Name)
					}
				}
			}
		}
		for _, k := range kingdom {
			if ms := t.Members(k); len(ms) != 1 || ms[0] != k {
				continue // a mixed pile: no BM+card bot buys it
			}
			rate, err := bmWith(t, k, kingdom, lands, *per)
			if err != nil {
				continue
			}
			g.Cards = append(g.Cards, probeCard{t.Defs[k].Name, rate})
		}
		sort.SliceStable(g.Cards, func(i, j int) bool { return g.Cards[i].Rate > g.Cards[j].Rate })
		data.Games = append(data.Games, g)
		fmt.Fprintf(os.Stderr, "%s %s: %d cards\n", id, g.Date, len(g.Cards))
	}
	logs, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	for _, f := range logs {
		if strings.HasSuffix(f, ".bot.log") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		score, _ := os.ReadFile(strings.TrimSuffix(f, ".log") + ".score.txt")
		r := replay.Check(client, t, string(b), string(score))
		data.Replay.Logs++
		data.Replay.Turns += r.Turns
		data.Replay.Matched += r.Matched
		if r.Done {
			data.Replay.Done++
		}
	}
	b, _ := json.MarshalIndent(data, "", " ")
	if *out == "" {
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	}
	return os.WriteFile(*out, b, 0o644)
}

// bmWith plays n games of Big Money with a card against plain Big Money, the
// two swapping seats every game as in "scry sim play", and gives BM+card's
// share, a tie counting half.
func bmWith(t *engine.Table, card engine.CardID, kingdom, lands []engine.CardID, n int) (float64, error) {
	specs := [2]string{"bm+" + t.Defs[card].Name, "bm"}
	score := 0.0
	for i := range n {
		s := uint64(7 + i)
		seats := [2]int{0, 1}
		if i%2 == 1 {
			seats = [2]int{1, 0}
		}
		var agents [2]engine.Agent
		for p := range agents {
			a, err := bots.New(specs[seats[p]], t, s*2+uint64(p))
			if err != nil {
				return 0, err
			}
			agents[p] = a
		}
		g, err := engine.New(t, engine.Setup{Kingdom: kingdom, Landscapes: lands, Names: simNames, Agents: agents, Seed: s})
		if err != nil {
			return 0, err
		}
		res, err := playSafely(g)
		if err != nil {
			return 0, err
		}
		switch {
		case res.Capped: // unfinished: neither side's
		case res.Winner < 0:
			score += 0.5
		case seats[res.Winner] == 0:
			score++
		}
	}
	return float64(int(score/float64(n)*1000+0.5)) / 1000, nil
}
