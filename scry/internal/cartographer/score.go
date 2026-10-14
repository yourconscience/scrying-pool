package cartographer

import (
	"strconv"
	"strings"
)

// ParseScoreScreen reads the end-of-game screen's text ("You Resigned", "You Lose", ...):
// per player "Turns: N", then VP cards with explanations and the deck as count/card pairs.
// isCard tells deck entries from card-text fragments in tooltips ("+2 Action"); nil accepts any name.
func ParseScoreScreen(text string, isCard func(string) bool) (turns map[string]int, decks map[string]Deck) {
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
	// Each player's block lists VP cards with explanations of varying shape
	// (Gardens, Wall, Pasture...), then the whole deck as count/card pairs. The
	// deck is the longest run of such pairs at the end of the block.
	var starts []int
	for i, l := range lines {
		if isPlayer(l) && (i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "Turns: ")) {
			starts = append(starts, i)
		}
	}
	isNum := func(s string) bool { _, err := strconv.Atoi(s); return err == nil }
	isPair := func(b []string, i int) bool {
		return i >= 0 && i+1 < len(b) && isNum(b[i]) && b[i+1] != "" && !isNum(b[i+1]) && (isCard == nil || isCard(b[i+1]))
	}
	for k, st := range starts {
		end := len(lines)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		block := lines[st+1 : end]
		for len(block) >= 2 && !isPair(block, len(block)-2) { // trailing buttons ("Ready", "Leave Table") and blanks
			block = block[:len(block)-1]
		}
		i := len(block) - 2
		for isPair(block, i-2) {
			i -= 2
		}
		d := Deck{}
		for ; isPair(block, i); i += 2 {
			n, _ := strconv.Atoi(block[i])
			d[block[i+1]] += n
		}
		decks[lines[st]] = d
	}
	return turns, decks
}

// ScoreScreenSources lists the names in the score screen's VP section: every VP
// card, landmark or ally that scored, e.g. "Province", "Wall", "Plateau Shepherds".
func ScoreScreenSources(text string) []string {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, "#") {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	seen := map[string]bool{}
	var out []string
	for i := 0; i+2 < len(lines); i++ {
		if _, err := strconv.Atoi(lines[i]); err != nil {
			continue
		}
		name := lines[i+1]
		if name == "" || strings.HasPrefix(name, "Turns: ") || lines[i+2] != "" {
			continue
		}
		if _, err := strconv.Atoi(name); err == nil {
			continue
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}
