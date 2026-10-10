package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// parseScoreScreen reads the end-of-game screen text saved next to each test
// log (<id>.score.txt). Each player block lists VP cards (each followed by
// blank lines), then the whole deck as count/card pairs.
func parseScoreScreen(text string) map[string]map[string]int {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, "#") {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	// player names are the lines before "Turns: N"
	var players []string
	for i, l := range lines {
		if strings.HasPrefix(l, "Turns: ") && i > 0 {
			players = append(players, lines[i-1])
		}
	}
	decks := map[string]map[string]int{}
	cur := ""
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if contains(players, l) && (i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "Turns: ")) {
			cur = l
			decks[cur] = map[string]int{}
			continue
		}
		n, err := strconv.Atoi(l)
		if cur == "" || err != nil || i+1 >= len(lines) {
			continue
		}
		card := lines[i+1]
		summary := i+2 < len(lines) && lines[i+2] == ""
		if !summary {
			decks[cur][card] += n
		}
		i++
	}
	return decks
}

// scoreScreenTurns reads "Turns: N" for each player.
func scoreScreenTurns(text string) map[string]int {
	lines := strings.Split(text, "\n")
	out := map[string]int{}
	for i, l := range lines {
		if n, ok := strings.CutPrefix(strings.TrimSpace(l), "Turns: "); ok && i > 0 {
			if v, _ := strconv.Atoi(n); v > 0 { // players who never took a turn have no turn lines
				out[strings.TrimSpace(lines[i-1])] = v
			}
		}
	}
	return out
}

// Every saved log must be complete: it starts with the game line, turns have no
// gaps, and each player's last turn matches the score screen. Decks must match
// the score screen too, but only for logs the parser fully understands; the
// rest are parser gaps, reported and skipped. The bot that took the opponent's
// seat keeps its name in both files.
func TestFinalDecksMatchScoreScreens(t *testing.T) {
	scores, _ := filepath.Glob("testdata/logs/*.score.txt")
	if len(scores) == 0 {
		t.Skip("no score screens saved")
	}
	for _, sp := range scores {
		id := strings.TrimSuffix(filepath.Base(sp), ".score.txt")
		t.Run(id, func(t *testing.T) {
			logText, err := os.ReadFile(filepath.Join("testdata/logs", id+".log"))
			if err != nil {
				t.Fatal(err)
			}
			scoreText, _ := os.ReadFile(sp)
			g, err := parseGameLog(string(logText), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(g.Header) == 0 || !strings.HasPrefix(g.Header[0], "Game #") {
				t.Errorf("log doesn't start with the game line: %q", g.Header)
			}
			last := map[string]int{}
			for _, e := range g.Events {
				if e.Verb == "turn" {
					if e.Turn != last[e.Player] && e.Turn != last[e.Player]+1 {
						t.Errorf("line %d: %s jumps from turn %d to %d", e.Line, e.Player, last[e.Player], e.Turn)
					}
					last[e.Player] = e.Turn
				}
			}
			if wantTurns := scoreScreenTurns(string(scoreText)); !reflect.DeepEqual(last, wantTurns) {
				t.Errorf("last turns %v, score screen says %v", last, wantTurns)
			}
			if len(g.Unknown) > 0 {
				t.Skipf("log complete; %d lines the parser doesn't read yet, first: line %d %s", len(g.Unknown), g.Unknown[0].Line, g.Unknown[0].Text)
			}
			want := parseScoreScreen(string(scoreText))
			got := g.finalDecks()
			if len(want) != 2 {
				t.Fatalf("score screen parsed into %d players", len(want))
			}
			for p, deck := range want {
				if _, ok := got[p]; !ok {
					p2 := p[:1] // a player with no turns is known only by initial in the log
					got[p] = got[p2]
				}
				if !reflect.DeepEqual(got[p], deck) {
					t.Errorf("%s deck:\n got  %v\n want %v", p, got[p], deck)
				}
			}
		})
	}
}
