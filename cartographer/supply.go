package cartographer

import "strings"

// Supply sizes follow the setup rules. For 2 players:
//   - kingdom piles have 10 cards, or 8 for Victory cards;
//   - split piles have 5 + 5 cards; rotating Allies piles 4 × 4; Knights 10; Castles 8; Ruins 10;
//   - basic cards: Copper 60 − 7 per player, Silver 40, Gold 30, Platinum 12, Potion 16,
//     Curse 10 per player after the first, and Victory cards 8 (12 with 3 or more players).
// Piles outside the Supply (Horses, Spoils, Travellers, ...) are tracked too, but
// don't count for "empty Supply pile" rules.

var pileGroups = func() map[string]string {
	m := map[string]string{}
	add := func(pile string, cards ...string) {
		for _, c := range cards {
			m[c] = pile
		}
	}
	add("Catapult", "Catapult", "Rocks")
	add("Encampment", "Encampment", "Plunder")
	add("Gladiator", "Gladiator", "Fortune")
	add("Patrician", "Patrician", "Emporium")
	add("Settlers", "Settlers", "Bustling Village")
	add("Sauna", "Sauna", "Avanto")
	// the client's kingdom viewer names the rotating piles "Augur Pile" and so on
	add("Augurs", "Augur Pile", "Herb Gatherer", "Acolyte", "Sorceress", "Sibyl")
	add("Clashes", "Clash Pile", "Battle Plan", "Archer", "Warlord", "Territory")
	add("Forts", "Fort Pile", "Tent", "Garrison", "Hill Fort", "Stronghold")
	add("Odysseys", "Odyssey Pile", "Old Map", "Voyage", "Sunken Treasure", "Distant Shore")
	add("Townsfolk", "Townsfolk Pile", "Town Crier", "Blacksmith", "Miller", "Elder")
	add("Wizards", "Wizard Pile", "Student", "Conjurer", "Sorcerer", "Lich")
	add("Knights", "Dame Anna", "Dame Josephine", "Dame Molly", "Dame Natalie", "Dame Sylvia",
		"Sir Bailey", "Sir Destry", "Sir Martin", "Sir Michael", "Sir Vander")
	add("Castles", "Humble Castle", "Crumbling Castle", "Small Castle", "Haunted Castle",
		"Opulent Castle", "Sprawling Castle", "Grand Castle", "King's Castle")
	add("Ruins", "Abandoned Mine", "Ruined Library", "Ruined Market", "Ruined Village", "Survivors")
	return m
}()

// nonSupply piles and their sizes; Travellers' upgrades have 5 each.
var nonSupply = map[string]int{
	"Horse": 30, "Spoils": 15, "Wish": 12, "Will-o'-Wisp": 12, "Imp": 13, "Ghost": 6, "Bat": 10,
	"Madman": 10, "Mercenary": 10, "Loot": 30,
	"Treasure Hunter": 5, "Warrior": 5, "Hero": 5, "Champion": 5,
	"Soldier": 5, "Fugitive": 5, "Disciple": 5, "Teacher": 5,
}

var groupSize = map[string]int{
	"Catapult": 10, "Encampment": 10, "Gladiator": 10, "Patrician": 10, "Settlers": 10, "Sauna": 10,
	"Augurs": 16, "Clashes": 16, "Forts": 16, "Odysseys": 16, "Townsfolk": 16, "Wizards": 16,
	"Knights": 10,
}

// PileOf names the pile a card comes from: "Plunder" is in the "Encampment" pile,
// every Loot in "Loot", most cards in their own pile.
func (c *Client) PileOf(card string) string {
	if p, ok := pileGroups[card]; ok {
		return p
	}
	if x := c.Cards[card]; x != nil && x.Is("Loot") {
		return "Loot"
	}
	return card
}

// InitialSize is a pile's size at the start of a game with n players.
func (c *Client) InitialSize(pile string, n int) int {
	if s, ok := nonSupply[pile]; ok {
		return s
	}
	if s, ok := groupSize[pile]; ok {
		return s
	}
	victory := 8
	if n > 2 {
		victory = 12
	}
	switch pile {
	case "Copper":
		return 60 - 7*n
	case "Silver":
		return 40
	case "Gold":
		return 30
	case "Platinum":
		return 12
	case "Potion":
		return 16
	case "Curse":
		return 10 * (n - 1)
	case "Ruins":
		return 10 * (n - 1)
	case "Castles":
		return victory
	case "Port":
		return 12
	case "Rats":
		return 20
	}
	if x := c.Cards[pile]; x != nil && x.Is("Victory") {
		return victory
	}
	return 10
}

// SupplyState is what is left in each pile the game touched.
type SupplyState struct {
	Initial map[string]int
	Left    map[string]int
	Trash   Deck
}

// Empty reports whether a Supply pile was emptied.
func (s SupplyState) Empty(pile string) bool {
	return s.InSupply(pile) && s.Initial[pile] > 0 && s.Left[pile] <= 0
}

// InSupply reports whether a pile the game touched is a Supply pile (Horses,
// Spoils and the like are not).
func (s SupplyState) InSupply(pile string) bool {
	_, outside := nonSupply[pile]
	_, touched := s.Initial[pile]
	return touched && !outside
}

// EmptyPiles lists the emptied Supply piles.
func (s SupplyState) EmptyPiles() []string {
	var out []string
	for p := range s.Initial {
		if s.Empty(p) {
			out = append(out, p)
		}
	}
	return out
}

// trashGainers can gain a card from the trash instead of the Supply. Which one
// they did is decided by whether the trash holds that card.
var trashGainers = map[string]bool{"Shaman": true, "Graverobber": true, "Rogue": true, "Lurker": true, "Lich": true}

// Supply replays the piles: gains take from them (or from the trash), returns
// put back, and some cards trash or exile straight from the Supply.
func (g *Game) Supply(c *Client) SupplyState {
	n := len(g.Players)
	s := SupplyState{Initial: map[string]int{}, Left: map[string]int{}, Trash: Deck{}}
	take := func(card string, k int) {
		pile := c.PileOf(card)
		if _, ok := s.Initial[pile]; !ok {
			s.Initial[pile] = c.InitialSize(pile, n)
			s.Left[pile] = s.Initial[pile]
		}
		s.Left[pile] -= k
	}
	for _, e := range g.Events {
		cards := e.Args[1].Cards
		for _, cn := range cards {
			if cn.Card == nil || !cn.Card.InDeck() {
				continue
			}
			name := cn.Card.Name
			switch {
			case e.Template == "STARTS_WITH":
			case e.Template == "GAIN_FROM_TRASH":
				s.Trash[name] -= cn.N
			case gains[e.Template]:
				source := e.Cause
				if e.Template == "GAIN_WITH" { // "gains an Estate. (Shaman)"
					if cs := e.Args[2].Cards; len(cs) > 0 && cs[0].Card != nil {
						source = cs[0].Card.Name
					}
				}
				if trashGainers[source] && s.Trash[name] >= cn.N {
					s.Trash[name] -= cn.N
				} else {
					take(name, cn.N)
				}
			case strings.HasPrefix(e.Template, "TRASH"):
				s.Trash[name] += cn.N
				if supplyTrashers[e.Cause] {
					take(name, cn.N)
				}
			case e.Template == "EXILE" && supplyExilers[e.Cause]:
				take(name, cn.N)
			case toPile[e.Template] && toSupplyPile(e.Args[2].Text) && strings.ToLower(e.Args[2].Text) != "the box":
				take(name, -cn.N)
			case losses[e.Template] && !strings.HasPrefix(e.Template, "TRASH"): // Traveller exchanged back to its pile
				take(name, -cn.N)
			}
			if backFromTrash[e.Cause][e.Template] && strings.HasPrefix(e.CauseOf, "TRASH") && name == e.Cause {
				s.Trash[name] -= cn.N
			}
		}
	}
	return s
}

// pileName turns "the Smithy pile" into "Smithy".
func pileName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "the ")
	return strings.TrimSuffix(s, " pile")
}

// States lists the States each player holds (Misery's Miserable, Lost in the Woods, ...).
func (g *Game) States() map[string]Deck {
	out := map[string]Deck{}
	for _, e := range g.Events {
		sign := 0
		switch e.Template {
		case "TAKE", "RECEIVES", "EXCHANGE_RECEIVE":
			sign = 1
		case "RETURN", "EXCHANGE_RETURN":
			sign = -1
		}
		if sign == 0 || e.Player == "" {
			continue
		}
		for _, cn := range e.Args[1].Cards {
			if cn.Card == nil || !cn.Card.Is("State") {
				continue
			}
			if out[e.Player] == nil {
				out[e.Player] = Deck{}
			}
			out[e.Player][cn.Card.Name] += sign * cn.N
		}
	}
	return out
}

// OnMat counts the cards each player has on a mat at the end, e.g. "Tavern".
func (g *Game) OnMat(mat string) map[string]Deck {
	out := map[string]Deck{}
	add := func(p string, cs []CardN, sign int) {
		if p == "" {
			return
		}
		if out[p] == nil {
			out[p] = Deck{}
		}
		for _, cn := range cs {
			if cn.Card != nil {
				out[p][cn.Card.Name] += sign * cn.N
			}
		}
	}
	for _, e := range g.Events {
		switch {
		case e.Template == "PUT_ON_MAT" && strings.Contains(e.Args[2].Text, mat):
			add(e.Player, e.Args[1].Cards, 1)
		case e.Template == "CALL" && mat == "Tavern":
			add(e.Player, e.Args[1].Cards, -1)
		}
	}
	return out
}
