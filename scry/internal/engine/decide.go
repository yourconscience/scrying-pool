package engine

// Kind says what a decision is for, so an agent knows what picking an option does.
type Kind uint8

const (
	PlayAction    Kind = iota // play one Action from hand, or none to end the Action phase
	PlayTreasures             // play Treasures from hand, in the order picked
	Buy                       // buy one card, or none to end the turn
	ToGain                    // gain a card from the Supply
	ToTrash                   // trash cards
	ToDiscard                 // discard cards
	ToTopdeck                 // put cards onto the deck
	ToOrder                   // put all the cards back onto the deck; the first picked ends on top
	ToPlay                    // play a card, e.g. with Throne Room or Vassal
	ToSetAside                // set a card aside, e.g. an Action drawn by Library
	ToReveal                  // reveal a card, e.g. a Moat against an Attack
)

var kindNames = [...]string{"play an Action", "play Treasures", "buy", "gain", "trash", "discard", "topdeck", "order", "play", "set aside", "reveal"}

func (k Kind) String() string { return kindNames[k] }

// Option is one choice: a card instance, or a Supply pile (Inst -1).
type Option struct {
	Card CardID
	Inst Inst
}

// Decision is everything the engine asks a player. The answer picks between
// Min and Max distinct options, by index.
type Decision struct {
	Player  int
	Kind    Kind
	Source  CardID // the card asking, e.g. Chapel; 0 for the turn's own phases
	Options []Option
	Min     int
	Max     int
}

// Agent makes a player's decisions: a bot, a search, a network.
type Agent interface {
	Decide(v *View, d *Decision) []int
}

// View is what a player may know when deciding: their own hand, every public
// zone, and the counts of the hidden ones.
type View struct {
	g *Game
	p int
}

func (v *View) Me() int       { return v.p }
func (v *View) Table() *Table { return v.g.T }
func (v *View) Def(c CardID) *Def {
	return v.g.T.Defs[c]
}

// Hand is the deciding player's hand.
func (v *View) Hand() []CardID { return v.g.ids(v.g.P[v.p].Z[Hand]) }

// Owned is a player's whole deck by card; every gain and trash is public.
func (v *View) Owned(p int) map[CardID]int {
	out := map[CardID]int{}
	for _, x := range v.g.Owned(p) {
		out[v.g.cards[x]]++
	}
	return out
}

// InPlay is a player's cards in play.
func (v *View) InPlay(p int) []CardID { return v.g.ids(v.g.P[p].Z[InPlay]) }

// DeckSize and DiscardSize count a player's draw and discard piles.
func (v *View) DeckSize(p int) int    { return len(v.g.P[p].Z[Deck]) }
func (v *View) DiscardSize(p int) int { return len(v.g.P[p].Z[Discard]) }
func (v *View) HandSize(p int) int    { return len(v.g.P[p].Z[Hand]) }

func (v *View) Actions() int { return v.g.P[v.p].Actions }
func (v *View) Buys() int    { return v.g.P[v.p].Buys }
func (v *View) Coins() int   { return v.g.P[v.p].Coins }
func (v *View) Turn() int    { return v.g.turn }

// Left is how many cards a Supply pile has.
func (v *View) Left(c CardID) int { return v.g.Left(c) }

// Kingdom is the game's kingdom cards.
func (v *View) Kingdom() []CardID { return v.g.Kingdom }

// Piles lists the Supply piles' cards.
func (v *View) Piles() []CardID {
	out := make([]CardID, len(v.g.Piles))
	for i, p := range v.g.Piles {
		out[i] = p.Card
	}
	return out
}

func (v *View) EmptyPiles() int   { return v.g.EmptyPiles() }
func (v *View) VP(p int) int      { return v.g.VP(p) }
func (v *View) Cost(c CardID) int { return v.g.Cost(c) }
