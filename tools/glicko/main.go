// glicko regenerates scry/cards/glicko.json from the community's Dominion Card
// Glicko ratings, a published Google sheet of how strong players rate each card
// against others of its cost. Only each card's rating and cost group are kept.
//
// Run from the repo root: go run ./tools/glicko
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const sheet = "https://docs.google.com/spreadsheets/d/e/2PACX-1vTkdmqYioc1WZb-RlsCH7qNIe75VAWasuhu_8OWiwnhmCj3TKJXczm6j8vxgYiRYdZIE9zlSBnwxWC9/pub"

// the sheet's tabs: kingdom cards, and Events and Projects
var tabs = []string{"0", "1523942532"}

// cost groups as the sheet's "Qvist category" names them
var costs = map[string]string{"0-2-cost": "2", "3-cost": "3", "4-cost": "4", "5-cost": "5", "6-cost or more": "6", "Potion cost": "P"}

type rating struct {
	Rating float64 `json:"rating"`
	Cost   string  `json:"cost"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "glicko:", err)
		os.Exit(1)
	}
}

func run() error {
	doc := struct {
		Source  string            `json:"source"`
		Updated string            `json:"updated"`
		Cards   map[string]rating `json:"cards"`
	}{Source: strings.TrimSuffix(sheet, "/pub") + "/pubhtml", Cards: map[string]rating{}}
	for _, gid := range tabs {
		resp, err := http.Get(sheet + "?gid=" + gid + "&single=true&output=csv")
		if err != nil {
			return err
		}
		rows, err := csv.NewReader(resp.Body).ReadAll()
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("tab %s: %w", gid, err)
		}
		if len(rows) < 2 || len(rows[0]) < 13 || rows[0][2] != "rating" || rows[0][10] != "Qvist category" {
			return fmt.Errorf("tab %s: unexpected header %q", gid, rows[0])
		}
		doc.Updated = rows[0][12] // "Last Update:" then the time
		for _, r := range rows[1:] {
			cost, ok := costs[r[10]]
			v, err := strconv.ParseFloat(r[2], 64)
			if !ok || err != nil || r[1] == "" {
				continue
			}
			doc.Cards[r[1]] = rating{v, cost}
		}
	}
	b, _ := json.MarshalIndent(doc, "", " ")
	if err := os.WriteFile("scry/cards/glicko.json", append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote scry/cards/glicko.json: %d cards, updated %s\n", len(doc.Cards), doc.Updated)
	return nil
}
