package cartographer

import "fmt"

// Counters are a player's tokens and balances.
type Counters struct {
	Coffers   int `json:"coffers"`
	Villagers int `json:"villagers"`
	Favors    int `json:"favors"`
	Debt      int `json:"debt"`
	VPTokens  int `json:"vp_tokens"`
}

// Turn sums up one turn and the state after it.
type Turn struct {
	Number   int                 `json:"number"`
	Player   string              `json:"player"`
	Extra    string              `json:"extra,omitempty"` // what gave an extra turn, e.g. "Voyage"
	Coins    int                 `json:"coins"`           // $ from Treasures, +$ and Coffers this turn
	Bought   []CardN             `json:"bought"`          // cards and landscapes bought
	Gained   []CardN             `json:"gained"`          // other gains (Horses, Groom, Shaman, ...)
	Trashed  []CardN             `json:"trashed"`         // what the turn's player trashed
	Actions  int                 `json:"actions"`         // Action cards played
	Drawn    int                 `json:"drawn"`           // cards drawn, including the next hand
	Shuffles int                 `json:"shuffles"`
	Decks    Decks               `json:"decks"`    // every player's owned cards after the turn
	Counters map[string]Counters `json:"counters"` // every player's tokens after the turn
}

// Templates by turn column. The number is the argument that holds the amount.
var (
	coinArg = map[string]int{
		"PLAY_TREASURES_FOR": 2, "PLAY_TREASURES_AGAIN_FOR": 2, "PLAY_TREASURES_THIRD_FOR": 2,
		"GETS_COIN": 1, "GETS_COINS": 1, "GETS_COIN_FROM": 1, "GETS_COINS_FROM": 1,
		"USES_COFFER": 1, "USES_COFFERS": 1,
	}
	boughtT  = map[string]bool{"BUY": true, "BUY_AND_GAIN": true}
	gainedT  = map[string]bool{"GAIN": true, "GAIN_WITH": true, "GAIN_ON_DRAWPILE": true, "GAIN_FROM_TRASH": true, "GAIN_SETTING_ASIDE": true, "GAIN_TO_HAND": true}
	trashedT = map[string]bool{"TRASH": true, "TRASH_WITH": true}
	playedT  = map[string]bool{"PLAY": true, "PLAY_AGAIN": true, "PLAY_THIRD": true, "PLAY_WITH": true, "PLAY_AGAIN_WITH": true, "PLAY_THIRD_WITH": true, "PLAY_USING_WAY": true, "REACTS_PLAYING": true}
	drawnT   = map[string]bool{"DRAW": true, "DRAW_FROM": true, "DRAWS_FROM": true}

	// counter changes: template -> (counter, sign); the amount is Argument1
	counterT = map[string]struct {
		field string
		sign  int
	}{
		"GETS_COFFER": {"coffers", 1}, "GETS_COFFERS": {"coffers", 1}, "GETS_COFFER_FROM": {"coffers", 1}, "GETS_COFFERS_FROM": {"coffers", 1},
		"USES_COFFER": {"coffers", -1}, "USES_COFFERS": {"coffers", -1},
		"GETS_VILLAGER": {"villagers", 1}, "GETS_VILLAGERS": {"villagers", 1}, "GETS_VILLAGER_FROM": {"villagers", 1}, "GETS_VILLAGERS_FROM": {"villagers", 1},
		"USES_VILLAGER": {"villagers", -1}, "USES_VILLAGERS": {"villagers", -1},
		"GETS_FAVOR": {"favors", 1}, "GETS_FAVORS": {"favors", 1}, "GETS_FAVOR_FROM": {"favors", 1}, "GETS_FAVORS_FROM": {"favors", 1},
		"SPENDS_FAVOR": {"favors", -1}, "SPENDS_FAVORS": {"favors", -1},
		"TAKES_DEBT": {"debt", 1}, "REPAYS_DEBT": {"debt", -1}, // REPAYS_SOME_DEBT sets the balance, below
		"PLAYER_GETS_VP": {"vp", 1}, "PLAYER_GETS_VP_FROM": {"vp", 1}, "TAKE_VP_FROM": {"vp", 1},
	}
)

func amount(a Arg) int {
	if a.Number != nil {
		return *a.Number
	}
	return 0
}

func (cs *Counters) add(field string, n int) {
	switch field {
	case "coffers":
		cs.Coffers += n
	case "villagers":
		cs.Villagers += n
	case "favors":
		cs.Favors += n
	case "debt":
		cs.Debt += n
	case "vp":
		cs.VPTokens += n
	}
}

// Turns replays the game turn by turn. Decks and counters change on every
// player's events, including during the other player's turn (attacks,
// start-of-turn gains). Events before turn 1 (starting decks) count too.
func (g *Game) Turns() []Turn {
	decks := Decks{}
	counters := map[string]*Counters{}
	for _, p := range g.Players {
		decks[p] = Deck{}
		counters[p] = &Counters{}
	}
	var turns []Turn
	var cur *Turn
	finish := func() {
		if cur == nil {
			return
		}
		cur.Decks = Decks{}
		cur.Counters = map[string]Counters{}
		for p, d := range decks {
			cp := Deck{}
			for k, v := range d {
				cp[k] = v
			}
			cur.Decks[p] = cp
		}
		for p, c := range counters {
			cur.Counters[p] = *c
		}
		turns = append(turns, *cur)
		cur = nil
	}
	for _, e := range g.Events {
		if e.Template == "NEW_TURN" {
			finish()
			cur = &Turn{Number: e.Turn, Player: e.Player, Extra: e.Extra, Bought: []CardN{}, Gained: []CardN{}, Trashed: []CardN{}}
			continue
		}
		decks.Apply(e)
		if ct, ok := counterT[e.Template]; ok && e.Player != "" {
			if counters[e.Player] == nil {
				counters[e.Player] = &Counters{}
			}
			counters[e.Player].add(ct.field, ct.sign*amount(e.Args[1]))
		}
		if e.Player != "" && counters[e.Player] != nil {
			cs := counters[e.Player]
			switch e.Template {
			case "COFFERS_FROM_PATRON":
				cs.Coffers++
			case "BUY", "BUY_AND_GAIN": // the log doesn't say "takes N debt" when buying a card with a debt cost
				for _, c := range e.Args[1].Cards {
					if c.Card != nil {
						cs.Debt += c.Card.Debt * c.N
					}
				}
			case "TAX_TAKE_DEBT": // Tax: "takes 1 Debt from the Moneylender pile"
				var n int
				fmt.Sscan(e.Args[1].Text, &n)
				cs.Debt += n
			case "REPAYS_SOME_DEBT": // "repays 3 debt (2 remaining)": the log's own balance
				if a := e.Args[2]; a.Number != nil {
					cs.Debt = *a.Number
				}
			}
		}
		if cur == nil || e.Player != cur.Player {
			continue
		}
		cards := e.Args[1].Cards
		if n, ok := coinArg[e.Template]; ok {
			cur.Coins += amount(e.Args[n])
		}
		switch {
		case e.Template == "PLUS_COIN_FROM":
			cur.Coins++
		case e.Template == "LOSES_COIN" || e.Template == "LOSES_COINS":
			cur.Coins -= amount(e.Args[1])
		case boughtT[e.Template]:
			cur.Bought = append(cur.Bought, cards...)
		case gainedT[e.Template]:
			cur.Gained = append(cur.Gained, cards...)
		case trashedT[e.Template]:
			cur.Trashed = append(cur.Trashed, cards...)
		case drawnT[e.Template]:
			for _, c := range cards {
				cur.Drawn += c.N
			}
		case e.Template == "SHUFFLES":
			cur.Shuffles++
		case playedT[e.Template]:
			for _, c := range cards {
				if c.Card != nil && c.Card.Is("Action") {
					cur.Actions += c.N
				}
			}
		}
	}
	finish()
	return turns
}

// Is reports whether the card has the type, e.g. "Action".
func (c *Card) Is(t string) bool {
	for _, x := range c.Types {
		if x == t {
			return true
		}
	}
	return false
}
