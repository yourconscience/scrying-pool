package cartographer

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// CheckResult says how far a copied log can be trusted.
type CheckResult struct {
	Lines    int            // log lines read
	Unknown  int            // lines no template matches
	Turns    map[string]int // last turn of each player in the log
	MinTurns int            // turns of the player with the fewest
	Problems []string       // empty when the log is complete and matches its score screen
}

func (r CheckResult) OK() bool { return len(r.Problems) == 0 }

// Check parses a copied log and compares it with the game's score screen: every
// line must be read, each player's last turn and final deck must match. The bot
// that took a seat keeps its name in both, so no renaming is needed.
func (c *Client) Check(log, score string) CheckResult {
	var r CheckResult
	g, err := c.Parse(log, nil)
	if err != nil {
		r.Problems = append(r.Problems, err.Error())
		return r
	}
	r.Unknown = len(g.Unknown)
	for _, l := range strings.Split(log, "\n")[1:] {
		if strings.Contains(l, "\t") {
			r.Lines++
		}
	}
	if len(g.Header) == 0 || !strings.HasPrefix(g.Header[0], "Game #") {
		r.Problems = append(r.Problems, "the log doesn't start with the game line")
	}
	if r.Unknown > 0 {
		r.Problems = append(r.Problems, fmt.Sprintf("%d unknown lines, first: line %d %q", r.Unknown, g.Unknown[0].Line, g.Unknown[0].Text))
	}
	r.Turns = map[string]int{}
	for _, e := range g.Events {
		if e.Template == "NEW_TURN" && e.Extra != "Fleet" { // the score screen doesn't count Fleet's extra turns
			r.Turns[e.Player] = e.Turn
		}
	}
	r.MinTurns = -1
	for _, p := range g.Players {
		if r.MinTurns < 0 || r.Turns[p] < r.MinTurns {
			r.MinTurns = r.Turns[p]
		}
	}
	if r.MinTurns < 0 {
		r.MinTurns = 0
	}
	if strings.TrimSpace(score) == "" {
		r.Problems = append(r.Problems, "no score screen")
		return r
	}
	wantTurns, wantDecks := ParseScoreScreen(score, func(name string) bool { return c.Cards[name] != nil })
	if !reflect.DeepEqual(r.Turns, wantTurns) && !(len(r.Turns) == 0 && len(wantTurns) == 0) {
		r.Problems = append(r.Problems, fmt.Sprintf("last turns %v, score screen says %v", r.Turns, wantTurns))
	}
	got := g.FinalDecks()
	var names []string
	for p := range wantDecks {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		have, ok := got[p]
		if !ok {
			have = got[p[:1]] // a player with no turns is known only by initial
		}
		if want := wantDecks[p]; !reflect.DeepEqual(have, want) {
			r.Problems = append(r.Problems, fmt.Sprintf("%s's deck differs: %s", p, deckDiff(have, want)))
		}
	}
	if len(wantDecks) == 0 {
		r.Problems = append(r.Problems, "score screen has no decks")
	}
	return r
}

// deckDiff lists the cards whose counts differ, e.g. "Kitsune 2 (score screen 4)".
func deckDiff(have, want Deck) string {
	seen := map[string]bool{}
	var out []string
	for _, d := range []Deck{have, want} {
		for k := range d {
			if !seen[k] && have[k] != want[k] {
				out = append(out, fmt.Sprintf("%s %d (score screen %d)", k, have[k], want[k]))
			}
			seen[k] = true
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// CheckScore compares computed final VP with known final scores (e.g. DomBot's).
// final is keyed by the players' real names; a bot that took a seat is matched to
// the remaining name. The game's scoring landmarks and allies come from the score
// screen's VP section.
func (c *Client) CheckScore(log, score string, final map[string]int) (got map[string]int, problems []string) {
	g, err := c.Parse(log, nil)
	if err != nil {
		return nil, []string{err.Error()}
	}
	vp := c.Score(g, ScoreInput{Landscapes: c.Landscapes(score)})
	seats := SeatNames(g.Players, OriginalPlayers(score))
	if seats == nil {
		return nil, []string{"can't tell which seat was which player (no \"# original players\" line)"}
	}
	got = map[string]int{}
	for p, b := range vp {
		name := seats[p]
		got[name] = b.Total()
		if want, ok := final[name]; !ok {
			problems = append(problems, fmt.Sprintf("no known score for %s", name))
		} else if got[name] != want {
			problems = append(problems, fmt.Sprintf("%s: computed %d VP %v, final score %d", name, got[name], b, want))
		}
	}
	sort.Strings(problems)
	return got, problems
}

// Landscapes lists the landmarks and allies a saved game scores, e.g. "Wall":
// the ones its score screen lists and the ones its "# landscapes:" line names.
// The score screen shows an ally such as Plateau Shepherds without a count, so
// it can't be read there.
func (c *Client) Landscapes(score string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		for _, card := range c.Cards {
			if (strings.EqualFold(card.Name, name) || strings.EqualFold(card.Key, name)) && (card.Is("Landmark") || card.Is("Ally")) && !seen[card.Name] {
				seen[card.Name] = true
				out = append(out, card.Name)
				return
			}
		}
	}
	for _, name := range ScoreScreenSources(score) {
		add(name)
	}
	for _, l := range strings.Split(score, "\n") {
		if v, ok := strings.CutPrefix(l, "# landscapes: "); ok {
			for _, name := range strings.Split(v, ", ") {
				add(name)
			}
		}
	}
	return out
}

// SeatNames maps the names in a reloaded game's log to the real players. The
// player who reloads the game sits in seat 1, which belonged to the first
// original player; the bot sits in seat 2, the second original player. In your
// own games the reloading player is the one whose name is among the original
// players; in other people's games neither log name is, and seat 1 plays first.
func SeatNames(logPlayers, original []string) map[string]string {
	if len(original) != 2 || len(logPlayers) != 2 {
		return nil
	}
	in := func(n string) bool { return n == original[0] || n == original[1] }
	viewer, bot := logPlayers[0], logPlayers[1]
	if !in(viewer) && in(bot) {
		viewer, bot = bot, viewer
	}
	if in(bot) {
		return nil
	}
	return map[string]string{viewer: original[0], bot: original[1]}
}

// OriginalPlayers reads "# original players: a / b" from a saved score screen.
func OriginalPlayers(score string) []string {
	for _, l := range strings.Split(score, "\n") {
		if v, ok := strings.CutPrefix(l, "# original players: "); ok {
			return strings.Split(strings.TrimSpace(v), " / ")
		}
	}
	return nil
}
