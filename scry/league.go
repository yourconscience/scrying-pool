//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yourconscience/scrying-pool/scry/internal/advisor"
)

// `scry league`: a league division's DomBot "!match" replies, pasted into one
// file, as a comparison of its players. docs/league.md has what each number is.

// leagueGame is one game line of a reply: "185961251 (24m) — gamrevinu: 39, markus: 26".
// The player named first went first. DomBot underlines the winner, which a copy
// loses, so the higher score wins unless the line ends with "winner=NAME".
type leagueGame struct {
	ID     string `json:"id"`
	First  string `json:"first"`
	Second string `json:"second"`
	Score  [2]int `json:"score"`            // the first player's, then the second's
	Winner string `json:"winner,omitempty"` // "" for a tie
}

var reLeagueGame = regexp.MustCompile(`^(\d{8,10}) \([^)]*\)\s+[—–-]\s+(.+?): (-?\d+), (.+?): (-?\d+)(?:\s+winner=(.+?))?$`)

// parseLeague reads the games of every reply in text. A game pasted twice counts
// once. The warnings name the replies whose match score ("4–2") the game lines
// don't add up to: a resignation that won with fewer points shows up that way.
func parseLeague(text string) (games []leagueGame, warnings []string, err error) {
	text = strings.NewReplacer("\r\n", "\n", "−", "-").Replace(text)
	type reply struct {
		title, a, b string
		want, got   [2]int
		scored      bool
	}
	var replies []*reply
	var cur *reply
	seen := map[string]bool{}
	for i, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if a, b, ok := strings.Cut(l, " v. "); ok && !reBotGame.MatchString(l) {
			cur = &reply{title: l, a: a, b: b}
			replies = append(replies, cur)
			continue
		}
		if cur != nil && !cur.scored && reBotMatchScore.MatchString(l) {
			x, y, _ := strings.Cut(strings.NewReplacer(" ", "", "–", "-").Replace(l), "-")
			cur.want[0], _ = strconv.Atoi(x)
			cur.want[1], _ = strconv.Atoi(y)
			cur.scored = true
			continue
		}
		if !reBotGame.MatchString(l) {
			continue
		}
		m := reLeagueGame.FindStringSubmatch(l)
		if m == nil {
			return nil, nil, fmt.Errorf("line %d: expected a game like \"185961251 (24m) — alice: 39, bob: 26\", got %q", i+1, l)
		}
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		g := leagueGame{ID: m[1], First: m[2], Second: m[4]}
		g.Score[0], _ = strconv.Atoi(m[3])
		g.Score[1], _ = strconv.Atoi(m[5])
		switch w := m[6]; {
		case strings.EqualFold(w, g.First):
			g.Winner = g.First
		case strings.EqualFold(w, g.Second):
			g.Winner = g.Second
		case w != "":
			return nil, nil, fmt.Errorf("line %d: winner=%s is neither %s nor %s", i+1, w, g.First, g.Second)
		case g.Score[0] > g.Score[1]:
			g.Winner = g.First
		case g.Score[1] > g.Score[0]:
			g.Winner = g.Second
		}
		if cur != nil {
			switch {
			case g.Winner == "":
			case strings.EqualFold(g.Winner, cur.a):
				cur.got[0]++
			case strings.EqualFold(g.Winner, cur.b):
				cur.got[1]++
			}
		}
		games = append(games, g)
	}
	if len(games) == 0 {
		return nil, nil, fmt.Errorf(`no games found; expected DomBot "!match" replies with lines like "185961251 (24m) — alice: 39, bob: 26"`)
	}
	for _, r := range replies {
		if r.scored && r.got != r.want && r.got[0]+r.got[1] > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: the reply says %d–%d, the game lines give %d–%d. A game won with fewer or equal points (a resignation, or a tie broken on turns) needs winner=NAME at the end of its line.",
				r.title, r.want[0], r.want[1], r.got[0], r.got[1]))
		}
	}
	return games, warnings, nil
}

// leagueReport compares the division's players. The records come from the
// replies; the rest from the saved logs.
type leagueReport struct {
	Games         int            `json:"games"`
	Saved         int            `json:"saved"`
	Missing       []string       `json:"missing,omitempty"` // games in the replies that aren't saved
	Decided       int            `json:"decided"`           // games with a winner
	FirstWins     int            `json:"first_wins"`        // of those, won by the player who went first
	PileEndings   int            `json:"pile_endings"`      // saved games that ended on piles
	PileEnderWins int            `json:"pile_ender_wins"`   // of those, won by the player who emptied the third pile
	Players       []leaguePlayer `json:"players"`           // most wins first
	Warnings      []string       `json:"warnings,omitempty"`
}

type leaguePlayer struct {
	Name        string `json:"name"`
	Wins        int    `json:"wins"`
	Losses      int    `json:"losses"`
	Ties        int    `json:"ties"`
	FirstWins   int    `json:"first_wins"`
	FirstGames  int    `json:"first_games"`
	SecondWins  int    `json:"second_wins"`
	SecondGames int    `json:"second_games"`
	// over the saved games
	Saved       int     `json:"saved"`
	SavedLosses int     `json:"saved_losses"`
	Coins       float64 `json:"coins_mid"`         // mean $ per turn on turns 3-12
	CoinsVs     float64 `json:"coins_vs_opponent"` // that minus the opponent's, in the same games
	PilesEnded  int     `json:"piles_ended"`       // pile endings where they emptied the third pile (took the last turn)
	PilesWon    int     `json:"piles_ended_won"`   // and won
	PileLosses  int     `json:"pile_losses"`       // losses where the opponent emptied the third pile
	LostLead    int     `json:"lost_lead"`         // losses where the opponent took the last turn while they were level or ahead before it
}

// buildLeague reads the games saved in dir: each player's games through
// buildGames, as `scry games -me PLAYER --ids ...` would.
func buildLeague(dir string, games []leagueGame) (*leagueReport, error) {
	client, err := loadClient()
	if err != nil {
		return nil, err
	}
	account, profiles, features, err := advisor.Corpus(client, dir)
	if err != nil {
		return nil, err
	}
	r := &leagueReport{Games: len(games)}
	players := map[string]*leaguePlayer{}
	ids := map[string][]string{}
	byID := map[string]leagueGame{}
	var names []string
	for _, g := range games {
		byID[g.ID] = g
		if isSaved(dir, g.ID) {
			r.Saved++
		} else {
			r.Missing = append(r.Missing, g.ID)
		}
		if g.Winner != "" {
			r.Decided++
			if g.Winner == g.First {
				r.FirstWins++
			}
		}
		for seat, name := range []string{g.First, g.Second} {
			p := players[name]
			if p == nil {
				p = &leaguePlayer{Name: name}
				players[name] = p
				names = append(names, name)
			}
			ids[name] = append(ids[name], g.ID)
			won := g.Winner == name
			switch {
			case g.Winner == "":
				p.Ties++
			case won:
				p.Wins++
			default:
				p.Losses++
			}
			if seat == 0 {
				p.FirstGames++
				if won {
					p.FirstWins++
				}
			} else {
				p.SecondGames++
				if won {
					p.SecondWins++
				}
			}
		}
	}
	for _, name := range names {
		p := players[name]
		saved := false
		for _, id := range ids[name] {
			saved = saved || isSaved(dir, id)
		}
		if !saved {
			continue
		}
		gr, err := buildGames(client, dir, account, profiles, features, gamesAsk{Me: name, IDs: ids[name]})
		if err != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		var coins, vs float64
		for _, rg := range gr.Games {
			g := byID[rg.ID]
			won, lost := g.Winner == name, g.Winner != "" && g.Winner != name
			p.Saved++
			coins += rg.CoinsMid
			vs += rg.CoinsMid - rg.Them.CoinsMid
			if lost {
				p.SavedLosses++
			}
			if rg.View == nil {
				continue
			}
			if rg.Ending == "piles" {
				switch rg.View.End.By {
				case "you":
					p.PilesEnded++
					r.PileEndings++
					if won {
						p.PilesWon++
						r.PileEnderWins++
					}
				case "them":
					if lost {
						p.PileLosses++
					}
				}
			}
			if gap := rg.View.Gap; lost && len(gap) > 1 && !gap[len(gap)-1].Mine && gap[len(gap)-2].Gap >= 0 {
				p.LostLead++
			}
		}
		if p.Saved > 0 {
			p.Coins, p.CoinsVs = coins/float64(p.Saved), vs/float64(p.Saved)
		}
	}
	for _, name := range names {
		r.Players = append(r.Players, *players[name])
	}
	sort.SliceStable(r.Players, func(i, j int) bool {
		a, b := r.Players[i], r.Players[j]
		if a.Wins != b.Wins {
			return a.Wins > b.Wins
		}
		return a.Losses < b.Losses
	})
	return r, nil
}

func record(w, l, t int) string {
	if t > 0 {
		return fmt.Sprintf("%d-%d-%d", w, l, t)
	}
	return fmt.Sprintf("%d-%d", w, l)
}

func (r *leagueReport) markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# League: %d players, %d games\n\n", len(r.Players), r.Games)
	if r.Decided > 0 {
		fmt.Fprintf(&b, "The player who went first won %d of %d decided games (%s). ", r.FirstWins, r.Decided, pct(float64(r.FirstWins)/float64(r.Decided)))
	}
	if r.PileEndings > 0 {
		fmt.Fprintf(&b, "The player who emptied the third pile won %d of %d pile endings. ", r.PileEnderWins, r.PileEndings)
	}
	fmt.Fprintf(&b, "%d of the %d games are saved; the numbers from the logs cover those.\n\n", r.Saved, r.Games)
	if len(r.Missing) > 0 {
		fmt.Fprintf(&b, "Not saved: %s. `scry league -load FILE` loads them.\n\n", strings.Join(r.Missing, ", "))
	}
	b.WriteString("| Player | Record | Going first | Going second | Saved | $ t3-12 | vs opponent | Piles ended: won | Lost to their piles | Lost the lead on their last turn |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, p := range r.Players {
		cells := []string{p.Name, record(p.Wins, p.Losses, p.Ties),
			fmt.Sprintf("%d of %d", p.FirstWins, p.FirstGames), fmt.Sprintf("%d of %d", p.SecondWins, p.SecondGames), fmt.Sprint(p.Saved)}
		if p.Saved == 0 {
			cells = append(cells, "-", "-", "-", "-", "-")
		} else {
			cells = append(cells, fmt.Sprintf("$%.2f", p.Coins), signed(p.CoinsVs), fmt.Sprintf("%d of %d", p.PilesWon, p.PilesEnded),
				fmt.Sprintf("%d of %d", p.PileLosses, p.SavedLosses), fmt.Sprintf("%d of %d", p.LostLead, p.SavedLosses))
		}
		fmt.Fprintf(&b, "| %s |\n", strings.Join(cells, " | "))
	}
	b.WriteString(`
- Record, going first and going second: wins from the replies, over all games.
- The rest covers the saved games. $ t3-12 is the mean $ per turn on turns 3-12; vs opponent is that minus the opponent's in the same games.
- Piles ended: pile endings where the player took the last turn, emptying the third pile, and how many of them they won.
- Lost to their piles and Lost the lead: of the saved losses, those where the opponent ended the game on piles, and those where the opponent took the last turn while the player was level or ahead before it.
`)
	if len(r.Warnings) > 0 {
		b.WriteString("\n## Check\n\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}
	return b.String()
}

// leagueDir is the league's games folder: dir, else <user config>/scrying-pool/league-NAME,
// NAME being the replies file's name. It is apart from your own games.
func leagueDir(dir, file string) (string, error) {
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		name := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)), "league-")
		dir = filepath.Join(base, "scrying-pool", "league-"+name)
	}
	return gamesDir(dir)
}

// loadLeague saves the games dir lacks: copied from your games folder when they
// are there, else loaded through the dominion.games tab in Brave by collect.
func loadLeague(dir string, games []leagueGame, minTurns int, budget time.Duration) error {
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	mine := filepath.Join(base, "scrying-pool", "games")
	var copied, todo []string
	for _, g := range games {
		switch {
		case isSaved(dir, g.ID):
		case filepath.Clean(mine) != filepath.Clean(dir) && isSaved(mine, g.ID):
			if err := copyGame(mine, dir, g.ID); err != nil {
				return err
			}
			copied = append(copied, g.ID)
		default:
			todo = append(todo, g.ID)
		}
	}
	if len(copied) > 0 {
		fmt.Fprintf(os.Stderr, "copied %d games from %s\n", len(copied), mine)
	}
	if len(todo) == 0 {
		fmt.Fprintln(os.Stderr, "all games are saved")
		return nil
	}
	fmt.Fprintf(os.Stderr, "loading %d games through the dominion.games tab in Brave: %s\n", len(todo), strings.Join(todo, " "))
	return collect(dir, minTurns, budget, 0, todo, lastGames{})
}

// copyGame copies a saved game's files, the score last: isSaved looks for it.
func copyGame(from, to, id string) error {
	files, err := filepath.Glob(filepath.Join(from, id+".*"))
	if err != nil {
		return err
	}
	sort.SliceStable(files, func(i, j int) bool {
		return !strings.HasSuffix(files[i], ".score.txt") && strings.HasSuffix(files[j], ".score.txt")
	})
	for _, f := range files {
		src, err := os.Open(f)
		if err != nil {
			return err
		}
		dst, err := os.Create(filepath.Join(to, filepath.Base(f)))
		if err == nil {
			_, err = io.Copy(dst, src)
			if cerr := dst.Close(); err == nil {
				err = cerr
			}
		}
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// leagueText is the report on the games saved in dir, as Markdown or JSON.
func leagueText(dir string, games []leagueGame, warnings []string, asJSON bool) (string, error) {
	r, err := buildLeague(dir, games)
	if err != nil {
		return "", err
	}
	r.Warnings = append(warnings, r.Warnings...)
	for _, w := range r.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if asJSON {
		b, err := json.MarshalIndent(r, "", " ")
		return string(b) + "\n", err
	}
	return r.markdown(), nil
}
