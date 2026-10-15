//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
)

// buildSite writes the "My games" page as a static site that any host can serve,
// with one JSON file per player profile. Profiles carry win rates only: no game
// list and no opponents' names.
//
//	out/index.html     the page
//	out/recent.html    the first player's latest games (Recent games)
//	out/*.css, *.js    their assets
//	out/data/players.json, out/data/<player>.json, out/data/recent.json
//
// With data set, the profiles are copied from that folder (a players.json, the
// files it lists and recent.json) instead of being made from the saved games, so
// a CI job can rebuild the pages without them.
func buildSite(out string, players []string, data string) error {
	if err := os.MkdirAll(filepath.Join(out, "data"), 0o755); err != nil {
		return err
	}
	var n int
	var err error
	if data != "" {
		n, err = copyProfiles(data, filepath.Join(out, "data"))
	} else {
		n, err = makeProfiles(filepath.Join(out, "data"), players)
	}
	if err != nil {
		return err
	}
	for _, f := range []string{"style.css", "insights.css", "insights.js", "recent.css", "recent.js"} {
		b, err := webFiles.ReadFile("web/" + f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, f), b, 0o644); err != nil {
			return err
		}
	}
	for name, dst := range map[string]string{"insights.html": filepath.Join(out, "index.html"), "recent.html": filepath.Join(out, "recent.html")} {
		page, err := webFiles.ReadFile("web/" + name)
		if err != nil {
			return err
		}
		html := strings.Replace(string(page), `<meta name="scry-data" content="api">`, `<meta name="scry-data" content="static">`, 1)
		if err := os.WriteFile(dst, []byte(html), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("wrote %s: %d profiles\n", out, n)
	return nil
}

type siteEntry struct {
	Name  string `json:"name"`
	Games int    `json:"games"`
	File  string `json:"file"`
}

type siteList struct {
	Default string      `json:"default"`
	Players []siteEntry `json:"players"`
}

// makeProfiles makes each player's profile from the saved games.
func makeProfiles(dir string, players []string) (int, error) {
	client, err := loadClient()
	if err != nil {
		return 0, err
	}
	games, err := gamesDir("")
	if err != nil {
		return 0, err
	}
	account, _, features, err := advisor.Corpus(client, games)
	if err != nil {
		return 0, err
	}
	if len(players) == 0 {
		players = []string{account}
	}
	list := siteList{Default: players[0]}
	for _, p := range players {
		mine := advisor.Of(p, features)
		if len(mine) == 0 {
			return 0, fmt.Errorf("no saved games of %q", p)
		}
		in := advisor.Summarize(p, mine)
		in.Games = nil
		// the static page links the images where KingdomCreator publishes them
		// rather than copying them: they belong to Rio Grande Games
		in.Images = cardImages(in, func(_, rel string) string { return kingdomCreatorImages + rel })
		file := "data/" + slug(p) + ".json"
		b, _ := json.Marshal(in)
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(file)), b, 0o644); err != nil {
			return 0, err
		}
		list.Players = append(list.Players, siteEntry{p, len(mine), file})
	}
	// the first player's latest games, without what only the local app shows
	recent := advisor.LatestGames(players[0], features, recentGames)
	for i, g := range recent.Games {
		f := *g.Features
		f.Kingdom, f.Landscapes, f.Roles, f.Took, f.OpeningBuys, f.Coins = nil, nil, nil, nil, nil, nil
		recent.Games[i].Features = &f
	}
	b, _ := json.Marshal(recent)
	if err := os.WriteFile(filepath.Join(dir, "recent.json"), b, 0o644); err != nil {
		return 0, err
	}
	b, _ = json.MarshalIndent(list, "", " ")
	return len(list.Players), os.WriteFile(filepath.Join(dir, "players.json"), b, 0o644)
}

// copyProfiles copies players.json and the profiles it lists from src to dir.
func copyProfiles(src, dir string) (int, error) {
	b, err := os.ReadFile(filepath.Join(src, "players.json"))
	if err != nil {
		return 0, err
	}
	var list siteList
	if err := json.Unmarshal(b, &list); err != nil {
		return 0, fmt.Errorf("%s/players.json: %w", src, err)
	}
	files := []string{"players.json"}
	for _, p := range list.Players {
		files = append(files, filepath.Base(p.File))
	}
	if _, err := os.Stat(filepath.Join(src, "recent.json")); err == nil {
		files = append(files, "recent.json")
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
			return 0, err
		}
	}
	return len(list.Players), nil
}

// kingdomCreatorImages is where the files in card-images/ come from, pinned to
// the commit they were copied at.
const kingdomCreatorImages = "https://raw.githubusercontent.com/blakevanlan/KingdomCreator/d9a802db2e41d52786eaa02097279cbf1b64c778/docs/img/cards/"

var reSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	s := strings.Trim(reSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		s = "player"
	}
	return s
}
