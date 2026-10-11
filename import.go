//go:build !js

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yourconscience/scrying-pool/cartographer"
)

// exportedGame is one line of the capture script's export (web/capture.js).
type exportedGame struct {
	ID       string   `json:"id"`
	Row      string   `json:"row"`      // e.g. "2026-10-09 | Nikhec: 23, gamrevinu: 32" from DomBot or the profile
	Original []string `json:"original"` // the seats' original players
	Log      string   `json:"log"`
	Bot      string   `json:"bot"`      // lines the bot played after the load; the score screen counts them
	Decision int      `json:"decision"` // the decision the game was loaded at
	Markers  bool     `json:"markers"`  // whether the client's decision markers were read
	Score    string   `json:"score"`
}

// importGames saves exported games and checks each one against its score screen.
func importGames(file, dir string, minTurns int) error {
	var in io.Reader = os.Stdin
	if file != "" && file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(base, "scrying-pool", "games")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cpath := cartographer.DefaultClientPath()
	if cpath == "" {
		return fmt.Errorf("no dominion.games client tables found; export them first (see README)")
	}
	client, err := cartographer.LoadClient(cpath)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	var pass, fail, short int
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var g exportedGame
		if err := json.Unmarshal([]byte(line), &g); err != nil {
			return fmt.Errorf("not a capture export line: %w", err)
		}
		score := "# row: " + g.Row + "\n# original players: " + strings.Join(g.Original, " / ") + "\n"
		if g.Markers {
			score += fmt.Sprintf("# loaded at decision %d; bot lines cut: %d\n", g.Decision, strings.Count(g.Bot, "\n"))
		}
		score += g.Score
		r := client.Check(g.Log+g.Bot, score)
		if r.MinTurns < minTurns {
			short++
			fmt.Printf("SHORT %s: a player had %d turns, skipped (%s)\n", g.ID, r.MinTurns, g.Row)
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, g.ID+".log"), []byte(g.Log), 0o644); err != nil {
			return err
		}
		if g.Bot != "" {
			if err := os.WriteFile(filepath.Join(dir, g.ID+".bot.log"), []byte(g.Bot), 0o644); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(dir, g.ID+".score.txt"), []byte(score), 0o644); err != nil {
			return err
		}
		if r.OK() {
			pass++
			fmt.Printf("PASS  %s: %d lines, turns %v (%s)\n", g.ID, r.Lines, r.Turns, g.Row)
		} else {
			fail++
			fmt.Printf("FAIL  %s: %s (%s)\n", g.ID, strings.Join(r.Problems, "; "), g.Row)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	fmt.Printf("%d passed, %d failed, %d short games skipped; files in %s\n", pass, fail, short, dir)
	return nil
}

// checkGames runs the cartographer check on saved logs: each <id>.log with its
// <id>.score.txt, from a folder (default: the saved games) or a list of files.
// priorScores reads final scores from a saved DomBot !prior list:
// [{"id": "184479482", "result": "Nikhec: 23, gamrevinu: 32"}, ...].
func priorScores(path string) (map[string]map[string]int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID     string `json:"id"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`^(.+): (-?\d+), (.+): (-?\d+)$`)
	out := map[string]map[string]int{}
	for _, r := range rows {
		// DomBot's minus sign comes through as "−" or, in copied text, as "?": "R4D4: ?5" is -5
		m := re.FindStringSubmatch(strings.NewReplacer("\u2212", "-", "?", "-").Replace(r.Result))
		if m == nil {
			continue
		}
		a, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[4])
		out[r.ID] = map[string]int{m[1]: a, m[3]: b}
	}
	return out, nil
}

func checkGames(args []string, priorPath string) error {
	var prior map[string]map[string]int
	if priorPath != "" {
		var err error
		if prior, err = priorScores(priorPath); err != nil {
			return err
		}
	}
	cpath := cartographer.DefaultClientPath()
	if cpath == "" {
		return fmt.Errorf("no dominion.games client tables found; export them first (see README)")
	}
	client, err := cartographer.LoadClient(cpath)
	if err != nil {
		return err
	}
	var logs []string
	if len(args) == 0 {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		args = []string{filepath.Join(base, "scrying-pool", "games")}
	}
	for _, a := range args {
		if st, err := os.Stat(a); err == nil && st.IsDir() {
			found, _ := filepath.Glob(filepath.Join(a, "*.log"))
			for _, f := range found {
				if !strings.HasSuffix(f, ".bot.log") {
					logs = append(logs, f)
				}
			}
		} else {
			logs = append(logs, a)
		}
	}
	var pass, fail, noScore, scored, scoreFail int
	for _, lp := range logs {
		id := strings.TrimSuffix(filepath.Base(lp), ".log")
		b, err := os.ReadFile(lp)
		if err != nil {
			return err
		}
		sb, err := os.ReadFile(strings.TrimSuffix(lp, ".log") + ".score.txt")
		if err != nil {
			noScore++
			continue
		}
		bot, _ := os.ReadFile(strings.TrimSuffix(lp, ".log") + ".bot.log") // the score screen counts the bot's moves
		r := client.Check(string(b)+string(bot), string(sb))
		problems := r.Problems
		if final, ok := prior[id]; ok {
			scored++
			_, sp := client.CheckScore(string(b), string(sb), final)
			problems = append(problems, sp...)
			if len(sp) > 0 {
				scoreFail++
			}
		}
		if len(problems) == 0 {
			pass++
		} else {
			fail++
			fmt.Printf("FAIL %s: %s\n", id, strings.Join(problems, "; "))
		}
	}
	fmt.Printf("%d passed, %d failed, %d without a score screen (of %d logs)\n", pass, fail, noScore, len(logs))
	if prior != nil {
		fmt.Printf("final scores: %d of %d games match the known scores\n", scored-scoreFail, scored)
	}
	return nil
}
