package advisor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yourconscience/scrying-pool/cartographer"
)

// Header reads a "# key: ..." line from a saved score file.
func Header(score, key string) string {
	for _, l := range strings.Split(score, "\n") {
		if v, ok := strings.CutPrefix(l, "# "+key+": "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Cards reads a "# key: A, B, C" card list from a saved score file.
func Cards(score, key string) []string {
	if v := Header(score, key); v != "" {
		return strings.Split(v, ", ")
	}
	return nil
}

var reResult = regexp.MustCompile(`^(.+): (-?\d+), (.+): (-?\d+)$`)

// ResultScores reads "Nikhec: 23, gamrevinu: 32". DomBot's minus sign comes
// through as "−" or, in copied text, as "?": "R4D4: ?5" is -5.
func ResultScores(result string) (map[string]int, bool) {
	m := reResult.FindStringSubmatch(strings.NewReplacer("−", "-", "?", "-").Replace(strings.TrimSpace(result)))
	if m == nil {
		return nil, false
	}
	a, _ := strconv.Atoi(m[2])
	b, _ := strconv.Atoi(m[4])
	return map[string]int{m[1]: a, m[3]: b}, true
}

// RowScores reads the final scores from a score file's row, "2026-Oct-09 | Nikhec: 23, gamrevinu: 32".
func RowScores(row string) (map[string]int, bool) {
	_, result, ok := strings.Cut(row, " | ")
	if !ok {
		return nil, false
	}
	return ResultScores(result)
}

// Saved is one saved game with what its score file says about it.
type Saved struct {
	ID         string
	Row        string // "2026-Oct-09 | Nikhec: 23, gamrevinu: 32"
	Account    string // who loaded it: the first seat in the log
	Original   []string
	Kingdom    []string
	Landscapes []string
	Scores     map[string]int // final scores from the row, when it has them
	Score      string         // the whole score file
	Game       *cartographer.Game
}

// Load parses every saved game in dir, naming seats after the original players.
func Load(client *cartographer.Client, dir string) ([]*Saved, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	// the kingdom viewer can name cards by the client's key ("Coin Of The Realm")
	names := map[string]string{}
	for name := range client.Cards {
		names[strings.ToLower(name)] = name
	}
	canon := func(cards []string) []string {
		for i, c := range cards {
			if n, ok := names[strings.ToLower(c)]; ok {
				cards[i] = n
			}
		}
		return cards
	}
	var out []*Saved
	for _, f := range files {
		if strings.HasSuffix(f, ".bot.log") {
			continue
		}
		text, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		score, err := os.ReadFile(strings.TrimSuffix(f, ".log") + ".score.txt")
		if err != nil {
			continue
		}
		g, err := client.Parse(string(text), nil)
		if err != nil || len(g.Players) == 0 {
			continue
		}
		s := &Saved{ID: strings.TrimSuffix(filepath.Base(f), ".log"), Score: string(score), Account: g.Players[0],
			Row: Header(string(score), "row"), Original: cartographer.OriginalPlayers(string(score)),
			Kingdom: canon(Cards(string(score), "kingdom")), Landscapes: canon(Cards(string(score), "landscapes"))}
		if seats := cartographer.SeatNames(g.Players, s.Original); seats != nil {
			if g, err = client.Parse(string(text), seats); err != nil {
				continue
			}
		}
		s.Game = g
		s.Scores, _ = RowScores(s.Row)
		out = append(out, s)
	}
	return out, nil
}

// Corpus loads the saved games in dir, measures the cards over all of them and
// computes every player's features in every game. Account is the account that
// loaded most of the games: usually whoever collected them.
func Corpus(client *cartographer.Client, dir string) (account string, profiles map[string]*Profile, features []*Features, err error) {
	saved, err := Load(client, dir)
	if err != nil {
		return "", nil, nil, err
	}
	n := map[string]int{}
	for _, s := range saved {
		n[s.Account]++
	}
	for a, k := range n {
		if k > n[account] {
			account = a
		}
	}
	games := make([]*cartographer.Game, len(saved))
	for i, s := range saved {
		games[i] = s.Game
	}
	profiles = Profiles(games)
	for _, s := range saved {
		features = append(features, GameFeatures(client, s, profiles)...)
	}
	return account, profiles, features, nil
}

// Of picks one player's games.
func Of(player string, features []*Features) []*Features {
	var out []*Features
	for _, f := range features {
		if f.Me == player {
			out = append(out, f)
		}
	}
	return out
}

// PlayerCount is a player and how many saved games they played.
type PlayerCount struct {
	Name  string `json:"name"`
	Games int    `json:"games"`
}

// Players lists the players with at least min games, most games first.
func Players(features []*Features, min int) []PlayerCount {
	n := map[string]int{}
	for _, f := range features {
		n[f.Me]++
	}
	var out []PlayerCount
	for p, k := range n {
		if k >= min {
			out = append(out, PlayerCount{p, k})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Games != out[j].Games {
			return out[i].Games > out[j].Games
		}
		return out[i].Name < out[j].Name
	})
	return out
}
