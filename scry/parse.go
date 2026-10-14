package main

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Chat copied from the dominion.games chat box after a match.

type chatLine struct {
	Who, Text string // Who is empty for system lines
}

type chatGame struct {
	Winner string
	Score  [2]int // match score after this game, in Chat.Players order
	Lines  []chatLine
}

type Chat struct {
	MatchID string
	Players [2]string
	Games   []chatGame
	Tail    []chatLine // lines after the last result (match not finished or extra talk)
}

var (
	reMatchCreated = regexp.MustCompile(`^Match #(\d+) created$`)
	reChatScore    = regexp.MustCompile(`^Match(?: completed)?: (.+?) (\d+), (.+?) (\d+)(?:\s+[—-].*)?$`)
)

func parseChat(text string) (*Chat, error) {
	c := &Chat{}
	var cur []chatLine
	prev := map[string]int{}
	lines := nonEmptyLines(text)
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if m := reMatchCreated.FindStringSubmatch(l); m != nil {
			c.MatchID = m[1]
			continue
		}
		if m := reChatScore.FindStringSubmatch(l); m != nil {
			a, _ := strconv.Atoi(m[2])
			b, _ := strconv.Atoi(m[4])
			if c.Players[0] == "" {
				c.Players = [2]string{m[1], m[3]}
			}
			score := map[string]int{m[1]: a, m[3]: b}
			g := chatGame{Score: [2]int{score[c.Players[0]], score[c.Players[1]]}}
			for _, p := range c.Players {
				if score[p] > prev[p] {
					g.Winner = p
				}
			}
			prev = score
			// players' "gg" right after the result belongs to the game that just ended
			for i+1 < len(lines) {
				cl := splitChatLine(lines[i+1])
				if !c.isPlayer(cl.Who) || !strings.HasPrefix(strings.ToLower(cl.Text), "gg") {
					break
				}
				cur = append(cur, cl)
				i++
			}
			g.Lines = cur
			c.Games = append(c.Games, g)
			cur = nil
			continue
		}
		cur = append(cur, splitChatLine(l))
	}
	c.Tail = cur
	if len(c.Games) == 0 {
		return nil, errors.New(`no game results found; the chat should contain lines like "Match: Alice 1, Bob 0"`)
	}
	return c, nil
}

func (c *Chat) isPlayer(who string) bool {
	return who != "" && (who == c.Players[0] || who == c.Players[1])
}

func splitChatLine(l string) chatLine {
	if i := strings.Index(l, ": "); i > 0 && i <= 40 {
		return chatLine{Who: l[:i], Text: strings.TrimSpace(l[i+2:])}
	}
	return chatLine{Text: l}
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// DomBot's "!match a, b" summary, as plain text (copied from Discord or rendered by renderMessages).

type gameResult struct {
	Points       map[string]int
	Winner       string
	End          string
	Turns        string
	ID, Duration string
	Cards        []string
	Empty        string
}

type Summary struct {
	Title, Score, Begin, Total string
	Games                      []gameResult
}

var (
	reBotScore      = regexp.MustCompile(`^(.+?): (-?\d+), (.+?): (-?\d+) \((.+?), ([\d.]+) turns?\)$`)
	reBotDetail     = regexp.MustCompile(`^(\d+) \((\w+)\)\s+—\s+(.+?)(?:\s+—\s+Empty piles: (.+))?$`)
	reBotMatchScore = regexp.MustCompile(`^\d+\s*[–-]\s*\d+$`)
)

func parseSummary(text string) (*Summary, error) {
	s := &Summary{}
	lines := nonEmptyLines(strings.ReplaceAll(text, "−", "-"))
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "Games beginning "):
			s.Begin = strings.TrimPrefix(l, "Games beginning ")
		case strings.HasPrefix(l, "Total time: "):
			s.Total = strings.TrimPrefix(l, "Total time: ")
		case reBotScore.MatchString(l):
			m := reBotScore.FindStringSubmatch(l)
			a, _ := strconv.Atoi(m[2])
			b, _ := strconv.Atoi(m[4])
			g := gameResult{Points: map[string]int{m[1]: a, m[3]: b}, End: m[5], Turns: m[6], Winner: m[1]}
			if b > a {
				g.Winner = m[3]
			}
			if g.End == "Resigned" {
				loser := m[1]
				if g.Winner == m[1] {
					loser = m[3]
				}
				g.End = loser + " resigned"
			}
			if i+1 < len(lines) {
				if d := reBotDetail.FindStringSubmatch(lines[i+1]); d != nil {
					g.ID, g.Duration, g.Empty = d[1], d[2], d[4]
					for _, card := range strings.Split(d[3], ",") {
						g.Cards = append(g.Cards, strings.TrimSpace(card))
					}
					i++
				}
			}
			s.Games = append(s.Games, g)
		case s.Title == "" && strings.Contains(l, " v. "):
			s.Title = l
		case s.Title != "" && s.Score == "" && len(s.Games) == 0 && reBotMatchScore.MatchString(l):
			s.Score = l
		}
	}
	if len(s.Games) == 0 {
		return nil, errors.New(`no games found in the DomBot summary; expected lines like "Alice: 30, Bob: 20 (Provinces, 15.5 turns)"`)
	}
	return s, nil
}

// align finds the DomBot games that correspond to the chat's games. DomBot's
// "!match" can list more games (older matches between the same players), so we
// look for the latest run of games whose winners match the chat, in order.
func align(c *Chat, s *Summary) ([]*gameResult, bool) {
	n := len(c.Games)
	for start := len(s.Games) - n; start >= 0; start-- {
		ok := true
		for j := 0; j < n && ok; j++ {
			ok = s.Games[start+j].Winner == c.Games[j].Winner
		}
		if ok {
			out := make([]*gameResult, n)
			for j := range out {
				out[j] = &s.Games[start+j]
			}
			return out, true
		}
	}
	return make([]*gameResult, n), false
}
