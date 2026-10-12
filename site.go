//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yourconscience/scrying-pool/advisor"
)

// buildSite writes the "My games" page as a static site that any host can serve,
// with one JSON file per player profile. Profiles carry win rates only: no game
// list and no opponents' names.
//
//	out/index.html     the page
//	out/artifact.html  the same page without the html/head/body wrapper
//	out/*.css, *.js    its assets
//	out/data/players.json, out/data/<player>.json
func buildSite(out string, players []string) error {
	client, err := loadClient()
	if err != nil {
		return err
	}
	dir, err := gamesDir("")
	if err != nil {
		return err
	}
	account, _, features, err := advisor.Corpus(client, dir)
	if err != nil {
		return err
	}
	if len(players) == 0 {
		players = []string{account}
	}
	if err := os.MkdirAll(filepath.Join(out, "data"), 0o755); err != nil {
		return err
	}
	type entry struct {
		Name  string `json:"name"`
		Games int    `json:"games"`
		File  string `json:"file"`
	}
	list := struct {
		Default string  `json:"default"`
		Players []entry `json:"players"`
	}{Default: players[0]}
	for _, p := range players {
		mine := advisor.Of(p, features)
		if len(mine) == 0 {
			return fmt.Errorf("no saved games of %q", p)
		}
		in := advisor.Summarize(p, mine)
		in.Games = nil
		file := "data/" + slug(p) + ".json"
		b, _ := json.Marshal(in)
		if err := os.WriteFile(filepath.Join(out, file), b, 0o644); err != nil {
			return err
		}
		list.Players = append(list.Players, entry{p, len(mine), file})
	}
	b, _ := json.MarshalIndent(list, "", " ")
	if err := os.WriteFile(filepath.Join(out, "data", "players.json"), b, 0o644); err != nil {
		return err
	}
	for _, f := range []string{"style.css", "insights.css", "insights.js"} {
		b, err := webFiles.ReadFile("web/" + f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, f), b, 0o644); err != nil {
			return err
		}
	}
	page, err := webFiles.ReadFile("web/insights.html")
	if err != nil {
		return err
	}
	html := strings.Replace(string(page), `<meta name="scry-data" content="api">`, `<meta name="scry-data" content="static">`, 1)
	if err := os.WriteFile(filepath.Join(out, "index.html"), []byte(html), 0o644); err != nil {
		return err
	}
	var kept []string
	for _, l := range strings.Split(html, "\n") {
		switch strings.TrimSpace(l) {
		case "<!doctype html>", `<html lang="en">`, "<head>", "</head>", "<body>", "</body>", "</html>",
			`<meta charset="utf-8">`, `<meta name="viewport" content="width=device-width, initial-scale=1">`:
			continue
		}
		kept = append(kept, l)
	}
	if err := os.WriteFile(filepath.Join(out, "artifact.html"), []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d profiles\n", out, len(list.Players))
	return nil
}

var reSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	s := strings.Trim(reSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		s = "player"
	}
	return s
}
