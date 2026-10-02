// dombot turns dominion.games match chat into readable Markdown notes, optionally
// enriched with game results fetched from DomBot over your own Discord DM.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const usageText = `usage:
  dombot web [-addr 127.0.0.1:8765]                  local web UI
  dombot format [-dombot FILE | -ask] CHAT_FILE      chat (+ DomBot results) -> Markdown
  dombot match PLAYER1 PLAYER2                       ask DomBot "!match PLAYER1, PLAYER2"
  dombot ask '<command>'                             send any DomBot command, e.g. '!game 184206813'

-ask, match and ask DM DomBot from your Discord account (see README for the token and the ToS risk).
Run "dombot <command> -h" for flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	out := fs.String("o", "", "write output to this file instead of stdout")
	timeout := fs.Duration("timeout", 45*time.Second, "how long to wait for DomBot's reply")
	var err error
	switch cmd {
	case "web":
		addr := fs.String("addr", "127.0.0.1:8765", "listen address (keep it on localhost)")
		fs.Parse(args)
		err = serve(*addr, *timeout)
	case "format":
		botFile := fs.String("dombot", "", "file with DomBot's !match summary (copied from Discord)")
		ask := fs.Bool("ask", false, "fetch the summary from DomBot over Discord DM")
		fs.Parse(args)
		if fs.NArg() != 1 {
			fail(fmt.Errorf("format needs one CHAT_FILE"))
		}
		chat, rerr := os.ReadFile(fs.Arg(0))
		if rerr != nil {
			fail(rerr)
		}
		var bot []byte
		if *botFile != "" {
			if bot, rerr = os.ReadFile(*botFile); rerr != nil {
				fail(rerr)
			}
		}
		var res result
		if res, err = process(string(chat), string(bot), *ask, *timeout); err == nil {
			for _, w := range res.Warnings {
				fmt.Fprintln(os.Stderr, "warning:", w)
			}
			err = emit(*out, res.Markdown)
		}
	case "match", "ask":
		raw := fs.String("raw", "", "also save DomBot's raw reply JSON to this file")
		fs.Parse(args)
		var command string
		switch {
		case cmd == "match" && fs.NArg() == 2:
			command = fmt.Sprintf("!match %s, %s", fs.Arg(0), fs.Arg(1))
		case cmd == "ask" && fs.NArg() == 1:
			command = fs.Arg(0)
		default:
			fmt.Fprint(os.Stderr, usageText)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "sending %q to DomBot...\n", command)
		msgs, aerr := askDomBot(command, *timeout)
		if aerr != nil {
			fail(aerr)
		}
		if *raw != "" {
			b, _ := json.MarshalIndent(msgs, "", "  ")
			if err = os.WriteFile(*raw, b, 0o644); err != nil {
				fail(err)
			}
		}
		err = emit(*out, renderMessages(msgs, time.Local))
	default:
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	if err != nil {
		fail(err)
	}
}

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

func emit(path, text string) error {
	if path == "" {
		_, err := fmt.Print(text)
		return err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "wrote", path)
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "dombot:", err)
	os.Exit(1)
}
