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
		"INHERITS": true, // Inheritance sets a Supply card aside for the player; the score screen counts it
	}
	losses = map[string]bool{
		"TRASH": true, "TRASH_WITH": true,
		"EXCHANGE_RETURN": true, "RETURN": true, // "returns a Peasant" when exchanging a Traveller
	}
	toPile = map[string]bool{"RETURN_TO": true, "PUT_ON_BOTTOM_OF": true} // only when the target is a pile

	// The log doesn't say where an exiled card came from. These cards exile from
	// the Supply, which makes the card the player's (Exile counts as owned);
	// everything else exiles a card the player already has.
	supplyExilers = map[string]bool{"Camel Train": true, "Transport": true, "Invest": true, "Coven": true, "Enclave": true, "Way of the Worm": true, "Way of the Camel": true}

	// Likewise these trash a card from the Supply, not one the player owns.
	supplyTrashers = map[string]bool{"Gladiator": true, "Lurker": true, "Salt the Earth": true}

	// Cards that move themselves out of the trash right after being trashed were
	// never lost: Fortress goes to the hand, Lich to the discard pile, and a Trail
	// played from the trash goes into play. Other cards reacting to a trash
	// (Market Square discards other copies) don't count.
	backFromTrash = map[string]map[string]bool{
		"Fortress": {"PUT_IN_HAND": true, "PUT_IN_HAND_FROM": true},
		"Lich":     {"DISCARD": true, "DISCARD_WITH": true, "PUT_IN_DISCARD": true}, // "discards a Lich. (Lich)"
		"Trail":    {"REACTS_PLAYING": true},
	}
	// where each of those lines puts the card
	backFromTrashZone = map[string]string{
		"PUT_IN_HAND": ZoneHand, "PUT_IN_HAND_FROM": ZoneHand,
		"DISCARD": ZoneDiscard, "DISCARD_WITH": ZoneDiscard, "PUT_IN_DISCARD": ZoneDiscard,
		"REACTS_PLAYING": ZonePlay,
	}
)

// trashesSupply reports a trash from the Supply: a Lurker played, not one a
// Watchtower trashes as it's gained.
func trashesSupply(e Event) bool { return supplyTrashers[e.Cause] && !gains[e.CauseOf] }

// backFromTrashed reports a line that takes a card just trashed out of the
// trash; the trash line may name it after another: "trashes a Curse and a Trail".
func backFromTrashed(e Event, cards []CardN) bool {
	return len(cards) == 1 && cards[0].Card != nil && backFromTrash[cards[0].Card.Name][e.Template] && strings.HasPrefix(e.CauseOf, "TRASH")
}

// Change is one change of ownership: Sign is +1 for cards gained, -1 for cards lost.
type Change struct {
	Player string
	Cards  []CardN
	Sign   int
}

// Changes says how an event changes who owns which cards.
func (e Event) Changes() []Change {
	cards := e.Args[1].Cards
	switch {
	case gains[e.Template]:
		return []Change{{e.Player, cards, 1}}
	case losses[e.Template] && !(strings.HasPrefix(e.Template, "TRASH") && trashesSupply(e)):
		return []Change{{e.Player, cards, -1}}
	case toPile[e.Template] && toSupplyPile(e.Args[2].Text):
		return []Change{{e.Player, cards, -1}}
	case backFromTrashed(e, cards):
		return []Change{{e.Player, cards, 1}}
	case e.Template == "EXILE" && supplyExilers[e.Cause]:
		return []Change{{e.Player, cards, 1}}
	case e.Template == "PASS":
		return []Change{{e.Player, cards, -1}, {e.Args[2].Player, cards, 1}}
	}
	return nil
}

// Decks holds every player's deck by player name.
type Decks map[string]Deck

// toSupplyPile is true for "the Horse pile" and "the Box" (out of the game),
// false for the player's own "Draw Pile".
func toSupplyPile(target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	return t == "the box" || strings.HasPrefix(t, "the ") && strings.HasSuffix(t, " pile")
}

// Apply adds an event's changes to the decks.
func (decks Decks) Apply(e Event) {
	for _, ch := range e.Changes() {
		if ch.Player == "" {
			continue
		}
		if decks[ch.Player] == nil {
			decks[ch.Player] = Deck{}
		}
		d := decks[ch.Player]
		for _, c := range ch.Cards {
			name := "unknown"
			if c.Card != nil {
				if !c.Card.InDeck() {
					continue
				}
				name = c.Card.Name
			}
			d[name] += ch.Sign * c.N
			if d[name] == 0 {
				delete(d, name)
			}
		}
	}
}

// FinalDecks replays every change of ownership.
func (g *Game) FinalDecks() Decks {
	decks := Decks{}
	for _, p := range g.Players {
		decks[p] = Deck{}
	}
	for _, e := range g.Events {
		decks.Apply(e)
	}
	return decks
}
