//go:build !js

package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/yourconscience/scrying-pool/advisor"
)

// writeFeatures writes one CSV row per game.
func writeFeatures(w io.Writer, features []*advisor.Features) error {
	cw := csv.NewWriter(w)
	head := []string{"id", "date", "me", "opponent", "first", "result", "my_vp", "opp_vp", "complete", "ending", "turns",
		"opening", "opening_buys", "first_gold", "first_province"}
	roles := advisor.RoleNames()
	for _, r := range roles {
		head = append(head, "has_"+r)
	}
	head = append(head, "kingdom", "took", "landscapes", "coins")
	cw.Write(head)
	for _, f := range features {
		row := []string{f.ID, f.Date, f.Me, f.Opponent, strconv.FormatBool(f.First), strconv.FormatFloat(f.Result, 'f', -1, 64),
			strconv.Itoa(f.MyVP), strconv.Itoa(f.OppVP), strconv.FormatBool(f.Complete), f.Ending, strconv.Itoa(f.Turns),
			f.Opening, strings.Join(f.OpeningBuys, ";"), strconv.Itoa(f.FirstGold), strconv.Itoa(f.FirstProvince)}
		for _, r := range roles {
			row = append(row, strconv.FormatBool(f.Roles[r]))
		}
		var took []string
		for k := range f.Took {
			took = append(took, k)
		}
		sort.Strings(took)
		coins := make([]string, len(f.Coins))
		for i, c := range f.Coins {
			coins[i] = strconv.Itoa(c)
		}
		row = append(row, strings.Join(f.Kingdom, ";"), strings.Join(took, ";"), strings.Join(f.Landscapes, ";"), strings.Join(coins, ";"))
		cw.Write(row)
	}
	cw.Flush()
	return cw.Error()
}

// writeProfiles writes the card profiles, most played first.
func writeProfiles(w io.Writer, profiles map[string]*advisor.Profile) error {
	var ps []*advisor.Profile
	for _, p := range profiles {
		ps = append(ps, p)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Plays > ps[j].Plays })
	cw := csv.NewWriter(w)
	cw.Write([]string{"card", "plays", "actions", "buys", "cards", "coins", "trashes", "curses"})
	f := func(x float64) string { return strconv.FormatFloat(x, 'f', 2, 64) }
	for _, p := range ps {
		cw.Write([]string{p.Card, strconv.Itoa(p.Plays), f(p.Actions), f(p.Buys), f(p.Cards), f(p.Coins), f(p.Trashes), f(p.Curses)})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, "")
	return err
}
