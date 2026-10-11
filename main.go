//go:build !js

// scry is the command for Scrying Pool: reviews of finished Dominion Online games,
// plus readable match notes from spectator chat and DomBot results.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourconscience/scrying-pool/cartographer"
)

const usageText = `usage:
  scry web [-addr 127.0.0.1:8765]                  local web UI
  scry format [-dombot FILE | -ask] CHAT_FILE      chat (+ DomBot results) -> Markdown
  scry match PLAYER1 PLAYER2                       ask DomBot "!match PLAYER1, PLAYER2"
  scry ask '<command>'                             send any DomBot command, e.g. '!game 184206813'
  scry cards sync [-kinds kingdom,landscape]       download card art from the Dominion Strategy wiki
  scry bookmarklet [-client]                       print the "Copy log" (or "Copy client tables") bookmark
  scry client save [FILE]                          store client tables copied by the bookmark (stdin if no FILE)
  scry import [-dir DIR] [-min-turns 3] [FILE]     save games exported by the capture script and check each one
  scry check [-prior FILE] [DIR or LOG_FILES]      check saved logs against score screens (and final scores)
  scry review [-names "Bot=Player"] [-json] LOG    turn table and final decks for a copied game log

-ask, match and ask DM DomBot from your Discord account (see README for the token and the ToS risk).
Run "scry <command> -h" for flags.
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
	case "review":
		namesFlag := fs.String("names", "", `rename players, e.g. "Lord Rattington=UtagawaLk" (comma-separated); by default the seats' original players from the saved score screen next to the log`)
		asJSON := fs.Bool("json", false, "print players, turns (with decks, zones and counters), final decks, VP and Supply piles as JSON")
		fs.Parse(args)
		if fs.NArg() != 1 {
			fail(fmt.Errorf("review needs one LOG_FILE"))
		}
		text, rerr := os.ReadFile(fs.Arg(0))
		if rerr != nil {
			fail(rerr)
		}
		names := map[string]string{}
		for _, pair := range strings.Split(*namesFlag, ",") {
			if from, to, ok := strings.Cut(pair, "="); ok {
				names[strings.TrimSpace(from)] = strings.TrimSpace(to)
			}
		}
		cpath := cartographer.DefaultClientPath()
		if cpath == "" {
			fail(fmt.Errorf("no dominion.games client tables found; export them first (see README) or set SCRY_CLIENT"))
		}
		client, cerr := cartographer.LoadClient(cpath)
		if cerr != nil {
			fail(cerr)
		}
		g, perr := client.Parse(string(text), names)
		if perr != nil {
			fail(perr)
		}
		// the saved score screen names the game's landmarks, and a reloaded game's
		// seats belong to its original players: the one who loaded it sits in the
		// first player's seat, the bot in the second's
		var in cartographer.ScoreInput
		if score, err := os.ReadFile(strings.TrimSuffix(fs.Arg(0), ".log") + ".score.txt"); err == nil {
			in.Landscapes = client.Landscapes(string(score))
			if seats := cartographer.SeatNames(g.Players, cartographer.OriginalPlayers(string(score))); seats != nil && *namesFlag == "" {
				if g, perr = client.Parse(string(text), seats); perr != nil {
					fail(perr)
				}
			}
		}
		if *asJSON {
			supply := g.Supply(client)
			b, _ := json.MarshalIndent(map[string]any{"header": g.Header, "players": g.Players, "turns": g.Turns(), "final_decks": g.FinalDecks(),
				"final_vp": client.Score(g, in), "supply_left": supply.Left, "empty_piles": supply.EmptyPiles(), "trash": supply.Trash}, "", " ")
			err = emit(*out, string(b)+"\n")
		} else {
			err = emit(*out, reviewMarkdown(client, g, in))
		}
	case "bookmarklet":
		clientTables := fs.Bool("client", false, `print the "Copy client tables" bookmark instead of "Copy log"`)
		fs.Parse(args)
		file := "web/bookmarklet.js"
		if *clientTables {
			file = "web/bookmarklet-client.js"
		}
		src, _ := webFiles.ReadFile(file)
		var code []string
		for _, l := range strings.Split(string(src), "\n") {
			if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "//") {
				code = append(code, t)
			}
		}
		fmt.Println("javascript:" + url.PathEscape(strings.Join(code, "\n")))
	case "import":
		dir := fs.String("dir", "", "where to write <id>.log and <id>.score.txt (default: <user config>/scrying-pool/games)")
		minTurns := fs.Int("min-turns", 3, "skip games where a player had fewer turns (AFK opponents)")
		fs.Parse(args)
		err = importGames(fs.Arg(0), *dir, *minTurns)
	case "check":
		priorFlag := fs.String("prior", "", "also compare final VP with a saved DomBot !prior list (JSON)")
		fs.Parse(args)
		err = checkGames(fs.Args(), *priorFlag)
	case "client":
		if len(args) == 0 || args[0] != "save" {
			fmt.Fprint(os.Stderr, usageText)
			os.Exit(2)
		}
		fs.Parse(args[1:])
		var b []byte
		var rerr error
		if fs.NArg() == 1 {
			b, rerr = os.ReadFile(fs.Arg(0))
		} else {
			b, rerr = io.ReadAll(os.Stdin)
		}
		if rerr != nil {
			fail(rerr)
		}
		client, cerr := cartographer.ParseClient(b)
		if cerr != nil {
			fail(fmt.Errorf("not client tables from the Copy client tables bookmark: %w", cerr))
		}
		dir, derr := cartographer.ClientDir()
		if derr != nil {
			fail(derr)
		}
		if derr = os.MkdirAll(dir, 0o755); derr != nil {
			fail(derr)
		}
		path := filepath.Join(dir, "webclient-"+client.Version+".json")
		if err = os.WriteFile(path, b, 0o644); err == nil {
			fmt.Printf("saved %d templates and %d cards (client %s) to %s\n", len(client.Templates), len(client.Cards), client.Version, path)
		}
	case "cards":
		kinds := fs.String("kinds", "", "comma-separated card kinds (kingdom, landscape, basic, other); empty means all")
		if len(args) == 0 || args[0] != "sync" {
			fmt.Fprint(os.Stderr, usageText)
			os.Exit(2)
		}
		fs.Parse(args[1:])
		want := map[string]bool{}
		for _, k := range strings.Split(*kinds, ",") {
			if k = strings.TrimSpace(k); k != "" {
				want[k] = true
			}
		}
		err = syncArt(want)
	default:
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	if err != nil {
		fail(err)
	}
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
	fmt.Fprintln(os.Stderr, "scry:", err)
	os.Exit(1)
}
