package cartographer

import "strings"

// Deck counts the cards a player owns, by card name. Cards on mats, set aside or
// in Exile still belong to the player.
type Deck map[string]int

// Ownership changes by template. Everything else (plays, draws, discards,
// topdecks, reveals) moves cards between a player's own zones.
var (
	gains = map[string]bool{
		"STARTS_WITH": true, "GAIN": true, "GAIN_WITH": true, "BUY_AND_GAIN": true,
		"GAIN_ON_DRAWPILE": true, "GAIN_FROM_TRASH": true, "GAIN_SETTING_ASIDE": true, "GAIN_TO_HAND": true,
		"EXCHANGE_RECEIVE": true, "RECEIVES": true, // Travellers exchange; Boons are filtered by InDeck
	}
	losses = map[string]bool{
		"TRASH": true, "TRASH_WITH": true,
		"EXCHANGE_RETURN": true, "RETURN": true, // "returns a Peasant" when exchanging a Traveller
	}
	toPile = map[string]bool{"RETURN_TO": true, "PUT_ON_BOTTOM_OF": true} // only when the target is a pile

	// The log doesn't say where an exiled card came from. These cards exile from
	// the Supply, which makes the card the player's (Exile counts as owned);
	// everything else exiles a card the player already has.
	supplyExilers = map[string]bool{"Camel Train": true, "Transport": true, "Invest": true, "Way of the Worm": true, "Coven": true}
)

// FinalDecks replays every change of ownership.
func (g *Game) FinalDecks() map[string]Deck {
	decks := map[string]Deck{}
	for _, p := range g.Players {
		decks[p] = Deck{}
	}
	add := func(player string, cs []CardN, sign int) {
		if player == "" {
			return
		}
		if decks[player] == nil {
			decks[player] = Deck{}
		}
		for _, c := range cs {
			name := "unknown"
			if c.Card != nil {
				if !c.Card.InDeck() {
					continue
				}
				name = c.Card.Name
			}
			decks[player][name] += sign * c.N
			if decks[player][name] == 0 {
				delete(decks[player], name)
			}
		}
	}
	for _, e := range g.Events {
		cards := e.Args[1].Cards
		switch {
		case gains[e.Template]:
			add(e.Player, cards, 1)
		case losses[e.Template]:
			add(e.Player, cards, -1)
		case toPile[e.Template] && strings.HasSuffix(strings.ToLower(e.Args[2].Text), "pile"):
			add(e.Player, cards, -1)
		case e.Template == "EXILE" && supplyExilers[e.Cause]:
			add(e.Player, cards, 1)
		case e.Template == "PASS":
			add(e.Player, cards, -1)
			add(e.Args[2].Player, cards, 1)
		}
	}
	return decks
}
