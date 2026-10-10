package cartographer

import (
	"strconv"
	"strings"
)

// ParseScoreScreen reads the end-of-game screen's text ("You Resigned", "You Lose", ...):
// per player "Turns: N", then VP cards with explanations and the deck as count/card pairs.
func ParseScoreScreen(text string) (turns map[string]int, decks map[string]Deck) {
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
