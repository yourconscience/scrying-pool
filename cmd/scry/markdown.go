package main

import (
	"fmt"
	"strings"
)

var mdEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;",
	`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`,
	"#", `\#`, "|", `\|`, "~", `\~`,
)

// md escapes text so chat like "***sigh***" or "<b>" shows literally.
func md(s string) string { return mdEscaper.Replace(s) }

type report struct {
	Markdown string     `json:"markdown"`
	Warnings []string   `json:"warnings"`
	Games    []*Kingdom `json:"games"` // per chat game; null when DomBot results are missing
}

// buildReport renders the chat (and DomBot results, if any) as Markdown.
func buildReport(c *Chat, s *Summary) report {
	var warn []string
	results := make([]*gameResult, len(c.Games))
	if s != nil {
		var ok bool
		if results, ok = align(c, s); !ok {
			warn = append(warn, fmt.Sprintf("DomBot's games don't match the chat's %d results (winners differ or games are missing), so game details are left out. DomBot's !match only covers the last day or two.", len(c.Games)))
		}
	}

	kingdoms := make([]*Kingdom, len(c.Games))
	for i, r := range results {
		if r != nil && len(r.Cards) > 0 {
			k := classify(r.Cards)
			kingdoms[i] = &k
		}
	}

	a, b := c.Players[0], c.Players[1]
	last := c.Games[len(c.Games)-1].Score
	var w strings.Builder
	fmt.Fprintf(&w, "# %s vs %s: %d-%d\n\n", md(a), md(b), last[0], last[1])

	var meta []string
	if c.MatchID != "" {
		meta = append(meta, "Match #"+c.MatchID)
	}
	if s != nil && s.Begin != "" {
		meta = append(meta, "started "+md(s.Begin))
	}
	if s != nil && s.Total != "" {
		meta = append(meta, md(s.Total)+" total")
	}
	seq := make([]string, len(c.Games))
	for i, g := range c.Games {
		seq[i] = md(g.Winner)
	}
	meta = append(meta, "winners: "+strings.Join(seq, ", "))
	w.WriteString(strings.Join(meta, " · ") + "\n\n")
	for _, x := range warn {
		w.WriteString("> **Note:** " + md(x) + "\n\n")
	}

	// summary table
	if results[0] != nil {
		fmt.Fprintf(&w, "| # | Winner | %s | %s | End | Turns | Length | Game ID |\n|--:|---|--:|--:|---|--:|--:|---|\n", md(a), md(b))
		for i, r := range results {
			fmt.Fprintf(&w, "| %d | %s | %d | %d | %s | %s | %s | `%s` |\n",
				i+1, md(r.Winner), r.Points[a], r.Points[b], md(r.End), r.Turns, r.Duration, r.ID)
		}
	} else {
		fmt.Fprintf(&w, "| # | Winner | Match score (%s-%s) |\n|--:|---|---|\n", md(a), md(b))
		for i, g := range c.Games {
			fmt.Fprintf(&w, "| %d | %s | %d-%d |\n", i+1, md(g.Winner), g.Score[0], g.Score[1])
		}
	}
	w.WriteString("\n**Bold** names are the players; everyone else is a spectator.\n")

	for i, g := range c.Games {
		fmt.Fprintf(&w, "\n## Game %d: %s wins (match %d-%d)\n\n", i+1, md(g.Winner), g.Score[0], g.Score[1])
		if r := results[i]; r != nil {
			fmt.Fprintf(&w, "- **Result:** %s %d, %s %d (%s, %s turns, %s)\n", md(a), r.Points[a], md(b), r.Points[b], md(r.End), r.Turns, r.Duration)
			if k := kingdoms[i]; k != nil {
				writeKingdom(&w, k)
			}
			if r.Empty != "" {
				w.WriteString("- **Empty piles:** " + md(r.Empty) + "\n")
			}
			if r.ID != "" {
				w.WriteString("- **Game ID:** `" + r.ID + "`\n")
			}
			w.WriteString("\n")
		}
		writeChat(&w, c, g.Lines)
	}
	if len(c.Tail) > 0 {
		w.WriteString("\n## After the last result\n\n")
		writeChat(&w, c, c.Tail)
	}
	return report{Markdown: w.String(), Warnings: warn, Games: kingdoms}
}

func writeKingdom(w *strings.Builder, k *Kingdom) {
	var cards, lands []string
	for _, c := range k.Cards {
		cards = append(cards, md(c.Name)+" ("+costLabel(c)+")")
	}
	for _, c := range k.Landscapes {
		lands = append(lands, md(c.Name)+" ("+c.Types[0]+")")
	}
	if len(cards) > 0 {
		w.WriteString("- **Kingdom:** " + strings.Join(cards, ", ") + "\n")
	}
	if len(lands) > 0 {
		w.WriteString("- **Landscapes:** " + strings.Join(lands, ", ") + "\n")
	}
	if len(k.Unknown) > 0 {
		w.WriteString("- **Not in the card list:** " + md(strings.Join(k.Unknown, ", ")) + "\n")
	}
}

func writeChat(w *strings.Builder, c *Chat, lines []chatLine) {
	if len(lines) == 0 {
		w.WriteString("_No chat._\n")
		return
	}
	w.WriteString("**Chat**\n\n")
	for _, l := range lines {
		switch {
		case l.Who == "":
			w.WriteString("- _" + md(l.Text) + "_\n")
		case c.isPlayer(l.Who):
			w.WriteString("- **" + md(l.Who) + ":** " + md(l.Text) + "\n")
		default:
			w.WriteString("- " + md(l.Who) + ": " + md(l.Text) + "\n")
		}
	}
}
