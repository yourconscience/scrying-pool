package cartographer

import "sort"

// Scoring follows the cards' own texts. Fixed values for Victory cards come from
// the printed VP (checked against pydominion); the rest are the "worth" and
// "when scoring" rules of cards, landmarks, the Plateau Shepherds ally and the
// Misery states. Final scores were checked against DomBot's results for the
// saved games (see the tests). The cards the second editions removed (Great
// Hall, Silk Road, ...) are scored for the engine's games.

var fixedVP = map[string]int{
	"Estate": 1, "Duchy": 3, "Province": 6, "Colony": 10, "Curse": -1,
	"Mill": 1, "Crumbling Castle": 1, "Great Hall": 1, "Harem": 2,
	"Nobles": 2, "Farm": 2, "Island": 2, "Tunnel": 2, "Cemetery": 2, "Farmland": 2,
	"Distant Shore": 2, "Stronghold": 2, "Small Castle": 2, "Haunted Castle": 2, "Dame Josephine": 2,
	"Opulent Castle": 3, "Sprawling Castle": 4, "Grand Castle": 5,
}

// stateVP is VP from States a player holds at the end (Misery).
var stateVP = map[string]int{"Miserable": -2, "Twice Miserable": -4}

// Landmarks that score at the end of the game. Landmarks that hand out VP tokens
// during the game (Arena, Basilica, ...) are counted through the tokens.
var scoringLandmarks = map[string]bool{
	"Bandit Fort": true, "Fountain": true, "Keep": true, "Museum": true, "Obelisk": true, "Orchard": true,
	"Palace": true, "Tower": true, "Triumphal Arch": true, "Wall": true, "Wolf Den": true,
}

// ScoreInput is what scoring needs beyond the log.
type ScoreInput struct {
	Landscapes  []string // landmarks and allies in the game, e.g. "Wall", "Plateau Shepherds"
	ObeliskPile string   // the pile Obelisk chose; read from the log when empty
}

// VPBreakdown lists where a player's VP come from, e.g. "Province" 18, "Wall" -13, "VP tokens" 7.
type VPBreakdown map[string]int

func (b VPBreakdown) Total() int {
	t := 0
	for _, v := range b {
		t += v
	}
	return t
}

// Score computes every player's VP after the last event.
func (c *Client) Score(g *Game, in ScoreInput) map[string]VPBreakdown {
	turns := g.Turns()
	decks := g.FinalDecks()
	tokens := map[string]int{}
	favors := map[string]int{}
	if len(turns) > 0 {
		for p, cs := range turns[len(turns)-1].Counters {
			tokens[p], favors[p] = cs.VPTokens, cs.Favors
		}
	}
	states := g.States()
	tavern := g.OnMat("Tavern")
	supply := g.Supply(c)
	obelisk := in.ObeliskPile
	if obelisk == "" {
		for _, e := range g.Events {
			if e.Template == "OBELISK_CHOOSES_PILE" {
				obelisk = c.PileOf(pileName(e.Args[0].Text))
			}
		}
	}
	has := map[string]bool{}
	for _, l := range in.Landscapes {
		has[l] = true
	}

	out := map[string]VPBreakdown{}
	for _, p := range g.Players {
		d := decks[p]
		b := VPBreakdown{}
		count := func(pred func(*Card) bool) int {
			n := 0
			for name, k := range d {
				if card := c.Cards[name]; card != nil && pred(card) {
					n += k
				}
			}
			return n
		}
		total := 0
		for _, k := range d {
			total += k
		}
		distinct := len(d)
		victory := count(func(x *Card) bool { return x.Is("Victory") })
		castles := count(func(x *Card) bool { return x.Is("Castle") })
		victoryNames := 0
		for name := range d {
			if card := c.Cards[name]; card != nil && card.Is("Victory") {
				victoryNames++
			}
		}
		for name, k := range d {
			v := fixedVP[name] * k
			switch name {
			case "Gardens":
				v = total / 10 * k
			case "Duke":
				v = d["Duchy"] * k
			case "Fairgrounds":
				v = distinct / 5 * 2 * k
			case "Feodum":
				v = d["Silver"] / 3 * k
			case "Vineyard":
				v = count(func(x *Card) bool { return x.Is("Action") }) / 3 * k
			case "Humble Castle":
				v = castles * k
			case "King's Castle":
				v = 2 * castles * k
			case "Pasture":
				v = d["Estate"] * k
			case "Marchland":
				v = victory / 3 * k
			case "Silk Road":
				v = victory / 4 * k
			case "Territory":
				v = victoryNames * k
			case "Demesne":
				v = d["Gold"] * k
			case "Distant Lands":
				v = 4 * min(k, tavern[p]["Distant Lands"])
			}
			if v != 0 {
				b[name] += v
			}
		}
		for st, n := range states[p] {
			if v, ok := stateVP[st]; ok && n > 0 {
				b[st] += v * n
			}
		}
		if tokens[p] != 0 {
			b["VP tokens"] = tokens[p]
		}
		for l := range has {
			v := 0
			switch l {
			case "Bandit Fort":
				v = -2 * (d["Silver"] + d["Gold"])
			case "Fountain":
				if d["Copper"] >= 10 {
					v = 15
				}
			case "Keep":
				for name, k := range d {
					card := c.Cards[name]
					if card == nil || !card.Is("Treasure") {
						continue
					}
					most := true
					for _, q := range g.Players {
						if q != p && decks[q][name] > k {
							most = false
						}
					}
					if most {
						v += 5
					}
				}
			case "Museum":
				v = 2 * distinct
			case "Obelisk":
				if obelisk != "" {
					for name, k := range d {
						if c.PileOf(name) == obelisk {
							v += 2 * k
						}
					}
				}
			case "Orchard":
				for name, k := range d {
					if card := c.Cards[name]; card != nil && card.Is("Action") && k >= 3 {
						v += 4
					}
				}
			case "Palace":
				v = 3 * min(d["Copper"], d["Silver"], d["Gold"])
			case "Tower":
				for name, k := range d {
					card := c.Cards[name]
					if card != nil && !card.Is("Victory") && supply.Empty(c.PileOf(name)) {
						v += k
					}
				}
			case "Triumphal Arch":
				var counts []int
				for name, k := range d {
					if card := c.Cards[name]; card != nil && card.Is("Action") {
						counts = append(counts, k)
					}
				}
				sort.Sort(sort.Reverse(sort.IntSlice(counts)))
				if len(counts) >= 2 {
					v = 3 * counts[1]
				}
			case "Wall":
				if total > 15 {
					v = -(total - 15)
				}
			case "Wolf Den":
				for _, k := range d {
					if k == 1 {
						v -= 3
					}
				}
			case "Plateau Shepherds":
				twos := count(func(x *Card) bool { return x.Coin == 2 && x.Potion == 0 && x.Debt == 0 })
				v = 2 * min(favors[p], twos)
			}
			if v != 0 {
				b[l] = v
			}
		}
		out[p] = b
	}
	return out
}
