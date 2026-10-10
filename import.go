//go:build !js

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourconscience/scrying-pool/cartographer"
)

// exportedGame is one line of the capture script's export (web/capture.js).
type exportedGame struct {
	ID       string   `json:"id"`
	Row      string   `json:"row"`      // e.g. "2026-10-09 | Nikhec: 23, gamrevinu: 32" from DomBot or the profile
	Original []string `json:"original"` // the seats' original players
	Log      string   `json:"log"`
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
		score := "# row: " + g.Row + "\n# original players: " + strings.Join(g.Original, " / ") + "\n" + g.Score
		r := client.Check(g.Log, score)
		if r.MinTurns < minTurns {
			short++
			fmt.Printf("SHORT %s: a player had %d turns, skipped (%s)\n", g.ID, r.MinTurns, g.Row)
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, g.ID+".log"), []byte(g.Log), 0o644); err != nil {
			return err
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
