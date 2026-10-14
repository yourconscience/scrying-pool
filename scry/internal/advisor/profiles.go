// Package advisor turns saved games into features and plain statistics about a
// player's results: what the kingdoms had, how the games went, and how often the
// player won with and without each feature.
package advisor

import (
	"sort"

	"github.com/yourconscience/scrying-pool/scry/internal/cartographer"
)

// Profile is what a card does when played, averaged over the saved logs: the
// lines one level under its "plays" line. Nothing comes from card text.
type Profile struct {
	Card    string  `json:"card"`
	Plays   int     `json:"plays"`
	Actions float64 `json:"actions"` // +Actions per play
	Buys    float64 `json:"buys"`    // +Buys per play
	Cards   float64 `json:"cards"`   // cards drawn per play
	Coins   float64 `json:"coins"`   // +$ per play (Treasures count their own line)
	Trashes float64 `json:"trashes"` // cards the player trashes per play
	Curses  float64 `json:"curses"`  // Curses other players gain per play
}

var (
	playT = map[string]bool{"PLAY": true, "PLAY_AGAIN": true, "PLAY_THIRD": true, "PLAY_WITH": true, "PLAY_AGAIN_WITH": true, "PLAY_THIRD_WITH": true,
		"PLAY_TREASURES_FOR": true} // "plays a Silver. (+$2)"
	// the amount is Argument1
	actionT = map[string]bool{"GETS_ACTION": true, "GETS_ACTIONS": true}
	buyT    = map[string]bool{"GETS_BUY": true, "GETS_BUYS": true}
	coinT   = map[string]bool{"GETS_COIN": true, "GETS_COINS": true}
	drawT   = map[string]bool{"DRAW": true}
	trashT  = map[string]bool{"TRASH": true, "TRASH_WITH": true}
	gainT   = map[string]bool{"GAIN": true, "GAIN_WITH": true, "GAIN_ON_DRAWPILE": true, "GAIN_TO_HAND": true}
)

func number(a cartographer.Arg) int {
	if a.Number != nil {
		return *a.Number
	}
	return 0
}

func count(cs []cartographer.CardN) int {
	n := 0
	for _, c := range cs {
		n += c.N
	}
	return n
}

// Profiles measures every card played in the games.
func Profiles(games []*cartographer.Game) map[string]*Profile {
	type sums struct{ plays, actions, buys, cards, coins, trashes, curses int }
	acc := map[string]*sums{}
	for _, g := range games {
		ev := g.Events
		for i, e := range ev {
			if !playT[e.Template] {
				continue
			}
			cs := e.Args[1].Cards
			if len(cs) != 1 || cs[0].Card == nil {
				continue // "plays 3 Coppers and a Silver" can't be split per card
			}
			s := acc[cs[0].Card.Name]
			if s == nil {
				s = &sums{}
				acc[cs[0].Card.Name] = s
			}
			if e.Template == "PLAY_TREASURES_FOR" { // a Treasure's own +$ is on its line
				s.coins += number(e.Args[2])
			}
			// x belongs to the play when the play is the nearest line above x with
			// a lower level: levels can skip, as in "plays a Coven. (Royal Galley)"
			// at 0 with its effects at 2
			var direct []cartographer.Event
			var open []int // levels of the lines between the play and x
			for _, x := range ev[i+1:] {
				if x.Level <= e.Level || x.Template == "NEW_TURN" {
					break
				}
				for len(open) > 0 && open[len(open)-1] >= x.Level {
					open = open[:len(open)-1]
				}
				if len(open) == 0 {
					direct = append(direct, x)
				}
				open = append(open, x.Level)
			}
			// a card played twice without a second "plays" line (Royal Galley,
			// some Throne effects) repeats its effects: count each repeat as a play
			times := 1
			if len(direct) > 0 {
				times = 0
				for _, x := range direct {
					if x.Template == direct[0].Template {
						times++
					}
				}
			}
			s.plays += cs[0].N * times
			for _, x := range direct {
				switch {
				case actionT[x.Template] && x.Player == e.Player:
					s.actions += number(x.Args[1])
				case buyT[x.Template] && x.Player == e.Player:
					s.buys += number(x.Args[1])
				case coinT[x.Template] && x.Player == e.Player:
					s.coins += number(x.Args[1])
				case drawT[x.Template] && x.Player == e.Player:
					s.cards += count(x.Args[1].Cards)
				case trashT[x.Template] && x.Player == e.Player:
					s.trashes += count(x.Args[1].Cards)
				case gainT[x.Template] && x.Player != e.Player:
					for _, c := range x.Args[1].Cards {
						if c.Card != nil && c.Card.Name == "Curse" {
							s.curses += c.N
						}
					}
				}
			}
		}
	}
	out := map[string]*Profile{}
	for name, s := range acc {
		p := float64(s.plays)
		out[name] = &Profile{Card: name, Plays: s.plays, Actions: float64(s.actions) / p, Buys: float64(s.buys) / p,
			Cards: float64(s.cards) / p, Coins: float64(s.coins) / p, Trashes: float64(s.trashes) / p, Curses: float64(s.curses) / p}
	}
	return out
}

// minPlays is how often a card must have been played for its profile to count.
const minPlays = 5

// Roles a card can fill in a kingdom, judged from its profile and its types.
// The thresholds are per play and deliberately coarse.
var roles = []struct {
	Name string
	Desc string
	Has  func(p *Profile, types []string) bool
}{
	{"village", "+2 Actions or more per play", func(p *Profile, _ []string) bool { return p.Actions >= 1.8 }},
	{"draw", "draws 2 cards or more per play", func(p *Profile, _ []string) bool { return p.Cards >= 1.8 }},
	{"plus_buy", "+Buy on most plays", func(p *Profile, _ []string) bool { return p.Buys >= 0.6 }},
	{"trasher", "trashes a card or more per play", func(p *Profile, _ []string) bool { return p.Trashes >= 0.9 }},
	{"curser", "gives Curses on at least a third of plays", func(p *Profile, _ []string) bool { return p.Curses >= 0.33 }},
	{"attack", "an Attack", func(_ *Profile, types []string) bool { return hasType(types, "Attack") }},
}

func hasType(types []string, t string) bool {
	for _, x := range types {
		if x == t {
			return true
		}
	}
	return false
}

// RoleNames lists the roles in a fixed order.
func RoleNames() []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = r.Name
	}
	return out
}

// RoleDescriptions explains each role's rule.
func RoleDescriptions() map[string]string {
	out := map[string]string{}
	for _, r := range roles {
		out[r.Name] = r.Desc
	}
	return out
}

// CardRoles says which roles a card fills. A card without enough plays fills
// only the roles its types give (attack).
func CardRoles(card string, profiles map[string]*Profile, types []string) []string {
	p := profiles[card]
	enough := p != nil && p.Plays >= minPlays
	var out []string
	for _, r := range roles {
		if r.Name != "attack" && !enough {
			continue
		}
		if r.Has(p, types) {
			out = append(out, r.Name)
		}
	}
	sort.Strings(out)
	return out
}
