// carddb regenerates scry/cards/cards.json from the dominiontabs card database
// (https://github.com/sumpfork/dominiontabs, MIT). Only game facts are kept:
// names, types, costs and sets. Card text and images belong to Rio Grande Games
// and are not copied.
//
// Run from the repo root: go run ./tools/carddb
package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const src = "https://raw.githubusercontent.com/sumpfork/dominiontabs/%s/src/domdiv/card_db/"

// typeFix corrects types dominiontabs gets wrong.
var typeFix = map[string][]string{"Barbarian": {"Action", "Attack"}, "Prince": {"Action", "Duration"}}

type rawCard struct {
	Tag        string   `json:"card_tag"`
	Sets       []string `json:"cardset_tags"`
	Cost       string   `json:"cost"`
	PotCost    string   `json:"potcost"`
	DebtCost   string   `json:"debtcost"`
	Randomizer *bool    `json:"randomizer"`
	Types      []string `json:"types"`
}

type Card struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	Types  []string `json:"types"`
	Cost   string   `json:"cost,omitempty"` // coins, may end in * or +
	Potion int      `json:"potion,omitempty"`
	Debt   int      `json:"debt,omitempty"`
	Set    string   `json:"set"`
	Art    string   `json:"art"` // Dominion Strategy wiki art file, e.g. "RoninArt.jpg"
}

var landscapes = []string{"Event", "Landmark", "Project", "Way", "Ally", "Trait", "Prophecy"}

var basic = map[string]bool{
	"Copper": true, "Silver": true, "Gold": true, "Platinum": true, "Potion": true,
	"Estate": true, "Duchy": true, "Province": true, "Colony": true, "Curse": true,
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "carddb:", err)
		os.Exit(1)
	}
}

func run() error {
	sha, err := exec.Command("gh", "api", "repos/sumpfork/dominiontabs/commits/master", "--jq", ".sha").Output()
	if err != nil {
		return fmt.Errorf("resolving dominiontabs commit: %w", err)
	}
	commit := strings.TrimSpace(string(sha))
	base := fmt.Sprintf(src, commit)

	var raw []rawCard
	var names map[string]struct {
		Name string `json:"name"`
	}
	var sets map[string]struct {
		SetName string `json:"set_name"`
	}
	for path, v := range map[string]any{"cards_db.json.gz": &raw, "en_us/cards_en_us.json.gz": &names, "en_us/sets_en_us.json.gz": &sets} {
		if err := fetchGz(base+path, v); err != nil {
			return err
		}
	}

	byName := map[string]*Card{}
	for _, r := range raw {
		name := names[r.Tag].Name
		if name == "" {
			name = r.Tag
		}
		if _, dup := byName[name]; dup {
			continue // same card in several editions: keep the first entry
		}
		if t, ok := typeFix[name]; ok {
			r.Types = t
		}
		c := &Card{Name: name, Types: r.Types, Kind: kind(r, name)}
		c.Cost = r.Cost
		c.Potion, _ = strconv.Atoi(r.PotCost)
		c.Debt, _ = strconv.Atoi(r.DebtCost)
		if c.Cost == "0" && (c.Debt > 0 || c.Potion > 0) {
			c.Cost = ""
		}
		if len(r.Sets) > 0 {
			c.Set = setName(sets[r.Sets[0]].SetName, r.Sets[0])
		}
		c.Art = strings.ReplaceAll(strings.ReplaceAll(name, "’", "'"), " ", "_") + "Art.jpg"
		byName[name] = c
	}
	out := make([]*Card, 0, len(byName))
	for _, c := range byName {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	doc := map[string]any{
		"source": "https://github.com/sumpfork/dominiontabs/tree/" + commit + " (MIT; names, types, costs and sets only)",
		"cards":  out,
	}
	b, _ := json.MarshalIndent(doc, "", " ")
	if err := os.WriteFile("scry/cards/cards.json", append(b, '\n'), 0o644); err != nil {
		return err
	}
	counts := map[string]int{}
	for _, c := range out {
		counts[c.Kind]++
	}
	fmt.Printf("wrote scry/cards/cards.json: %d cards %v\n", len(out), counts)
	return nil
}

func kind(r rawCard, name string) string {
	for _, l := range landscapes {
		for _, t := range r.Types {
			if t == l {
				return strings.ToLower(l)
			}
		}
	}
	if basic[name] {
		return "basic"
	}
	if r.Randomizer != nil && !*r.Randomizer {
		return "other" // non-supply or part of a pile: Prizes, Spirits, Heirlooms, Knights, ...
	}
	for _, t := range r.Types {
		switch t {
		case "Start Deck", "Trash", "State", "Boon", "Hex", "Artifact", "Loot", "Ruins", "Shelter":
			return "other"
		}
	}
	return "kingdom"
}

// setName drops edition suffixes so 1st and 2nd edition cards share a set label,
// and fixes the few tags dominiontabs has no English name for.
func setName(name, tag string) string {
	switch {
	case strings.HasPrefix(tag, "cornucopiaAndGuilds"):
		return "Cornucopia & Guilds"
	case strings.HasPrefix(tag, "guilds"):
		return "Guilds"
	case strings.HasPrefix(tag, "promo"):
		return "Promo"
	case strings.HasPrefix(tag, "cornucopia"):
		return "Cornucopia"
	}
	if name == "" {
		name = tag
	}
	for _, suffix := range []string{" 2nd Edition Upgrade", " 1st Edition", " 2nd Edition", " Extras"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

func fetchGz(url string, v any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
