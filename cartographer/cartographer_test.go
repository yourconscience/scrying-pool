package cartographer

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func loadClient(t *testing.T) *Client {
	t.Helper()
	p := DefaultClientPath()
	if p == "" {
		t.Skip("no exported dominion.games client tables (set SCRY_CLIENT or export them with scry)")
	}
	c, err := LoadClient(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// scoreScreen reads the end-of-game screen saved next to each test log: per
// player "Turns: N", then VP cards (each followed by blank lines) and the deck
// as count/card pairs.
func scoreScreen(text string) (turns map[string]int, decks map[string]Deck) {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, "#") {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	turns, decks = map[string]int{}, map[string]Deck{}
	var players []string
	for i, l := range lines {
		if n, ok := strings.CutPrefix(l, "Turns: "); ok && i > 0 {
			players = append(players, lines[i-1])
			if v, _ := strconv.Atoi(n); v > 0 {
				turns[lines[i-1]] = v
			}
		}
	}
	isPlayer := func(s string) bool {
		for _, p := range players {
			if p == s {
				return true
			}
		}
		return false
	}
	// Each player's block lists VP cards with explanations, separated by blank
	// lines, then the whole deck as count/card pairs after the last blank line.
	// A stray value line (e.g. Wall's penalty) can sit just before the deck.
	var starts []int
	for i, l := range lines {
		if isPlayer(l) && (i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "Turns: ")) {
			starts = append(starts, i)
		}
	}
	isNum := func(s string) bool { _, err := strconv.Atoi(s); return err == nil }
	for k, st := range starts {
		end := len(lines)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		block := lines[st+1 : end]
		for len(block) > 0 && block[len(block)-1] == "" {
			block = block[:len(block)-1]
		}
		last := -1
		for i, l := range block {
			if l == "" {
				last = i
			}
		}
		deck := block[last+1:]
		if len(deck) >= 2 && isNum(deck[0]) && isNum(deck[1]) {
			deck = deck[1:]
		}
		d := Deck{}
		for i := 0; i+1 < len(deck); i += 2 {
			n, err := strconv.Atoi(deck[i])
			if err != nil {
				break
			}
			d[deck[i+1]] += n
		}
		decks[lines[st]] = d
	}
	return turns, decks
}

// Every saved log must be read completely, and must end with the decks its
// score screen shows. The bot that took the opponent's seat keeps its name in
// both files.
func TestLogsMatchScoreScreens(t *testing.T) {
	c := loadClient(t)
	logs, _ := filepath.Glob("../testdata/logs/*.log")
	if len(logs) == 0 {
		t.Fatal("no test logs")
	}
	for _, lp := range logs {
		id := strings.TrimSuffix(filepath.Base(lp), ".log")
		t.Run(id, func(t *testing.T) {
			b, _ := os.ReadFile(lp)
			g, err := c.Parse(string(b), nil)
			if err != nil {
				t.Fatal(err)
			}
			for i, u := range g.Unknown {
				if i == 5 {
					t.Errorf("... %d unknown lines in all", len(g.Unknown))
					break
				}
				t.Errorf("unknown line %d: %s", u.Line, u.Text)
			}
			sb, err := os.ReadFile(strings.TrimSuffix(lp, ".log") + ".score.txt")
			if err != nil {
				return // older logs have no score screen
			}
			wantTurns, wantDecks := scoreScreen(string(sb))
			last := map[string]int{}
			for _, e := range g.Events {
				if e.Template == "NEW_TURN" {
					last[e.Player] = e.Turn
				}
			}
			if !reflect.DeepEqual(last, wantTurns) {
				t.Errorf("last turns %v, score screen says %v", last, wantTurns)
			}
			got := g.FinalDecks()
			for p, want := range wantDecks {
				have, ok := got[p]
				if !ok {
					have = got[p[:1]] // a player with no turns is known only by initial
				}
				if !reflect.DeepEqual(have, want) {
					t.Errorf("%s deck:\n got  %v\n want %v", p, have, want)
				}
			}
		})
	}
}
