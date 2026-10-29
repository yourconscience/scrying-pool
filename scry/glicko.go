//go:build !js

package main

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
)

// The community's Dominion Card Glicko ratings (tools/glicko), for the
// Profiles page's openings: how strong a card is against others of its cost.
//
//go:embed cards/glicko.json
var glickoJSON []byte

var glicko = func() map[string]advisor.Rating {
	var doc struct {
		Cards map[string]advisor.Rating `json:"cards"`
	}
	if err := json.Unmarshal(glickoJSON, &doc); err != nil {
		panic(err)
	}
	m := map[string]advisor.Rating{}
	for name, r := range doc.Cards {
		m[imageKey(name)] = r
	}
	return m
}()

// cardRating finds a card's rating under the names the logs use: "Kings Court"
// is the sheet's "King's Court", and a split pile ("Augur Pile") is rated as
// the pile ("Augurs").
func cardRating(name string) (advisor.Rating, bool) {
	k := imageKey(name)
	if r, ok := glicko[k]; ok {
		return r, true
	}
	if base, ok := strings.CutSuffix(k, "pile"); ok {
		for _, plural := range []string{base + "s", base + "es", base} {
			if r, ok := glicko[plural]; ok {
				return r, true
			}
		}
	}
	return advisor.Rating{}, false
}

// cardRatings rates the kingdom cards and the cards bought on turns 1 and 2 in
// the profile's games.
func cardRatings(in *advisor.Insights) map[string]advisor.Rating {
	out := map[string]advisor.Rating{}
	for _, g := range in.Games {
		for _, name := range append(append([]string{}, g.Kingdom...), g.OpeningBuys...) {
			if r, ok := cardRating(name); ok {
				out[name] = r
			}
		}
	}
	return out
}
