//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

var reDigits = regexp.MustCompile(`^\d+$`)

// gamesAsk is what a report covers: the games in IDs, else the Last latest ones
// (against Opponent, if it names one).
type gamesAsk struct {
	Me       string
	Opponent string
	Last     int
	IDs      []string
}

// gamesReport is `scry games` and the Last games page: what happened in some of
// your games, against what your rating levels predicted and your usual play.
type gamesReport struct {
	Me           string       `json:"me"`
	Opponent     string       `json:"opponent,omitempty"`
	Named        bool         `json:"named"`             // the games were asked for by number
	Asked        int          `json:"asked"`             // how many games were asked for
	Missing      []string     `json:"missing,omitempty"` // named games that aren't saved
	Newest       string       `json:"newest"`            // the date of your newest saved game
	Wins         int          `json:"wins"`
	Losses       int          `json:"losses"`
	Ties         int          `json:"ties"`
	Score        float64      `json:"score"`
	Expected     float64      `json:"expected"` // over the games with both levels
	Leveled      int          `json:"leveled"`
	LeveledScore float64      `json:"leveled_score"` // your score in those games
	These        gameStats    `json:"these"`
	WinStats     gameStats    `json:"win_stats"`
	LossStats    gameStats    `json:"loss_stats"`
	Usual        gameStats    `json:"usual"` // your other games of the last 90 days
	Compare      *gamesVs     `json:"compare,omitempty"`
	Takeaways    []string     `json:"takeaways"`
	FirstSeat    seatSplit    `json:"first_seat"`  // your results going first
	SecondSeat   seatSplit    `json:"second_seat"` // and going second
	Findings     []finding    `json:"findings"`    // points that repeat in 2 or more games
	Games        []reportGame `json:"games"`
}

// gameStats averages some of a player's games.
type gameStats struct {
	Games         int     `json:"games"`
	Score         float64 `json:"score"`          // share of the points
	Turns         float64 `json:"turns"`          //
	Coins         float64 `json:"coins"`          // $ per turn over turns 1-10
	FirstProvince float64 `json:"first_province"` // over the games with one
	NoProvince    int     `json:"no_province"`
}

// gamesVs compares you with the opponent, from the saved games only.
type gamesVs struct {
	MyLevel    float64   `json:"my_level"`
	OppLevel   float64   `json:"opp_level"`
	Wins       int       `json:"wins"` // head to head
	Losses     int       `json:"losses"`
	Ties       int       `json:"ties"`
	MyRecent   gameStats `json:"my_recent"` // the last 30 days
	OppRecent  gameStats `json:"opp_recent"`
	RecentDays int       `json:"recent_days"`
}

// reportGame is one game of the report. Features are mine.
type reportGame struct {
	*advisor.Features
	Chance   float64       `json:"chance,omitempty"`
	CoinsMid float64       `json:"coins_mid"` // $ per turn over turns 3-12
	Luck     *drawLuck     `json:"luck,omitempty"`
	Verdict  string        `json:"verdict,omitempty"` // of a loss: draw, deck or unclear
	Them     theirGame     `json:"them"`
	Turning  *turningPoint `json:"turning,omitempty"`
	Records  []cardRecord  `json:"records,omitempty"`
	View     *matchView    `json:"view,omitempty"` // matchview.go
}

// theirGame is the opponent's side of a game.
type theirGame struct {
	Opening       string    `json:"opening"`
	OpeningBuys   []string  `json:"opening_buys"`
	FirstGold     int       `json:"first_gold"`
	FirstProvince int       `json:"first_province"`
	CoinsMid      float64   `json:"coins_mid"`
	Luck          *drawLuck `json:"luck,omitempty"`
}

// drawLuck compares the hands a player was dealt on turns 3-12 with what their
// deck could give. It's nil when the log hides most of those hands: a log names
// the hands of the first seat only, unless it merges both seats.
type drawLuck struct {
	Luck     float64 `json:"luck"`      // mean of the treasure $ in hand minus 5 × the deck's $ per card
	BadHands int     `json:"bad_hands"` // hands at least $2 below that
	Hands    int     `json:"hands"`
}

// turningPoint is the three rounds where the VP gap moved most toward the winner.
type turningPoint struct {
	From      int      `json:"from"`
	To        int      `json:"to"`
	GapBefore int      `json:"gap_before"` // my VP minus theirs
	GapAfter  int      `json:"gap_after"`
	MyGains   []string `json:"my_gains"`
	TheirGain []string `json:"their_gains"`
}

// cardRecord is how you score in your saved games with a card you took.
type cardRecord struct {
	Card  string  `json:"card"`
	Games int     `json:"games"`
	Score float64 `json:"score"`
}

const (
	luckLow = 0.5 // about the worst 10% of the games with visible hands (docs/match-report.md)
	luckGap = 0.5 // about 1.25 standard deviations of one game's luck
)

// verdict says whether the hands look like the reason for a loss. It's a hint:
// luck counts treasure only, and it barely predicts the result over the saved
// games. A win or a tie gets none, and so do hidden hands.
func verdict(result float64, mine, theirs *drawLuck) string {
	if result >= 0.5 || mine == nil {
		return ""
	}
	if theirs == nil { // their hands are hidden: only mine to go on
		switch {
		case mine.Luck <= -luckLow:
			return "draw"
		case mine.Luck >= 0:
			return "deck"
		}
		return "unclear"
	}
	switch {
	case mine.Luck <= -luckLow && mine.Luck <= theirs.Luck-luckGap:
		return "draw"
	case mine.Luck < 0 && theirs.Luck < 0:
		return "unclear"
	}
	return "deck"
}

func statsOf(games []*advisor.Features) gameStats {
	s := gameStats{Games: len(games)}
	if len(games) == 0 {
		return s
	}
	coins, nCoins, firsts, nFirsts := 0, 0, 0, 0
	for _, g := range games {
		s.Score += g.Result / float64(len(games))
		s.Turns += float64(g.Turns) / float64(len(games))
		for _, c := range g.Coins[:min(10, len(g.Coins))] {
			coins += c
			nCoins++
		}
		if g.FirstProvince > 0 {
			firsts += g.FirstProvince
			nFirsts++
		} else {
			s.NoProvince++
		}
	}
	if nCoins > 0 {
		s.Coins = float64(coins) / float64(nCoins)
	}
	if nFirsts > 0 {
		s.FirstProvince = float64(firsts) / float64(nFirsts)
	}
	return s
}

// coinsMid is the mean $ per turn over turns 3-12.
func coinsMid(coins []int) float64 {
	if len(coins) < 3 {
		return 0
	}
	mid := coins[2:min(12, len(coins))]
	sum := 0
	for _, c := range mid {
		sum += c
	}
	return float64(sum) / float64(len(mid))
}

// treasureValue is what a card in hand pays as a Treasure; other cards count 0.
func treasureValue(client *cartographer.Client, profiles map[string]*advisor.Profile, card string) float64 {
	switch card {
	case "Copper":
		return 1
	case "Silver":
		return 2
	case "Gold":
		return 3
	case "Platinum":
		return 5
	}
	c := client.Cards[card]
	if c == nil || !slices.Contains(c.Types, "Treasure") || profiles[card] == nil {
		return 0
	}
	return profiles[card].Coins
}

func luckOf(client *cartographer.Client, profiles map[string]*advisor.Profile, turns []cartographer.Turn, player string) *drawLuck {
	out := &drawLuck{}
	sum := 0.0
	for i, t := range turns {
		if i == 0 || t.Player != player || t.Extra != "" || t.Number < 3 || t.Number > 12 {
			continue
		}
		before := turns[i-1] // the hand and the deck at the start of this turn
		hand := before.Zones[player][cartographer.ZoneHand]
		if hand == nil || hand.Unknown > 0 {
			continue
		}
		value, cards := 0.0, 0
		for card, n := range before.Decks[player] {
			value += treasureValue(client, profiles, card) * float64(n)
			cards += n
		}
		actual := 0.0
		for card, n := range hand.Known {
			actual += treasureValue(client, profiles, card) * float64(n)
		}
		if cards == 0 {
			continue
		}
		d := actual - 5*value/float64(cards)
		sum += d
		out.Hands++
		if d <= -2 {
			out.BadHands++
		}
	}
	if out.Hands < 3 {
		return nil
	}
	out.Luck = sum / float64(out.Hands)
	return out
}

// gainNames lists the cards in some turns' gains, the most gained first.
func gainNames(gains map[string]int) []string {
	names := make([]string, 0, len(gains))
	for name := range gains {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if gains[names[i]] != gains[names[j]] {
			return gains[names[i]] > gains[names[j]]
		}
		return names[i] < names[j]
	})
	for i, name := range names {
		if gains[name] > 1 {
			names[i] = fmt.Sprintf("%d× %s", gains[name], name)
		}
	}
	return names
}

// turningOf finds the three rounds where the gap moved most toward the winner.
func turningOf(client *cartographer.Client, s *advisor.Saved, turns []cartographer.Turn, me, opp string, result float64) *turningPoint {
	if result == 0.5 {
		return nil
	}
	byTurn, _ := scoresByTurn(client, s.Game, cartographer.ScoreInput{Landscapes: client.Landscapes(s.Score)})
	if len(byTurn) != len(turns) {
		return nil
	}
	sign := 1
	if result < 0.5 {
		sign = -1
	}
	gap := func(i int) int {
		if i < 0 {
			return 0
		}
		return byTurn[i][me].Total() - byTurn[i][opp].Total()
	}
	const window = 6 // three rounds
	best, swing := -1, 0
	for i := 0; i+window <= len(turns); i++ {
		if d := sign * (gap(i+window-1) - gap(i-1)); d > swing {
			best, swing = i, d
		}
	}
	if best < 0 {
		return nil
	}
	tp := &turningPoint{From: turns[best].Number, To: turns[best+window-1].Number, GapBefore: gap(best - 1), GapAfter: gap(best + window - 1)}
	gains := map[string]map[string]int{me: {}, opp: {}}
	for _, t := range turns[best : best+window] {
		for _, c := range append(slices.Clone(t.Bought), t.Gained...) {
			if c.Card != nil && gains[t.Player] != nil {
				gains[t.Player][c.Card.Name] += c.N
			}
		}
	}
	tp.MyGains, tp.TheirGain = gainNames(gains[me]), gainNames(gains[opp])
	return tp
}

// recordsOf is your record over the saved games with the kingdom cards you took
// in this game, for the cards you've taken in at least 5 games.
func recordsOf(g *advisor.Features, mine []*advisor.Features) []cardRecord {
	var out []cardRecord
	for _, card := range g.Kingdom {
		if !g.Took[card] {
			continue
		}
		r := cardRecord{Card: card}
		for _, f := range mine {
			if f.Took[card] {
				r.Games++
				r.Score += f.Result
			}
		}
		if r.Games >= 5 {
			r.Score /= float64(r.Games)
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Games > out[j].Games })
	return out
}

// buildGames makes the report from the saved games' features.
func buildGames(client *cartographer.Client, dir, account string, profiles map[string]*advisor.Profile, features []*advisor.Features, ask gamesAsk) (*gamesReport, error) {
	me := ask.Me
	if me == "" {
		me = account
	}
	latest := advisor.LatestGames(me, features, len(features)).Games // newest first
	if len(latest) == 0 {
		return nil, fmt.Errorf("no saved games of %q", me)
	}
	r := &gamesReport{Me: me, Opponent: ask.Opponent, Named: len(ask.IDs) > 0, Asked: ask.Last, Newest: latest[0].Date, Takeaways: []string{}, Games: []reportGame{}}
	var picked []advisor.RecentGame
	if r.Named {
		r.Asked = len(ask.IDs)
		for _, g := range latest {
			if slices.Contains(ask.IDs, g.ID) {
				picked = append(picked, g)
			}
		}
		for _, id := range ask.IDs {
			if !slices.ContainsFunc(picked, func(g advisor.RecentGame) bool { return g.ID == id }) && !slices.Contains(r.Missing, id) {
				r.Missing = append(r.Missing, id)
			}
		}
	} else {
		for _, g := range latest {
			if len(picked) < ask.Last && (ask.Opponent == "" || strings.EqualFold(g.Opponent, ask.Opponent)) {
				picked = append(picked, g)
			}
		}
	}
	if len(picked) == 0 {
		who := ""
		if ask.Opponent != "" {
			who = " against " + ask.Opponent
		}
		return nil, fmt.Errorf("none of those games of %s%s is saved", me, who)
	}

	ids := make([]string, len(picked))
	for i, g := range picked {
		ids[i] = g.ID
	}
	saved, err := advisor.LoadGames(client, dir, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]*advisor.Saved{}
	for _, s := range saved {
		byID[s.ID] = s
	}
	theirs := map[string]*advisor.Features{}
	var mine, wins, losses, these []*advisor.Features
	for _, f := range features {
		if f.Me == me {
			mine = append(mine, f)
		} else if slices.Contains(ids, f.ID) {
			theirs[f.ID] = f
		}
	}
	usual := slices.DeleteFunc(lastDays(mine, siteDays), func(f *advisor.Features) bool { return slices.Contains(ids, f.ID) })
	for _, g := range picked {
		f := g.Features
		these = append(these, f)
		switch f.Result {
		case 1:
			r.Wins++
			wins = append(wins, f)
		case 0:
			r.Losses++
			losses = append(losses, f)
		default:
			r.Ties++
		}
		r.Score += f.Result
		if g.Chance > 0 {
			r.Expected += g.Chance
			r.Leveled++
			r.LeveledScore += f.Result
		}
		rg := reportGame{Features: f, Chance: g.Chance, CoinsMid: coinsMid(f.Coins), Records: recordsOf(f, mine)}
		if t := theirs[f.ID]; t != nil {
			rg.Them = theirGame{Opening: t.Opening, OpeningBuys: t.OpeningBuys, FirstGold: t.FirstGold, FirstProvince: t.FirstProvince, CoinsMid: coinsMid(t.Coins)}
		}
		if s := byID[f.ID]; s != nil {
			turns := s.Game.Turns()
			rg.Luck = luckOf(client, profiles, turns, me)
			rg.Them.Luck = luckOf(client, profiles, turns, f.Opponent)
			rg.Verdict = verdict(f.Result, rg.Luck, rg.Them.Luck)
			rg.Turning = turningOf(client, s, turns, me, f.Opponent, f.Result)
			rg.View = matchViewOf(client, s, turns, f, theirs[f.ID], usual)
		}
		r.Games = append(r.Games, rg)
	}
	r.These, r.WinStats, r.LossStats = statsOf(these), statsOf(wins), statsOf(losses)
	r.Usual = statsOf(usual)
	if ask.Opponent != "" {
		r.Compare = compareWith(picked[0].Features, mine, features)
		r.Opponent = picked[0].Opponent // as dominion.games spells it
	}
	r.Takeaways = r.takeaways()
	r.FirstSeat, r.SecondSeat = seatsOf(r.Games)
	r.Findings = findingsOf(r.Games)
	return r, nil
}

func compareWith(newest *advisor.Features, mine, features []*advisor.Features) *gamesVs {
	const days = 30
	v := &gamesVs{MyLevel: newest.MyLevel, OppLevel: newest.OppLevel, RecentDays: days}
	for _, f := range mine {
		if f.Opponent == newest.Opponent {
			switch f.Result {
			case 1:
				v.Wins++
			case 0:
				v.Losses++
			default:
				v.Ties++
			}
		}
	}
	v.MyRecent = statsOf(lastDays(mine, days))
	v.OppRecent = statsOf(lastDays(advisor.Of(newest.Opponent, features), days))
	return v
}

// takeaways come from fixed rules, so the report never says what the numbers don't.
func (r *gamesReport) takeaways() []string {
	out := []string{}
	verdicts := map[string][]string{}
	hidden := 0
	for i, g := range r.Games {
		switch {
		case g.Result >= 0.5:
		case g.Verdict == "":
			hidden++
		default:
			verdicts[g.Verdict] = append(verdicts[g.Verdict], fmt.Sprintf("%d", i+1))
		}
	}
	if v := verdicts["draw"]; len(v) > 0 {
		out = append(out, gameLabel(v)+": the hands were far below what the deck's treasure could give.")
	}
	if v := verdicts["deck"]; len(v) > 0 {
		out = append(out, gameLabel(v)+": the hands were normal for the deck's treasure, so look at the build and the play first.")
	}
	if v := verdicts["unclear"]; len(v) > 0 {
		out = append(out, gameLabel(v)+": the hands don't settle it.")
	}
	if hidden > 0 {
		out = append(out, fmt.Sprintf("Lost games with your hands hidden: %d. The opponent went first, and the log names only the first seat's hands.", hidden))
	}
	if r.These.Games >= 5 && r.Usual.Games >= 20 {
		if d := r.These.FirstProvince - r.Usual.FirstProvince; r.These.FirstProvince > 0 && d >= 1 {
			out = append(out, fmt.Sprintf("Your first Province came %.1f turns later than usual (turn %.1f against %.1f).", d, r.These.FirstProvince, r.Usual.FirstProvince))
		}
		if d := r.Usual.Coins - r.These.Coins; d >= 0.3 {
			out = append(out, fmt.Sprintf("You had $%.1f per turn on turns 1-10, $%.1f below your usual $%.1f.", r.These.Coins, d, r.Usual.Coins))
		}
		if d := r.These.Score - r.Usual.Score; d <= -0.15 {
			out = append(out, fmt.Sprintf("You scored %s here against your usual %s; over %d games a gap this size can still be chance.", pct(r.These.Score), pct(r.Usual.Score), r.These.Games))
		}
	}
	if line := r.bySplit("Going first and second", func(g reportGame) string {
		if g.First {
			return "first"
		}
		return "second"
	}); line != "" {
		out = append(out, line)
	}
	if line := r.bySplit("Openings", func(g reportGame) string { return g.Opening }); line != "" {
		out = append(out, line)
	}
	return out
}

func gameLabel(numbers []string) string {
	if len(numbers) == 1 {
		return "Game " + numbers[0]
	}
	return "Games " + strings.Join(numbers, ", ")
}

// bySplit is your score per group of games, when there are at least 5 games and two groups.
func (r *gamesReport) bySplit(label string, group func(reportGame) string) string {
	if len(r.Games) < 5 {
		return ""
	}
	score, games := map[string]float64{}, map[string]int{}
	for _, g := range r.Games {
		if k := group(g); k != "" {
			score[k] += g.Result
			games[k]++
		}
	}
	if len(games) < 2 {
		return ""
	}
	keys := make([]string, 0, len(games))
	for k := range games {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if score[keys[i]]/float64(games[keys[i]]) != score[keys[j]]/float64(games[keys[j]]) {
			return score[keys[i]]/float64(games[keys[i]]) > score[keys[j]]/float64(games[keys[j]])
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s %s of %d", k, trimScore(score[k]), games[k])
	}
	return label + ": " + strings.Join(parts, ", ") + "."
}

func trimScore(x float64) string { return strings.TrimSuffix(fmt.Sprintf("%.1f", x), ".0") }

func signed(x float64) string { return fmt.Sprintf("%+.2f", x) }

func turnOr(t int) string {
	if t == 0 {
		return "none"
	}
	return fmt.Sprintf("t%d", t)
}

func (r *gamesReport) title() string {
	head := r.Me
	if r.Opponent != "" {
		head += " vs " + r.Opponent
	}
	if r.Named {
		return fmt.Sprintf("%s: %d named games", head, len(r.Games))
	}
	return fmt.Sprintf("%s: last %d games", head, len(r.Games))
}

func statCells(s gameStats) []string {
	if s.Games == 0 {
		return []string{"0", "-", "-", "-", "-", "-"}
	}
	first := "-"
	if s.FirstProvince > 0 {
		first = fmt.Sprintf("t%.1f", s.FirstProvince)
	}
	return []string{fmt.Sprint(s.Games), pct(s.Score), fmt.Sprintf("%.1f", s.Turns), fmt.Sprintf("$%.1f", s.Coins), first, fmt.Sprint(s.NoProvince)}
}

func luckText(l *drawLuck) string {
	if l == nil {
		return "hidden"
	}
	return fmt.Sprintf("%s $/turn, %d bad of %d hands", signed(l.Luck), l.BadHands, l.Hands)
}

func (r *gamesReport) markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.title())
	fmt.Fprintf(&b, "%d-%d-%d. Score %s", r.Wins, r.Losses, r.Ties, trimScore(r.Score))
	if r.Leveled > 0 {
		fmt.Fprintf(&b, "; %s of %d games with both levels, expected %.1f", trimScore(r.LeveledScore), r.Leveled, r.Expected)
	}
	fmt.Fprintf(&b, ". Your newest saved game is from %s.\n\n", r.Newest)
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "Not saved: %s.\n\n", strings.Join(r.Missing, ", "))
	}
	if c := r.Compare; c != nil {
		fmt.Fprintf(&b, "| | %s | %s |\n|---|---|---|\n", r.Me, r.Opponent)
		fmt.Fprintf(&b, "| Level at the newest game | %.1f | %.1f |\n", c.MyLevel, c.OppLevel)
		fmt.Fprintf(&b, "| Head to head, saved | %d-%d-%d | |\n", c.Wins, c.Losses, c.Ties)
		fmt.Fprintf(&b, "| Last %d days, saved | %s of %d | %s of %d |\n\n", c.RecentDays, pct(c.MyRecent.Score), c.MyRecent.Games, pct(c.OppRecent.Score), c.OppRecent.Games)
	}
	fmt.Fprint(&b, "| | Games | Score | Turns | $ t1-10 | First Province | No Province |\n|---|---|---|---|---|---|---|\n")
	for _, row := range []struct {
		name string
		s    gameStats
	}{{"These", r.These}, {"Wins", r.WinStats}, {"Losses", r.LossStats}, {"Usual, 90 days", r.Usual}} {
		fmt.Fprintf(&b, "| %s | %s |\n", row.name, strings.Join(statCells(row.s), " | "))
	}
	fmt.Fprintf(&b, "\nBy seat: first %s of %d, second %s of %d.\n", trimScore(r.FirstSeat.Score), r.FirstSeat.Games, trimScore(r.SecondSeat.Score), r.SecondSeat.Games)
	if len(r.Findings) > 0 {
		b.WriteString("\n## Across the match\n\n")
		for _, f := range r.Findings {
			fmt.Fprintf(&b, "- %s\n", f.Text)
		}
	}
	if len(r.Takeaways) > 0 {
		b.WriteString("\n## Takeaways\n\n")
		for _, t := range r.Takeaways {
			fmt.Fprintf(&b, "- %s\n", t)
		}
	}
	result := map[float64]string{1: "won", 0.5: "tied", 0: "lost"}
	for i, g := range r.Games {
		seat := "second"
		if g.First {
			seat = "first"
		}
		vp := []int{g.MyVP, g.OppVP}
		if len(g.Final) == 2 {
			vp = g.Final
		}
		fmt.Fprintf(&b, "\n## Game %d, %s: %s %d-%d against %s (%s, %d turns, you went %s)\n\n", i+1, g.ID, result[g.Result], vp[0], vp[1], g.Opponent, g.Ending, g.Turns, seat)
		if g.Chance > 0 {
			fmt.Fprintf(&b, "Levels %.1f and %.1f: win chance %s.\n", g.MyLevel, g.OppLevel, pct(g.Chance))
		}
		fmt.Fprintf(&b, "Kingdom: %s", strings.Join(g.Kingdom, ", "))
		if len(g.Landscapes) > 0 {
			fmt.Fprintf(&b, " | Landscapes: %s", strings.Join(g.Landscapes, ", "))
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "Opening: %s %s (%s: %s %s)\n", g.Opening, strings.Join(g.OpeningBuys, " / "), g.Opponent, g.Them.Opening, strings.Join(g.Them.OpeningBuys, " / "))
		fmt.Fprintf(&b, "First Gold %s (%s), first Province %s (%s); $%.1f per turn on t3-12 (%.1f)\n", turnOr(g.FirstGold), turnOr(g.Them.FirstGold), turnOr(g.FirstProvince), turnOr(g.Them.FirstProvince), g.CoinsMid, g.Them.CoinsMid)
		fmt.Fprintf(&b, "Draw luck: %s (%s: %s)", luckText(g.Luck), g.Opponent, luckText(g.Them.Luck))
		if g.Verdict != "" {
			fmt.Fprintf(&b, ". Verdict: %s", g.Verdict)
		}
		b.WriteString("\n")
		if tp := g.Turning; tp != nil {
			fmt.Fprintf(&b, "Turning point t%d-t%d: gap %+d to %+d; you gained %s; %s gained %s\n", tp.From, tp.To, tp.GapBefore, tp.GapAfter, listOr(tp.MyGains), g.Opponent, listOr(tp.TheirGain))
		}
		if len(g.Records) > 0 {
			parts := make([]string, len(g.Records))
			for j, c := range g.Records {
				parts[j] = fmt.Sprintf("%s %s of %d", c.Card, pct(c.Score), c.Games)
			}
			fmt.Fprintf(&b, "Your record with these cards: %s\n", strings.Join(parts, "; "))
		}
		g.viewMarkdown(&b)
	}
	return b.String()
}

func listOr(names []string) string {
	if len(names) == 0 {
		return "nothing"
	}
	return strings.Join(names, ", ")
}

// gamesText is the report from the saved games in dir, as Markdown or JSON.
func gamesText(dir string, ask gamesAsk, asJSON bool) (string, error) {
	client, err := loadClient()
	if err != nil {
		return "", err
	}
	account, profiles, features, err := advisor.Corpus(client, dir)
	if err != nil {
		return "", err
	}
	r, err := buildGames(client, dir, account, profiles, features, ask)
	if err != nil {
		return "", err
	}
	if asJSON {
		b, err := json.MarshalIndent(r, "", " ")
		return string(b) + "\n", err
	}
	return r.markdown(), nil
}
