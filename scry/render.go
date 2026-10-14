package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type user struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Fields      []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"fields"`
	Footer *struct {
		Text string `json:"text"`
	} `json:"footer"`
}

type message struct {
	ID        string         `json:"id"`
	Author    user           `json:"author"`
	Content   string         `json:"content"`
	Embeds    []discordEmbed `json:"embeds"`
	Timestamp string         `json:"timestamp"`
}

var (
	reTimestamp = regexp.MustCompile(`<t:(-?\d+)(?::([tTdDfFR]))?>`)
	reEmoji     = regexp.MustCompile(`<a?:(\w+):\d+>`)
	reLink      = regexp.MustCompile(`\[([^\]]+)\]\(<?https?://[^)>]+>?\)`)
	reEscape    = regexp.MustCompile(`\\([\\*_~|>\x60\[\]()#-])`)
)

// renderMessages turns DomBot replies into plain text that matches what you get
// when copying the embed from the Discord app.
func renderMessages(msgs []message, loc *time.Location) string {
	var parts []string
	for _, m := range msgs {
		if s := clean(m.Content, loc); s != "" {
			parts = append(parts, s)
		}
		for _, e := range m.Embeds {
			var b []string
			for _, s := range []string{e.Title, e.Description} {
				if s = clean(s, loc); s != "" {
					b = append(b, s)
				}
			}
			for _, f := range e.Fields {
				b = append(b, clean(f.Name, loc), clean(f.Value, loc))
			}
			if e.Footer != nil && strings.TrimSpace(e.Footer.Text) != "" {
				b = append(b, clean(e.Footer.Text, loc))
			}
			parts = append(parts, strings.Join(b, "\n"))
		}
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// clean strips Discord markdown and resolves timestamps and custom emoji.
func clean(s string, loc *time.Location) string {
	s = reTimestamp.ReplaceAllStringFunc(s, func(t string) string {
		m := reTimestamp.FindStringSubmatch(t)
		sec, _ := strconv.ParseInt(m[1], 10, 64)
		return formatStamp(time.Unix(sec, 0).In(loc), m[2])
	})
	s = reEmoji.ReplaceAllString(s, ":$1:")
	s = reLink.ReplaceAllString(s, "$1")

	// protect escaped characters, drop markdown markers, then restore
	const ph = "\x00"
	var kept []string
	s = reEscape.ReplaceAllStringFunc(s, func(e string) string {
		kept = append(kept, e[1:])
		return ph
	})
	for _, mk := range []string{"**", "__", "~~", "||", "*"} {
		s = strings.ReplaceAll(s, mk, "")
	}
	for _, k := range kept {
		s = strings.Replace(s, ph, k, 1)
	}

	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// formatStamp mirrors how the Discord client renders <t:...:style> (en-US).
func formatStamp(t time.Time, style string) string {
	switch style {
	case "t":
		return t.Format("3:04 PM")
	case "T":
		return t.Format("3:04:05 PM")
	case "d":
		return t.Format("1/2/2006")
	case "D":
		return t.Format("January 2, 2006")
	case "F":
		return t.Format("Monday, January 2, 2006 at 3:04 PM")
	default: // "f", "R" and no style
		return t.Format("January 2, 2006 at 3:04 PM")
	}
}
