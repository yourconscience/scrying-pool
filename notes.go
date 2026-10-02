package main

import (
	_ "embed"
	"fmt"
	"strings"
	"time"
)

// Demo match shown by "Load demo match" (spectator names replaced).
//
//go:embed sample/chat.txt
var sampleChat string

//go:embed sample/dombot.txt
var sampleDomBot string

type result struct {
	report
	DomBot string `json:"dombot"` // the DomBot summary text that was used, if any
	Source string `json:"source"` // "pasted", "discord" or "none"
}

// process is shared by the CLI and the web UI.
func process(chatText, botText string, ask bool, timeout time.Duration) (result, error) {
	c, err := parseChat(chatText)
	if err != nil {
		return result{}, err
	}
	res := result{Source: "none"}
	if strings.TrimSpace(botText) != "" {
		res.Source = "pasted"
	} else if ask {
		for _, p := range c.Players {
			if strings.ContainsAny(p, ",\n") || len(p) > 40 {
				return result{}, fmt.Errorf("unexpected player name %q", p)
			}
		}
		msgs, err := askDomBot(fmt.Sprintf("!match %s, %s", c.Players[0], c.Players[1]), timeout)
		if err != nil {
			return result{}, fmt.Errorf("asking DomBot: %w", err)
		}
		botText, res.Source = renderMessages(msgs, time.Local), "discord"
	}
	var s *Summary
	var extra []string
	if strings.TrimSpace(botText) != "" {
		if s, err = parseSummary(botText); err != nil {
			if res.Source == "pasted" {
				return result{}, err
			}
			extra = append(extra, "DomBot replied, but not with a match summary: "+firstLine(botText))
		}
	}
	res.report = buildReport(c, s)
	res.Warnings = append(extra, res.Warnings...)
	res.DomBot = botText
	return res, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
