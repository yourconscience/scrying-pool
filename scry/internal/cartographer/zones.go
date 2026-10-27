package cartographer

import (
	"slices"
	"sort"
	"strings"
)

// Zones say where each player's cards are: hand, draw pile, discard pile, in
// play, set aside, on a mat or in Exile. The log names most moves; where it
// doesn't say where a card came from, the replay takes it from the zone the
// card's template and cause imply, and records a ZoneMiss when the card isn't
// there. A card whose name the log hides (an opponent's "draws 5 cards") is
// counted in its zone as unknown; Pool tells which cards those can be.

const (
	ZoneHand          = "hand"
	ZoneDeck          = "deck" // the draw pile
	ZoneDiscard       = "discard"
	ZonePlay          = "in play"
	ZoneSetAside      = "set aside"
	ZoneRevealed      = "revealed" // looked at or revealed from the deck, until the card goes somewhere
	ZoneExile         = "Exile"
	ZoneTavern        = "Tavern mat"
	ZoneIsland        = "Island mat"
	ZoneNativeVillage = "Native Village mat"
)

// zoneOrder is the order zones are searched when a card isn't where it should be.
var zoneOrder = []string{ZoneRevealed, ZoneHand, ZonePlay, ZoneSetAside, ZoneDiscard, ZoneDeck, ZoneTavern, ZoneIsland, ZoneNativeVillage, ZoneExile}

// Cards in one zone: named ones, and how many the log doesn't name.
type Cards struct {
	Known   Deck `json:"known"`
	Unknown int  `json:"unknown,omitempty"`
}

func (c *Cards) Size() int {
	n := c.Unknown
	for _, k := range c.Known {
		n += k
	}
	return n
}

// Zones are one player's zones by name.
type Zones map[string]*Cards

func (z Zones) get(name string) *Cards {
	if z[name] == nil {
		z[name] = &Cards{Known: Deck{}}
	}
	return z[name]
}

func (z Zones) copy() Zones {
	out := Zones{}
	for name, c := range z {
		if c.Size() == 0 {
			continue
		}
		k := Deck{}
		for n, v := range c.Known {
			k[n] = v
		}
		out[name] = &Cards{Known: k, Unknown: c.Unknown}
	}
	return out
}

// Pool is what the unknown cards can be: the owned cards not seen in any zone.
func (z Zones) Pool(owned Deck) Deck {
	pool := Deck{}
	for n, k := range owned {
		pool[n] = k
	}
	for _, c := range z {
		for n, k := range c.Known {
			pool[n] -= k
			if pool[n] == 0 {
				delete(pool, n)
			}
		}
	}
	return pool
}

// ZoneMiss is a card that wasn't in the zone the replay expected.
type ZoneMiss struct {
	Line     int
	Player   string
	Template string
	Cause    string
	Card     string // "" for a card the log doesn't name
	Want     string // the zones searched first
	Found    string // where it was instead; "" when the player had no such card anywhere
}

// Cards that look at or reveal cards from the top of a deck (their own, or an
// attacked player's). Their reveals take the cards out of the deck until they
// go somewhere; the rest go back on top.
//
// The first editions' cards are here too, for the engine's games.
var deckLookers = names("Adventurer", "Advisor", "Apothecary", "Archive", "Bandit", "Border Guard", "Cardinal",
	"Carnival", "Cartographer", "Catacombs", "Crystal Ball", "Duchess", "Envoy",
	"Famine", "Foresight", "Forest Dwellers", "Fortune Hunter", "Gamble", "Ghost",
	"Giant", "Golem", "Herald", "Hunter", "Hunting Party", "Ironmonger", "Jack of All Trades", "Journeyman",
	"Library", "Loan", "Locusts", "Lookout", "Magpie", "Mapmaker", "Miller", "Mystic", "Native Village", "Navigator",
	"Night Watchman", "Noble Brigand", "Oracle", "Patrician", "Patrol", "Piazza", "Pirate Ship", "Poet", "Pursue",
	"Rabble", "Raze", "Rebuild", "Rogue", "Sage", "Scouting Party", "Scrying Pool",
	"Sea Chart", "Seer", "Sentinel", "Sentry", "Sextant", "Sorcerer", "Sorceress", "Survivors", "Swindler",
	"The Sun's Gift", "Vagrant", "Venture", "Wandering Minstrel", "War", "Will-o'-Wisp",
	"Wishing Well", "Zombie Spy", "Zombie Mason",
	"Dame Anna", "Dame Josephine", "Dame Molly", "Dame Natalie", "Dame Sylvia",
	"Sir Bailey", "Sir Destry", "Sir Martin", "Sir Michael", "Sir Vander")

// Cards that look through a discard pile: what they reveal stays there.
var discardLookers = names("Annex", "Bad Omens", "Bury", "Bustling Village", "Counting House", "Courier", "Harbinger", "Herald",
	"Herb Gatherer", "Hermit", "Inn", "March", "Mountain Village", "Orb", "Scavenger", "Settlers", "The Moon's Gift")

// Cards that discard or trash the top card of a deck without revealing it first.
var (
	topDiscarders = names("Courier", "Gamble", "Jester", "Sea Hag", "Vassal", "Warrior")
	topTrashers   = names("Barbarian", "Locusts", "Swindler", "Zombie Mason")
)

// Cards in play that trash themselves when their player gains a card: a Cage
// for a Victory card, a Search when the gain empties a pile.
var trashedFromPlayOnGain = names("Cage", "Search")

// Cards gained somewhere other than the discard pile, without a line saying so:
// by the card gained, or by what gained it (the line above, or "(Sickness)").
var (
	gainedToHand   = names("Den of Sin", "Ghost Town", "Guardian", "Night Watchman")
	gainedOntoDeck = names("Nomad Camp")
	gainsToHand    = names("Artisan", "Beggar", "Cobbler", "Falconer", "Joust", "Kind Emperor",
		"Mine", "Pickaxe", "Pirate", "Plague", "Rocks", "Sculptor", "Silver Mine", "Swap", "Torturer", "Trading Post",
		"Transmogrify", "Treasurer", "Wish")
	gainsOntoDeck = names("Armory", "Artificer", "Bureaucrat", "Crafters' Guild", "Demand", "Develop", "Fool's Gold",
		"Greed", "Sickness", "Stampede", "Supplies")
	gainsSettingAside = names("Quartermaster")
	// Research sets cards aside from the deck, not the hand
	setsAsideFromDeck = names("Research")
	// cards that play an Action more than once, and stay in play with a Duration it is
	playsTwice = names("Throne Room", "King's Court", "Crown", "Procession", "Disciple", "Coronet")
)

// Reactions that play themselves from hand when a card is gained.
var playsFromHandOnGain = names("Black Cat", "Falconer", "Sheepdog")

// Replays that play a card again without moving it, and plays of cards the player
// doesn't take (Captain plays a Supply card where it is).
var playsInPlace = names("Daimyo", "Flagship", "Captain")

// Cards that play a card from the Supply or the trash, leaving it there.
var playsFromElsewhere = names("Band of Misfits", "Captain", "Necromancer", "Overlord")

// Durations whose staying in play depends on what they did this turn.
var (
	staysIfSetAside = names("Archive", "Blockade", "Cargo Ship", "Church", "Contract", "Crypt", "Gear", "Ghost", "Grotto", "Haven", "Prince", "Research", "Royal Galley")
	staysIfDiscards = names("Secret Cave", "Tactician")
	// cards that discard or trash from hand, never themselves: a copy they name is the one in hand,
	// not one they just discarded (Count discards a Rats, then trashes the hand)
	fromHandOnly = names("Alley", "Amulet", "Bat", "Broker", "Capital City", "Carpenter", "Change", "Count", "Crucible", "Dungeon", "Enlarge", "Exorcist", "Hideout", "Innkeeper",
		"Marquis", "Modify", "Mountain Shrine", "Old Map", "Priest", "Recruiter", "Research", "Rice Broker", "River Shrine", "Rustic Village", "Scholar", "Secret Cave",
		"Student", "Swap", "Sycophant", "Tanuki")
	// cards that topdeck a card from hand, never themselves
	topdecksFromHand = names("Artisan", "Courtyard", "Haunting")
	// cards that draw, then may trash themselves: "trashes a Mining Village." is the one in play
	trashesItself = names("Mining Village", "Tragic Hero")
	// cards that draw, then return themselves: "returns an Experiment" is the one in play
	returnsItself = names("Experiment")
	// Durations that work now or next turn, by choice: the line that shows "now"
	leavesIfNow = map[string]string{"Amphora": "GETS_BUY", "Barge": "GETS_BUY", "Gondola": "GETS_COIN", "Stronghold": "GETS_COIN", "Village Green": "GETS_ACTIONS"}
	// stay for the rest of the game
	permanent = names("Champion", "Endless Chalice", "Hireling", "Quartermaster", "Samurai")
	// stay until something happens (Secluded Shrine: the next Treasure gained),
	// then leave in that turn's Clean-up; Cage and Landing Party leave by a line of their own
	untilTriggered = names("Abundance", "Cage", "Cutthroat", "Flagship", "Landing Party", "Search", "Secluded Shrine")
)

func names(ns ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range ns {
		m[n] = true
	}
	return m
}

// played is a card in play, kept to decide when it leaves.
type played struct {
	name   string
	card   *Card
	turn   int     // the turn it was played on (count of NEW_TURN lines)
	stays  int     // the owner's Clean-ups it still stays through; -1 until decided
	holds  *played // the Duration a Throne Room-like card played
	by     string  // the card whose line played it: a Throne Room
	again  bool    // played more than once: by a Throne Room, not a Shop
	agains int     // how many times it was played again
	nows   int     // its plays that worked now (Village Green): one replayed for later stays
	fired  bool    // an untilTriggered card whose time came; a Taskmaster that saw a $5 card gained this turn
	back   bool    // played by Prince: set aside again instead of discarded
	aside  bool    // its own play set cards aside: this Gear, not another one played on the turn; or discarded them (Secret Cave)
	nAside int     // how many cards its own play set aside (Archive)
	none   bool    // enchanted: its instructions didn't happen, so it doesn't stay
	spent  bool    // a Taskmaster whose turn ended without a $5 card gained, kept in play by Journey
	// inherits is the card an Estate played by Inheritance: the Estate stays in play like it
	inherits *Card
}

// galleyPlay is a card a Royal Galley played: once its lines end it is set
// aside, unless it left play (a Pillage trashes itself).
type galleyPlay struct {
	p     string
	rg, x *played
	level int
}

// pending is a card a recent line moved or looked at. Lines nested under it, or
// right after it under the same card, take that card from there first.
type pending struct {
	card, zone string
	level      int
	gained     bool
	src        string // what moved it: the card a "(Card)" names, else the line above's
	line       int    // the line that moved it
}

// source names what a line's effect comes from: "discards a Copper. (Tide Pools)".
func source(e Event) string {
	if cs := e.Args[2].Cards; strings.HasSuffix(e.Template, "_WITH") && len(cs) > 0 && cs[0].Card != nil {
		return cs[0].Card.Name
	}
	return e.Cause
}

type zoneReplay struct {
	zones      map[string]Zones
	avoid      map[string]Deck     // cards Avoid keeps out of the next shuffle
	fated      map[string][]string // Fated cards topdecked while shuffling: they go on the new deck, the last one on top
	traits     map[string]string   // the Trait on each card's pile: "The Tireless Trait is acquired by the Crown pile."
	kept       map[string]Deck     // cards kept in hand through this Clean-up
	top        map[string][]string // cards known to be on top of the deck, top first ("" for an unnamed one)
	bottom     map[string][]string // cards known to be at the bottom, the one nearest the middle first
	deckPos    string              // where the current line puts cards in the deck: "top", "bottom" or ""
	took       string              // the card the last takeOne took, when the deck's order names an unnamed one
	cleaning   string              // the player whose Clean-up has begun
	galleys    []*galleyPlay       // the cards Royal Galleys are playing, set aside when their lines end
	gang       bool                // Gang of Pickpockets is in the game: players discard down to 4 at the start of their turns
	biding     map[string][]string // the hand Biding Time set aside, back in hand unlogged at the start of the next turn ("" for an unnamed card)
	bidingDue  string              // the player whose turn began: their Biding Time cards come back with its first line, after the last turn's snapshot
	possTrash  map[string][]string // what a Possession turn trashed, set aside until the next turn begins
	shadowPlay bool                // the line plays a Shadow card, from the deck if not from hand
	owned      Decks
	received   map[string][]pending // Boons and Hexes being resolved: their lines follow at the same level
	shared     map[string][]pending // under "": the Boon just discarded, whose share Sacred Grove's other player's lines follow
	reacted    map[string][]pending // reactions being resolved: "reacts with a Tunnel", then its Gold
	playAt     map[int]*played      // the card the latest play line at each level played
	inPlay     map[string][]*played
	pend       map[string][]pending
	turn       int            // NEW_TURN lines so far
	turnOf     string         // whose turn it is
	evidence   map[string]int // this turn: "set aside:Gear", "discard:Tactician", "draw:Barge" counts
	cleanup    map[int]bool   // event indices that start a Clean-up
	events     []Event
	misses     []ZoneMiss
	line       int                   // the line being replayed
	latest     map[int]int           // the last line at each level: the current line's parents
	takes      map[string][]DeckTake // every card taken from each player's deck
	offHand    map[int][]string      // the named cards each line took from a zone other than the hand
}

func newZoneReplay(g *Game) *zoneReplay {
	r := &zoneReplay{zones: map[string]Zones{}, avoid: map[string]Deck{}, fated: map[string][]string{}, traits: map[string]string{}, kept: map[string]Deck{}, top: map[string][]string{}, bottom: map[string][]string{}, owned: Decks{}, received: map[string][]pending{}, shared: map[string][]pending{}, reacted: map[string][]pending{}, playAt: map[int]*played{}, inPlay: map[string][]*played{}, pend: map[string][]pending{}, evidence: map[string]int{}, latest: map[int]int{}}
	for _, p := range g.Players {
		r.zones[p] = Zones{}
	}
	r.cleanup = cleanupPoints(g)
	r.events = g.Events
	for _, e := range g.Events {
		if cs := e.Args[2].Cards; len(cs) > 0 && cs[0].Card != nil && cs[0].Card.Name == "Gang of Pickpockets" {
			r.gang = true
			break
		}
	}
	return r
}

var beforeShuffle = names("PUT_IN_DISCARD", "SPENDS_FAVOR", "SPENDS_FAVORS", "BOTTOMDECKS", "SHUFFLES_CARDS")

// shyDiscard: "discards a Patron.", "draws 2 cards. (Shy)": a Shy card from hand at the start of a turn.
func (r *zoneReplay) shyDiscard(i int) bool {
	j := i + 1
	for j < len(r.events) && r.events[j].Template == "SHUFFLES" { // the draw shuffles first
		j++
	}
	if j >= len(r.events) {
		return false
	}
	n := r.events[j]
	cs := n.Args[2].Cards
	return n.Template == "DRAW_FROM" && n.Player == r.events[i].Player && len(cs) > 0 && cs[0].Card != nil && cs[0].Card.Name == "Shy"
}

// movedAbove: the line above, at this level, discards or reveals the same card: "discards a Maroon.",
// "plays a Maroon." (Vassal); not a Pilgrim's "topdecks a Mining Road." nor a Gear's "sets a Guide aside"
// before an Inspiring play of another one
func (r *zoneReplay) movedAbove(i int, cards []CardN) bool {
	if i < 1 || len(cards) != 1 || cards[0].Card == nil {
		return false
	}
	x := r.events[i-1]
	if x.Template != "DISCARD" && !strings.HasPrefix(x.Template, "REVEAL") {
		return false
	}
	cs := x.Args[1].Cards
	return x.Level == r.events[i].Level && len(cs) == 1 && cs[0].Card != nil && cs[0].Card.Name == cards[0].Card.Name
}

// traitOf is the Trait on a card's pile: "the Castles pile" for a Humble Castle.
func (r *zoneReplay) traitOf(card string) string {
	if g, ok := pileGroups[card]; ok {
		return r.traits[g]
	}
	return r.traits[card]
}

// landingTop: "plays a Hammer.", ..., "topdecks a Landing Party.": a Landing Party in play
// whose time came, after the first card played on the turn, a Treasure.
func (r *zoneReplay) landingTop(i int, p string, cards []CardN) bool {
	e := r.events[i]
	if len(cards) != 1 || cards[0].Card == nil || cards[0].Card.Name != "Landing Party" || !strings.HasPrefix(e.CauseOf, "PLAY") {
		return false
	}
	c, play := r.cardOf(p, e.Cause), r.z(p)[ZonePlay]
	for k := i - 1; c == nil && k >= 0; k-- { // a Coin of the Realm already on the Tavern mat: the play line's card
		if f := r.events[k]; f.Level < e.Level {
			for _, cn := range f.Args[1].Cards {
				if cn.Card != nil && cn.Card.Name == e.Cause {
					c = cn.Card
				}
			}
			break
		}
	}
	return c != nil && c.Is("Treasure") && play != nil && play.Known["Landing Party"] > 0
}

// fatedTop: a topdeck between "shuffles 3 Coppers and a Replace." and "shuffles
// their deck.": a Fated card going on top of the new deck; or Order of Astrologers'
// between "spends 1 Favor." and the shuffle.
func fatedTop(es []Event, i int) bool {
	p, level := es[i].Player, es[i].Level
	for j := i; j >= 0 && es[j].Player == p && es[j].Level == level; j-- {
		switch es[j].Template {
		case "SHUFFLES_CARDS":
			return true
		case "SPENDS_FAVOR", "SPENDS_FAVORS":
			k := i + 1
			for k < len(es) && es[k].Template == "TOPDECK" && es[k].Player == p {
				k++
			}
			return k < len(es) && es[k].Template == "SHUFFLES" && es[k].Player == p
		case "TOPDECK":
		default:
			return false
		}
	}
	return false
}

// cleanupPoints finds where each turn's Clean-up discards: just before the turn
// player's last draw at the top level (the next hand), or the shuffle that
// draw needed. A turn without that draw cleans up at the next turn line.
func cleanupPoints(g *Game) map[int]bool {
	out := map[int]bool{}
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		p := g.Events[start].Player
		if x := g.Events[start+1 : end]; len(x) > 0 && (x[len(x)-1].Template == "SKIPS_A_TURN" || x[len(x)-1].Template == "SKIPS_A_TURN_REMAINING") {
			return // Lich: a skipped turn keeps the hand
		}
		at := end
		// before the shuffle, and what was done while shuffling: Order of Masons and
		// Avoid keep cards in the discard pile, Shadow cards go under the deck
		beforeIt := func() {
			for at-1 > start {
				x := g.Events[at-1]
				if x.Player != p || !(x.Template == "SHUFFLES" && x.Level == 0 || beforeShuffle[x.Template] || x.Template == "TOPDECK" && fatedTop(g.Events, at-1)) {
					break
				}
				at--
			}
		}
		for i := end - 1; i > start; i-- {
			e := g.Events[i]
			if e.Template == "DRAWS_LESS_BECAUSE" && e.Level == 0 && e.Player == p { // the -1 Card token: "skips a draw (because of Relic).", the draw one level in
				at = i
				break
			}
			if e.Template == "DRAW" && e.Level == 0 && e.Player == p {
				at = i
				beforeIt()
				break
			}
		}
		if x := g.Events[end-1]; at == end && x.Template == "SHUFFLES" && x.Level == 0 && x.Player == p {
			beforeIt() // nothing to draw: Avoid kept every card in the discard pile
		}
		if at < len(g.Events) {
			out[at] = true
		}
	}
	for i, e := range g.Events {
		if e.Template == "NEW_TURN" {
			flush(i)
			start = i
		}
	}
	// the log's last turn is usually cut off mid-turn: no Clean-up; but a game that ends
	// in Clean-up (a Friendly card gains a third pile's last card) logs it whole
	for i := max(start, 0); start >= 0 && i < len(g.Events); i++ {
		if e := g.Events[i]; e.Template == "BEGINS_CLEANUP_PHASE" && e.Player == g.Events[start].Player {
			flush(len(g.Events))
			break
		}
	}
	return out
}

func (r *zoneReplay) z(p string) Zones {
	if r.zones[p] == nil {
		r.zones[p] = Zones{}
	}
	return r.zones[p]
}

func (r *zoneReplay) put(p, zone, card string, n int) {
	if zone == ZoneDeck {
		for k := 0; k < n; k++ {
			switch r.deckPos {
			case "top":
				r.top[p] = append([]string{card}, r.top[p]...)
			case "bottom":
				r.bottom[p] = append(r.bottom[p], card)
			}
		}
	}
	c := r.z(p).get(zone)
	if card == "" {
		c.Unknown += n
	} else {
		c.Known[card] += n
	}
}

// takeOne removes one copy of card (or one unnamed card) from a zone if it can.
func (r *zoneReplay) takeOne(p, zone, card string, allowUnknown bool) (ok bool) {
	defer func() {
		if ok && r.offHand != nil && zone != ZoneHand && card != "" {
			r.offHand[r.line] = append(r.offHand[r.line], card)
		}
	}()
	r.took = card
	c := r.z(p)[zone]
	if c == nil {
		return false
	}
	if zone == ZoneDeck {
		if !r.takeDeck(p, c, card, allowUnknown) {
			return false
		}
		if r.takes != nil && !r.shadowPlay {
			r.takes[p] = append(r.takes[p], DeckTake{r.line, r.took})
		}
		return true
	}
	if zone == ZonePlay && card == "" { // cards in play are always named
		return false
	}
	if card != "" && c.Known[card] > 0 {
		c.Known[card]--
		if c.Known[card] == 0 {
			delete(c.Known, card)
		}
		return true
	}
	if !allowUnknown {
		return false
	}
	if card == "" && len(c.Known) > 0 { // an unnamed card from a zone with named ones: which it was isn't known
		for n, k := range c.Known {
			c.Unknown += k
			delete(c.Known, n)
		}
	}
	if c.Unknown > 0 {
		c.Unknown--
		return true
	}
	return false
}

// took is one card taken: its zone, and its name ("" if unnamed).
type took struct{ zone, name string }

// takeCard takes one copy of card (or one unnamed card) for event e, searching
// the pending cards, then the zones in want, then everywhere (a miss).
func (r *zoneReplay) takeCard(e Event, p, card string, want []string, usePending bool) took {
	zone := r.find(e, p, card, want, usePending)
	if zone != "" {
		card = r.took
	}
	return took{zone, card}
}

func (r *zoneReplay) find(e Event, p, card string, want []string, usePending bool) string {
	// what was just revealed comes first, also when the reveal didn't name the
	// cards: an opponent's Miller "looks at 4 cards", then discards "2 other cards and a Town".
	// Not below the reveal, in a play it led to: the Village a Golem plays and
	// the Trail a Lookout trashes draw from the deck, not the revealed cards.
	nearReveal := func(x pending) bool { return x.zone == ZoneRevealed && x.level >= e.Level }
	near := false
	for _, x := range r.pend[p] {
		near = near || nearReveal(x)
	}
	if c := r.z(p)[ZoneRevealed]; usePending && near && c != nil && c.Size() > 0 {
		if r.takeOne(p, ZoneRevealed, card, false) || r.couldBeUnknown(p, card) && r.takeOne(p, ZoneRevealed, card, true) {
			ps := r.pend[p]
			for i := len(ps) - 1; i >= 0; i-- {
				if nearReveal(ps[i]) && (ps[i].card == card || ps[i].card == "") {
					r.pend[p] = append(ps[:i:i], ps[i+1:]...) // the Copper a Jack looked at and discarded; then it draws from the deck, not a Golem's revealed Watchtower
					break
				}
			}
			return ZoneRevealed
		}
	}
	// an unnamed card moved under a gain is the gained card: Watchtower "topdecks a card"
	if card == "" && usePending {
		ps := r.pend[p]
		for i := len(ps) - 1; i >= 0; i-- {
			if x := ps[i]; x.gained && x.level < e.Level && contains(want, x.zone) && r.takeOne(p, x.zone, x.card, false) {
				return x.zone
			}
		}
	}
	if card != "" && usePending {
		// a revealed card first (Wandering Minstrel topdecks the Actions it
		// revealed), then a card a line above moved (a Horse returns itself to
		// its pile, not the Horse it drew), then one the same card moved just
		// before (Farmhands plays what it set aside, not what Tide Pools
		// discarded); but a trash takes from hand, not what Remake or an Old Witch just gained
		ps := r.pend[p]
		for _, pick := range []func(x pending) bool{
			nearReveal,
			func(x pending) bool { return r.above(e, x) && contains(want, x.zone) },
			func(x pending) bool {
				itself := card == e.Cause && strings.HasPrefix(e.CauseOf, "PLAY") && // not the one it drew
					(trashesItself[card] && strings.HasPrefix(e.Template, "TRASH") || returnsItself[card] && e.Template == "RETURN_TO")
				// not a card no card moved: Frog's topdecked Page isn't the one exchanged after it
				// nor one it put on the deck: two Landing Parties topdeck themselves after one Treasure
				return contains(want, x.zone) && x.src != "" && x.src == source(e) && !(x.gained && x.level == e.Level && strings.HasPrefix(e.Template, "TRASH")) && !(itself && x.zone != ZonePlay) &&
					!(x.zone == ZoneDeck && (strings.HasPrefix(e.Template, "TOPDECK") || e.Template == "INSERT_IN_DECK" || e.Template == "BOTTOMDECKS")) // a replayed Secret Passage inserts another; Sibyl puts another under
			},
		} {
			for i := len(ps) - 1; i >= 0; i-- {
				if x := ps[i]; x.card == card && pick(x) && r.takeOne(p, x.zone, card, false) {
					r.pend[p] = append(ps[:i:i], ps[i+1:]...) // it moved on: the gained Infirmary it plays is no longer in the discard pile
					return x.zone
				}
			}
		}
	}
	// zone by zone: an unnamed card in the first zone beats a named copy in a later one
	unknownOK := r.couldBeUnknown(p, card)
	for _, zone := range want {
		if r.takeOne(p, zone, card, false) || unknownOK && r.takeOne(p, zone, card, true) {
			return zone
		}
	}
	miss := ZoneMiss{Line: e.Line, Player: p, Template: e.Template, Cause: e.Cause, Card: card, Want: strings.Join(want, ",")}
	for _, allowUnknown := range []bool{false, true} {
		for _, zone := range zoneOrder {
			if r.takeOne(p, zone, card, allowUnknown) {
				miss.Found = zone
				r.misses = append(r.misses, miss)
				return zone
			}
		}
	}
	r.misses = append(r.misses, miss)
	return ""
}

// above: the pending card's line is the nearest one above e's: the Hermit a
// Horse played with Way of the Mouse trashes from the discard pile, not that Horse.
func (r *zoneReplay) above(e Event, x pending) bool {
	if x.level >= e.Level || r.latest[x.level] != x.line {
		return false
	}
	for k := x.level + 1; k < e.Level; k++ {
		if r.latest[k] > x.line {
			return false
		}
	}
	return true
}

// move takes the event's cards from the wanted zones and puts them in to.
// Cards that aren't kept in a deck (Boons, Events, States, ...) are skipped.
func (r *zoneReplay) move(e Event, p string, cards []CardN, want []string, to string) {
	var moved []pending // pending only after the line: "topdecks 2 Coppers" moves two, not one twice
	defer func() { r.pend[p] = append(r.pend[p], moved...) }()
	for _, cn := range cards {
		name := ""
		if cn.Card != nil {
			if !cn.Card.InDeck() {
				continue
			}
			name = cn.Card.Name
		}
		for k := 0; k < cn.N; k++ { // one at a time, so that a card in transit counts as seen
			t := r.takeCard(e, p, name, want, to != ZoneRevealed) // a reveal takes fresh cards from the deck
			if x := r.playAt[e.Level-1]; t.zone == ZonePlay && e.CauseOf == "PLAY_USING_WAY" && x != nil && x.name == t.name && containsPlayed(r.inPlay[p], x) {
				r.inPlay[p] = slices.DeleteFunc(r.inPlay[p], func(y *played) bool { return y == x }) // the card the Way played: Turtle sets this one aside, not one a Journey kept out
			} else if t.zone == ZonePlay {
				r.leavePlay(p, t.name)
			}
			if to == "" {
				continue
			}
			// a card found nowhere is put where the line says only if a copy is
			// unaccounted for; otherwise the replay would own more than the player
			if t.zone == "" && (t.name == "" || !r.couldBeUnknown(p, t.name)) {
				continue
			}
			r.put(p, to, t.name, 1)
			if to == ZonePlay {
				r.inPlay[p] = append(r.inPlay[p], &played{name: t.name, card: cn.Card, turn: r.turn, stays: -1})
			}
			if t.name != "" || to == ZoneRevealed { // unnamed revealed cards too: they stay revealed until the line ends
				moved = append(moved, pending{t.name, to, e.Level, false, source(e), e.Line})
			}
		}
	}
}

// gain puts newly owned cards in a zone.
func (r *zoneReplay) gain(e Event, p string, cards []CardN, to string) {
	for _, cn := range cards {
		name := ""
		if cn.Card != nil {
			if !cn.Card.InDeck() {
				continue
			}
			name = cn.Card.Name
		}
		dest := to
		if dest == ZoneDiscard {
			by := r.cause(e)
			if (e.CauseOf == "RETURN_TO" || gains[e.CauseOf]) && by != "Rocks" && by == e.Cause {
				by = "" // a Trader's Silver for a Fool's Gold: the card returned or gained didn't gain it; a gained Rocks did, and so did the Greed a gained Cursed Village gave
			}
			if a := e.Args[2]; e.Template == "GAIN_WITH" && a.Text != "" {
				by = strings.TrimSpace(a.Text)
			}
			if by == "Fool's Gold" && strings.HasPrefix(e.CauseOf, "PLAY") {
				by = "" // its Gold onto the deck is its reaction: "plays a Fool's Gold and a Treasure Trove." gains for the Trove
			}
			if rs := r.reacted[p]; len(rs) > 0 && rs[len(rs)-1].level == e.Level {
				by = "" // a reaction's: the Tunnel's Gold, not one onto the deck for the Fool's Gold discarded with it; "(Beggar)" Silver
				if rs[len(rs)-1].card == "Urchin" {
					r.reacted[p] = rs[:len(rs)-1] // its one Mercenary: the Bureaucrat's Silver after it goes onto the deck
				}
			}
			aside := gainsSettingAside[by] && !strings.HasPrefix(e.CauseOf, "TRASH") // not a trashed Squire's Attack after a Quartermaster in one line
			switch {
			case gainsToHand[by], gainedToHand[name] && !gainsOntoDeck[by] && !aside: // a Guardian an Armory gains goes onto the deck
				dest = ZoneHand
			case gainedOntoDeck[name] || gainsOntoDeck[by],
				by == "Invasion" && cn.Card != nil && cn.Card.Is("Action") && !cn.Card.Is("Loot"): // its Action card, not the Duchy or the Loot
				dest = ZoneDeck
			case aside:
				dest = ZoneSetAside
			}
		}
		r.put(p, dest, name, cn.N)
		if name != "" {
			r.pend[p] = append(r.pend[p], pending{name, dest, e.Level, true, source(e), e.Line})
		}
	}
}

// lose removes cards that leave the player (trashed, returned to a pile).
func (r *zoneReplay) lose(e Event, p string, cards []CardN, want []string) {
	r.move(e, p, cards, want, "")
}

func (r *zoneReplay) leavePlay(p, name string) {
	ps := r.inPlay[p]
	pick := -1
	for i := len(ps) - 1; i >= 0; i-- {
		if ps[i].name != name {
			continue
		}
		if pick < 0 {
			pick = i
		}
		// in Clean-up Scheme topdecks the Monkey that's done, not the one staying out,
		// and Herbalist the Treasure discarded: last turn's Astrolabe, not this turn's;
		// Way of the Frog topdecks the Lighthouse played with it, not the one staying out
		if r.cleaning == p && (ps[i].stays == 0 || ps[i].none) {
			pick = i
			break
		}
		// before Clean-up Bonfire trashes the Caravan Guard whose turn this is, not the one just played
		// (a Flagship still waiting stays: Way of the Horse returns the one it played); in it a
		// Tireless Stowaway played on another's turn is set aside, not the one played on this
		if ps[i].turn < r.turn && ps[i].stays <= 0 && ps[i].holds == nil && !permanent[ps[i].name] && !(ps[i].name == "Prince" && ps[i].aside) && !(untilTriggered[ps[i].name] && !ps[i].fired) {
			pick = i
			break
		}
	}
	if pick >= 0 {
		r.inPlay[p] = append(ps[:pick], ps[pick+1:]...)
	}
}

// moveAll moves a whole zone, e.g. the discard pile into the deck when shuffling.
func (r *zoneReplay) moveAll(p, from, to string) {
	src := r.z(p)[from]
	if src == nil {
		return
	}
	switch {
	case from == ZoneDeck: // the deck to the discard pile
		r.top[p], r.bottom[p] = nil, nil
	case to == ZoneDeck && from == ZoneDiscard: // a shuffle: what was on top is mixed in; Shadow cards put under it stay at the bottom
		r.top[p] = nil
	case to == ZoneDeck: // revealed cards put back on top, in an order the log doesn't give
		for n, k := range src.Known {
			for ; k > 0; k-- {
				r.top[p] = append([]string{n}, r.top[p]...)
			}
		}
		for k := src.Unknown; k > 0; k-- {
			r.top[p] = append([]string{""}, r.top[p]...)
		}
	}
	dst := r.z(p).get(to)
	for n, k := range src.Known {
		dst.Known[n] += k
	}
	dst.Unknown += src.Unknown
	r.z(p)[from] = &Cards{Known: Deck{}}
}

// leftAside: the card a gain set aside has gone (a Watchtower trashed it, a
// Trail played itself), so the Blockade that set it aside holds nothing.
func (r *zoneReplay) leftAside(i int) {
	e := r.events[i]
	for j := i - 1; j >= 0; j-- {
		if g := r.events[j]; g.Template == "GAIN_SETTING_ASIDE" && g.Player == e.Player && g.Level < e.Level {
			r.evidence["set aside:"+r.cause(g)]--
			return
		}
	}
}

// before is the line before line i at its level, past the lines nested under that one, or -1.
func (r *zoneReplay) before(i int) int {
	k := i - 1
	for k >= 0 && r.events[k].Level > r.events[i].Level {
		k--
	}
	return k
}

// after is the line after line i at its level or above, past the lines nested under it, or len(r.events).
func (r *zoneReplay) after(i int) int {
	k := i + 1
	for k < len(r.events) && r.events[k].Level > r.events[i].Level {
		k++
	}
	return k
}

// parent is the template of the line event i is nested under, also one without a card ("ends their buy phase").
func (r *zoneReplay) parent(i int) string {
	for k := i - 1; k >= 0; k-- {
		if r.events[k].Level < r.events[i].Level {
			return r.events[k].Template
		}
	}
	return ""
}

// overpaid reports whether line i is under the gain of a card bought with
// overpay: "buys a Herald.", "loses 1 Coin.", "gains a Herald."
func (r *zoneReplay) overpaid(i int) bool {
	for k := i - 1; k >= 0; k-- {
		if r.events[k].Level >= r.events[i].Level {
			continue
		}
		if r.events[k].Template != "GAIN" {
			return false
		}
		for j := k - 1; j >= 0; j-- {
			if b := r.events[j]; b.Level <= r.events[k].Level {
				return b.Template == "BUY" && b.Player == r.events[k].Player && b.Args[1].Text == r.events[k].Args[1].Text
			}
		}
		return false
	}
	return false
}

// note records the parts of a turn that decide whether a Duration stays.
func (r *zoneReplay) note(kind, card string) {
	if card != "" {
		r.evidence[kind+":"+card]++
	}
}

// expire drops pending cards whose line has ended, and puts revealed cards
// nothing took back onto the deck.
func (r *zoneReplay) expire(e Event) {
	for _, m := range []map[string][]pending{r.pend, r.received, r.reacted, r.shared} {
		for p, ps := range m {
			keep := ps[:0]
			for _, x := range ps {
				if e.Level > x.level || (e.Level == x.level && x.level > 0) {
					keep = append(keep, x)
				}
			}
			m[p] = keep
		}
	}
	// a reaction's effects end with another player's line at its level: the
	// Curse a Torturer gives after a Beggar reacted goes to hand
	for p, ps := range r.reacted {
		keep := ps[:0]
		for _, x := range ps {
			if p == e.Player || e.Player == "" || x.level != e.Level {
				keep = append(keep, x)
			}
		}
		r.reacted[p] = keep
	}
	for p, zs := range r.zones {
		rev := zs[ZoneRevealed]
		if rev == nil || rev.Size() == 0 {
			continue
		}
		used := false
		for _, x := range r.pend[p] {
			if x.zone == ZoneRevealed {
				used = true
			}
		}
		replay := e.Template == "PLAY_WITH" && playsInPlace[strings.TrimSpace(e.Args[2].Text)] // a Flagship replays the Herald that revealed it
		if !used || e.Template == "NEW_TURN" || replay {
			r.moveAll(p, ZoneRevealed, ZoneDeck)
		}
	}
}

// othersPlay: another player has the card in play.
func (r *zoneReplay) othersPlay(p, name string) bool {
	for q, z := range r.zones {
		if c := z[ZonePlay]; q != p && c != nil && c.Known[name] > 0 {
			return true
		}
	}
	return false
}

// cleanUp discards the turn player's hand and the cards in play that don't stay.
func (r *zoneReplay) cleanUp(p string, i int) {
	// a Taskmaster stays for the replay its owner's next turn shows: Bridge can make a $5 card cost $4
	if n, ok := r.taskmasterReplays(p, i); ok {
		for _, x := range r.inPlay[p] {
			if x.name == "Taskmaster" {
				x.fired, x.spent = n > 0, false
				n--
			}
		}
	}
	for q := range r.zones {
		if r.z(q)[ZoneRevealed] != nil {
			r.moveAll(q, ZoneRevealed, ZoneDeck)
		}
	}
	r.moveAll(p, ZoneHand, ZoneDiscard)
	for n, k := range r.kept[p] { // "keeps an Estate in hand"
		for ; k > 0 && r.takeOne(p, ZoneDiscard, n, false); k-- {
			r.put(p, ZoneHand, n, 1)
		}
	}
	delete(r.kept, p)
	for q, ps := range r.inPlay {
		if q == p && r.evidence["bought:Journey"] > 0 {
			for _, x := range ps { // a Taskmaster decides now all the same: the extra turn's gains are another turn's
				if x.name == "Taskmaster" {
					x.spent = !x.decideStays(r, p, q)
				}
			}
			continue // they leave in the extra turn's Clean-up
		}
		var keep []*played
		for _, x := range ps {
			if !x.decideStays(r, p, q) {
				continue
			}
			keep = append(keep, x)
		}
		// a Throne Room stays as long as the Duration it played, or the Throne Room
		out := keep
		for changed := true; changed; {
			changed = false
			var next []*played
			for _, x := range out {
				if x.holds != nil && !containsPlayed(out, x.holds) {
					changed = true
					continue
				}
				next = append(next, x)
			}
			out = next
		}
		for _, x := range ps {
			if !containsPlayed(out, x) {
				if !r.takeOne(q, ZonePlay, x.name, true) {
					continue // it left play by a line of its own
				}
				to := ZoneDiscard
				if x.back {
					to = ZoneSetAside
				}
				r.put(q, to, x.name, 1)
			}
		}
		r.inPlay[q] = out
	}
	r.evidence = map[string]int{}
}

// taskmasterReplays counts the Taskmasters replayed at the start of p's next turn after line i:
// "starts their turn.", "gets +1 Action. (Taskmaster)"; false when the log ends first.
func (r *zoneReplay) taskmasterReplays(p string, i int) (int, bool) {
	for k := i; k < len(r.events); k++ {
		if e := r.events[k]; e.Template != "NEW_TURN" || e.Player != p {
			continue
		}
		n := 0
		for k++; k < len(r.events) && (r.events[k].Level > 0 || r.events[k].Template == "STARTS_TURN"); k++ {
			if e := r.events[k]; e.Template == "GETS_ACTION_FROM" && e.Player == p {
				for _, a := range e.Args {
					if len(a.Cards) == 1 && a.Cards[0].Card != nil && a.Cards[0].Card.Name == "Taskmaster" {
						n++
					}
				}
			}
		}
		return n, true
	}
	return 0, false
}

func containsPlayed(ps []*played, x *played) bool {
	for _, y := range ps {
		if y == x {
			return true
		}
	}
	return false
}

// decideStays says whether a card in play stays through p's Clean-up.
func (x *played) decideStays(r *zoneReplay, p, owner string) bool {
	if x.none {
		return false
	}
	if x.inherits != nil { // decided as the card it inherited, or the one an Overlord played
		y := *x
		y.name, y.card, y.inherits = x.inherits.Name, x.inherits, nil
		ok := y.decideStays(r, p, owner)
		x.stays = y.stays
		return ok
	}
	if permanent[x.name] || x.name == "Prince" && x.aside { // a Prince stays with the card it set aside
		return true
	}
	if x.holds != nil {
		return true // decided by the card it holds
	}
	if untilTriggered[x.name] {
		return !x.fired
	}
	if owner != p {
		// another player's card: a Duration played on this turn waits for its owner's turn,
		// but a Village Green played now (reacting to a discard) leaves with this turn
		if x.turn == r.turn && leavesIfNow[x.name] != "" && (x.nows > 0 || r.evidence["now:"+x.name] > 0) {
			if x.nows == 0 {
				r.evidence["now:"+x.name]--
			}
			return false
		}
		return x.turn < r.turn || (x.card != nil && x.card.Is("Duration"))
	}
	if x.holds != nil {
		return true // decided by the card it holds
	}
	if x.name == "Taskmaster" { // it stays for each turn it saw a card costing exactly $5 gained
		ok := x.fired && !x.spent
		x.fired = false
		return ok
	}
	if x.stays < 0 { // played on a turn whose Clean-up hasn't come yet
		if x.turn != r.turn { // a Duration played on another player's turn (Caravan Guard) has done its work
			return false
		}
		if x.card == nil || !x.card.Is("Duration") {
			return false
		}
		// each copy uses up one line of evidence
		use := func(kind string) bool {
			if r.evidence[kind+":"+x.name] > 0 {
				r.evidence[kind+":"+x.name]--
				return true
			}
			return false
		}
		// a Village Green that Mastermind plays three times stays unless all three were now
		allNow := func(n int) bool {
			k := min(n, r.evidence["now:"+x.name])
			r.evidence["now:"+x.name] -= k
			return k == n
		}
		switch {
		case staysIfSetAside[x.name] && !x.aside && !use("set aside"):
			return false
		case staysIfDiscards[x.name] && !x.aside && !use("discard"):
			return false
		case x.name == "Garrison" && !use("token"): // it stays with tokens on it
			return false
		case x.name == "Voyage" && !use("extra turn"): // only for the extra turn it gave
			return false
		case leavesIfNow[x.name] != "" && (x.nows > x.agains || allNow(1+x.agains-x.nows)):
			return false
		case x.name == "Outpost" && r.evidence["extra turn"]+r.evidence["stays:Outpost"] > 0:
			return false // it only works as the first Outpost of a turn after another player's
		}
		if x.name == "Outpost" {
			r.evidence["stays:Outpost"]++
		}
		x.stays = 0
		switch x.name {
		case "Archive", "Crypt":
			x.stays = max(0, x.nAside-1) // a turn of putting a card into hand for each card it set aside
		}
		return true
	}
	if x.stays > 0 {
		x.stays--
		return true
	}
	return false
}

// step applies one event.
func (r *zoneReplay) step(i int, e Event) {
	if p := r.bidingDue; p != "" && e.Template != "NEW_TURN" {
		for _, c := range r.biding[p] {
			if r.takeOne(p, ZoneSetAside, c, true) {
				r.put(p, ZoneHand, r.took, 1)
			}
		}
		delete(r.biding, p)
		r.bidingDue = ""
	}
	defer r.owned.Apply(e)
	r.line = e.Line
	r.latest[e.Level] = e.Line
	r.deckPos = ""
	for k := len(r.galleys) - 1; k >= 0 && (e.Level <= r.galleys[k].level || e.Template == "NEW_TURN" || r.cleanup[i]); k-- {
		r.setAsideByGalley(r.galleys[k]) // the inner one first: a Courier played by one plays another from the discard pile
		r.galleys = r.galleys[:k]
	}
	switch e.Template {
	case "TOPDECK", "TOPDECK_WITH", "BACK_ON_DECK", "GAIN_ON_DRAWPILE":
		r.deckPos = "top"
	case "BOTTOMDECKS", "PUT_ON_BOTTOM_OF":
		r.deckPos = "bottom"
	}
	if r.cleanup[i] {
		r.cleanUp(r.turnOf, i)
	}
	r.expire(e)
	r.fire(e)
	if e.Template == "NEW_TURN" {
		if e.Extra != "" {
			r.evidence["extra turn"]++
		}
		for p, cs := range r.possTrash { // back in the discard pile, unlogged
			for _, c := range cs {
				if r.takeOne(p, ZoneSetAside, c, c == "") {
					r.put(p, ZoneDiscard, r.took, 1)
				}
			}
		}
		r.possTrash = nil
		r.turn++
		r.turnOf = e.Player
		r.bidingDue = e.Player
		r.pend = map[string][]pending{}
		r.received = map[string][]pending{}
		r.shared = map[string][]pending{}
		r.reacted = map[string][]pending{}
		r.playAt = map[int]*played{}
		r.cleaning = ""
		return
	}
	p := e.Player
	if e.Template == "PLAY_ENCHANTED" || e.Template == "PLAY_ENLIGHTENED" { // "plays a Souk.", "Souk is enchanted by Enchantress": the Souk does nothing, nor Enlightenment's Cage
		if x := r.playAt[e.Level-2]; e.Level >= 2 && x != nil && x.name == strings.TrimSpace(e.Args[0].Text) {
			x.none = true
		}
	}
	if cs := e.Args[1].Cards; e.Template == "ACQUIRES_TRAIT" && len(cs) > 0 && cs[0].Card != nil {
		pile := pileName(e.Args[0].Text)
		if g, ok := pileGroups[pile+" Pile"]; ok { // "the Augur pile"
			pile = g
		}
		r.traits[pile] = cs[0].Card.Name
	}
	if p == "" {
		return
	}
	for q, ps := range r.pend {
		// another player's line ends a chain at its level: what a Diplomat
		// discarded isn't the Copper the Clerk then has its victim topdeck
		if q == p {
			continue
		}
		keep := ps[:0]
		for _, x := range ps {
			if x.level < e.Level {
				keep = append(keep, x)
			}
		}
		r.pend[q] = keep
	}
	cards := e.Args[1].Cards
	arg2 := ""
	if cs := e.Args[2].Cards; len(cs) > 0 && cs[0].Card != nil {
		arg2 = cs[0].Card.Name
	}
	self := func(want ...string) []string { // cards that move themselves come from play first
		if gains[e.CauseOf] { // not one just gained: an Inn shuffles itself in from the discard pile
			return want
		}
		if len(cards) > 1 { // a Chapel "trashes a Lighthouse and a Chapel" from hand
			return want
		}
		way := "" // a Way's "this" is the card played with it: Way of the Turtle "sets a Horse aside" from play
		if x := r.playAt[e.Level-1]; e.CauseOf == "PLAY_USING_WAY" && x != nil {
			way = x.name
		}
		treasure := false // a Landing Party topdecks itself when a Treasure is played: "plays a Silver.", "topdecks a Landing Party."
		if c := r.cardOf(p, e.Cause); c != nil && strings.HasPrefix(e.CauseOf, "PLAY") {
			treasure = c.Is("Treasure")
		}
		for _, cn := range cards {
			if cn.Card != nil && (cn.Card.Name == e.Cause || cn.Card.Name == arg2 || cn.Card.Name == way || cn.Card.Name == "Landing Party" && treasure && e.Template == "TOPDECK") {
				return append([]string{ZonePlay}, want...)
			}
		}
		return want
	}
	if c := cards; e.Template == "DISCARD" && len(c) == 1 && c[0].Card != nil && (c[0].Card.Is("Boon") || c[0].Card.Is("Hex")) {
		r.discardReceived(c[0].Card, e.Level) // its lines are done: a Changeling exchanged after Greed isn't Greed's
	}
	cause := r.cause(e)
	switch underPlay := strings.HasPrefix(e.CauseOf, "PLAY") || e.CauseOf == "REACTS_PLAYING"; { // not a Coin of the Realm called after a Village Green it discarded
	case underPlay && leavesIfNow[cause] == e.Template,
		underPlay && cause == "Village Green" && e.Template == "DRAW" && !(i+1 < len(r.events) && r.events[i+1].Template == "GETS_ACTIONS" && r.events[i+1].Level == e.Level): // after a Snowy Village it gets no +Actions
		if x := r.playAt[e.Level-1]; x != nil && x.name == cause {
			x.nows++ // this copy's: a Gondola played for later beside one played now stays
		} else {
			r.note("now", cause)
		}
	}
	if n := e.Args[1].Number; e.Template == "GARRISON_ADDS_TOKEN" && n != nil && *n == 1 { // a Garrison's first token
		r.note("token", "Garrison")
	}
	if e.Template == "VOYAGE_SUCCESS" {
		r.note("extra turn", "Voyage")
	}
	fromTrash := backFromTrashed(e, cards)

	switch t := e.Template; {
	case fromTrash: // Fortress back to hand, Lich to the discard pile, Trail into play
		to := backFromTrashZone[t]
		r.gain(e, p, cards, to)
		if to == ZonePlay {
			r.inPlay[p] = append(r.inPlay[p], &played{name: cards[0].Card.Name, card: cards[0].Card, turn: r.turn, stays: -1})
		}

	case t == "REACTS_WITH" && len(cards) == 1 && cards[0].Card != nil:
		r.reacted[p] = append(r.reacted[p], pending{cards[0].Card.Name, "", e.Level, false, source(e), e.Line})
	case (t == "RECEIVES" || t == "EXCHANGE_RECEIVE" || t == "TAKE") && len(cards) == 1 && cards[0].Card != nil && !cards[0].Card.InDeck():
		if n := i + 1; t == "TAKE" && n < len(r.events) && r.events[n].Player != p && r.events[n].Level == e.Level && strings.Contains(r.events[n].Template, "RECEIVE") {
			break // a Hex another player receives: its lines are theirs
		}
		r.received[p] = append(r.received[p], pending{cards[0].Card.Name, "", e.Level, false, source(e), e.Line})

	// cards that become the player's
	case t == "STARTS_WITH":
		r.gain(e, p, cards, ZoneDiscard) // shuffled into the deck by the first shuffle
	case t == "GAIN_ON_DRAWPILE":
		r.gain(e, p, cards, ZoneDeck)
	case t == "GAIN_TO_HAND":
		r.gain(e, p, cards, ZoneHand)
	case t == "GAIN_SETTING_ASIDE" && len(cards) == 1 && cards[0].Card != nil && gainedOntoDeck[cards[0].Card.Name]:
		r.gain(e, p, cards, ZoneDeck) // a Nomad Camp moves itself from set aside, unlogged
	case t == "GAIN_SETTING_ASIDE", t == "INHERITS":
		r.gain(e, p, cards, ZoneSetAside)
		r.note("set aside", cause)
	case t == "GAIN" && p != r.turnOf && e.Cause == "Province" && gains[e.CauseOf] && len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Name == "Gold" &&
		r.before(i) >= 0 && strings.HasPrefix(r.events[r.before(i)].Template, "TRASH") && r.events[r.before(i)].Player == p:
		r.gain(e, p, cards, ZoneDeck) // another player's Province: "trashes a Fool's Gold.", ("gets 1 VP from Tomb."), "gains a Gold." onto the deck
	case gains[t]:
		r.gain(e, p, cards, ZoneDiscard)
	case exilesFromSupply(e):
		r.gain(e, p, cards, ZoneExile)

	// cards that leave the player
	case strings.HasPrefix(t, "TRASH"):
		if trashesSupply(e) {
			break
		}
		want := self(ZoneHand, ZonePlay, ZoneDiscard)
		reacted := i > 0 && r.events[i-1].Template == "REACTS_WITH"
		fromHand := (e.CauseOf == "BUY_AND_GAIN" || e.CauseOf == "GAIN" || e.CauseOf == "GAIN_WITH" || e.CauseOf == "GAIN_SETTING_ASIDE") && cause != "Mint" && !reacted
		sirenAside := false // a Siren a Blockade set aside trashes itself: the Blockade has nothing left to do
		if fromHand {
			want = []string{ZoneHand} // Plan's Trashing token, a Hovel: not the card just gained, unless a Watchtower reacts
			if len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Name == "Siren" && cause == "Siren" {
				// a Siren trashes itself unless an Action is trashed from hand: the one just gained, where it went
				for k := len(r.pend[p]) - 1; k >= 0; k-- {
					if x := r.pend[p][k]; x.gained && x.card == "Siren" && x.level == e.Level-1 {
						want = []string{x.zone, ZoneHand}
						sirenAside = x.zone == ZoneSetAside
						break
					}
				}
				if n := i + 1; n < len(r.events) && r.events[n].Template == "TOPDECK" && r.events[n].Level == e.Level-1 && r.events[n].Args[1].Text == "a Siren" {
					want, sirenAside = []string{ZoneHand}, false // Replace topdecks the gained one: a Siren from hand was trashed
				}
			}
		}
		if cause == "Jack of All Trades" && strings.HasPrefix(e.CauseOf, "PLAY") || fromHandOnly[cause] && !gains[e.CauseOf] { // not a Watchtower trashing the Jack just bought
			want = []string{ZoneHand} // not the card it just discarded from the deck, nor the Amulet in play
		}
		if cause == "Bonfire" {
			want = []string{ZonePlay} // Coppers in play, not those in hand
		}
		if cause == "Monastery" && len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Name == "Copper" {
			// its trashing from hand, then its Coppers in play: one line of Coppers is most likely those played
			switch f := r.events[i-1]; {
			case f.Template == "TRASH" && f.Player == p && f.Level == e.Level:
				want = []string{ZonePlay}
			case i+1 < len(r.events) && r.events[i+1].Template == "TRASH" && r.events[i+1].Player == p && r.events[i+1].Level == e.Level:
				want = []string{ZoneHand}
			default:
				want = []string{ZonePlay, ZoneHand}
			}
		}
		if r.parent(i) == "BEGINS_CLEANUP_PHASE" {
			want = []string{ZonePlay, ZoneHand} // Improve's Action it would discard from play (a Raze may have trashed another)
		}
		if cause == "Hermit" && strings.HasPrefix(e.CauseOf, "PLAY") {
			want = []string{ZoneDiscard, ZoneHand} // never itself: the Hermit it looked at in the discard pile
		}
		if topTrashers[cause] && (!gains[e.CauseOf] && source(e) == cause || cause != e.Cause) { // a Swindler played or Locusts received, not one a Watchtower trashes as it's gained, nor Sewers after it
			want = []string{ZoneDeck}
		}
		if n := r.after(i); len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Name == "Urchin" && n < len(r.events) && // past Tomb's VP
			r.events[n].Template == "GAIN" && r.events[n].Player == p && r.events[n].Level == e.Level && r.events[n].Args[1].Text == "a Mercenary" {
			want = []string{ZonePlay} // an Urchin in play, trashed for a Mercenary as another Attack is played
			r.reacted[p] = append(r.reacted[p], pending{card: "Urchin", level: e.Level})
		}
		itself := false // the gained card, not what a gained Souk trashes from hand
		for _, cn := range cards {
			itself = itself || cn.Card == nil || cn.Card.Name == e.Cause
		}
		if strings.Contains(e.CauseOf, "GAIN") && itself && !fromHand { // Plan's token trashes a Magpie from hand as a Magpie is gained
			want = append(want, ZoneDeck, ZoneSetAside) // Watchtower trashing what Develop gained onto the deck, or Blockade set aside
		}
		if strings.HasSuffix(t, "_WITH") && source(e) == "Secluded Shrine" {
			want = []string{ZoneHand} // not the Silver a Beggar just gained onto the deck
		}
		if c := cards[0].Card; len(cards) == 1 && c != nil && trashedFromPlayOnGain[c.Name] && gains[e.CauseOf] && !reacted && // not a Watchtower's
			(!strings.HasSuffix(t, "_WITH") || source(e) == c.Name) && // not a Cage a Secluded Shrine trashes from hand
			(c.Name != "Search" || r.after(i) < len(r.events) && r.events[r.after(i)].Template == "DURATION_STOPS_WORKING") { // nor a Search a Siren does
			want = []string{ZonePlay, ZoneHand} // also under "gains an Estate. (Shaman)", not the Search just gained
		}
		if e.Possessed {
			r.move(e, p, cards, want, ZoneSetAside)
			if r.possTrash == nil {
				r.possTrash = map[string][]string{}
			}
			for _, cn := range cards {
				for range cn.N {
					name := ""
					if cn.Card != nil {
						name = cn.Card.Name
					}
					r.possTrash[p] = append(r.possTrash[p], name)
				}
			}
			break
		}
		r.lose(e, p, cards, want)
		if e.CauseOf == "GAIN_SETTING_ASIDE" && (reacted || sirenAside) { // a Watchtower trashed what a Blockade set aside, not Plan's token a card from hand
			r.leftAside(i)
		}
	case t == "PUT_ON_BOTTOM_OF" && !strings.EqualFold(strings.TrimSpace(e.Args[2].Text), "Draw Pile") && !toSupplyPile(e.Args[2].Text):
		// the Black Market deck: not the player's card
	case t == "RETURN_TO" && r.parent(i) == "BEGINS_CLEANUP_PHASE" && len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Name == "Encampment":
		// the Encampment it set aside, not one in hand; a Reckless card returns from play
		r.lose(e, p, cards, []string{ZoneSetAside, ZonePlay, ZoneHand, ZoneDiscard, ZoneDeck})
	case t == "RETURN_TO" && fromHandOnly[cause]: // Swap returns a card from hand, not the Duration in play
		r.lose(e, p, cards, []string{ZoneHand, ZonePlay, ZoneSetAside, ZoneDiscard, ZoneDeck})
	case t == "RETURN_TO", t == "PUT_ON_BOTTOM_OF" && toSupplyPile(e.Args[2].Text), t == "EXCHANGE_RETURN", t == "RETURN":
		r.lose(e, p, cards, []string{ZonePlay, ZoneHand, ZoneSetAside, ZoneDiscard, ZoneDeck})
		if e.CauseOf == "GAIN_SETTING_ASIDE" { // a Trader exchanges what a Blockade set aside
			r.leftAside(i)
		}
	case t == "PASS":
		r.lose(e, p, cards, []string{ZoneHand})
		if q := e.Args[2].Player; q != "" {
			r.gain(e, q, cards, ZoneHand)
		}

	// moves between the player's zones
	case t == "DRAW", t == "DRAW_FROM", t == "DRAWS_FROM", t == "WISH_CORRECT": // Wishing Well puts the card it finds in hand
		r.move(e, p, cards, []string{ZoneDeck}, ZoneHand)
	case t == "PLAY_AGAIN", t == "PLAY_THIRD", t == "PLAY_TREASURES_AGAIN_FOR", t == "PLAY_TREASURES_THIRD_FOR":
		holder := e.Cause
		if i > 0 {
			if c := r.events[i-1]; c.Template == "CALL" && c.Player == p && c.Level == e.Level && len(c.Args[1].Cards) == 1 && c.Args[1].Cards[0].Card != nil {
				holder = c.Args[1].Cards[0].Card.Name // "calls a Royal Carriage.", "plays a Swamp Hag again."
			}
		}
		r.replay(e, p, cards, holder)
	case t == "PLAY" && (cause == "Estate" || cause == "Overlord") && strings.HasPrefix(e.CauseOf, "PLAY"): // an Estate playing the card it inherited, not a Weaver discarded with one
		if x := r.playAt[e.Level-1]; x != nil && x.name == cause && len(cards) == 1 {
			// an Overlord played twice stays as long as the longer: a Crowned one that played
			// an Enchantress, then a Bandit, stays like the Enchantress, and an Archive outlasts it
			if old := x.inherits; old == nil || !old.Is("Duration") || cards[0].Card.Is("Duration") && old.Name != "Archive" {
				x.inherits = cards[0].Card
			}
			r.playAt[e.Level] = x
			if x.inherits.Is("Duration") && playsTwice[x.by] {
				r.holdInherited(p, x)
			} // what the Archive an Overlord played sets aside keeps the Overlord in play
		}
	case t == "PLAY_NULLIFIED": // "plays Copper (nullified by Highwayman)" describes the play above
	case t == "PLAY_WITH" && (arg2 == "Mastermind" && r.replayed(i, 3) || arg2 == "Ghost" && r.replayed(i, 2)): // they log each play
		r.replay(e, p, cards, arg2) // a Mastermind that played a Duration stays in play with it
	case t == "PLAY_WITH" && playsInPlace[arg2]:
		r.playAt[e.Level] = nil // its lines are the replayed card's: "plays a Grotto. (Flagship)", "sets a Grotto aside with Grotto."
		for k := len(r.inPlay[p]) - 1; k >= 0 && len(cards) == 1 && cards[0].Card != nil; k-- {
			if r.inPlay[p][k].name == cards[0].Card.Name {
				r.playAt[e.Level] = r.inPlay[p][k]
				r.inPlay[p][k].agains++ // a Village Green replayed for later stays
				break
			}
		}
	case t == "PLAY_WITH" && (arg2 == "Mastermind" || (arg2 == "Royal Galley" || arg2 == "") && e.Cause != ""): // at the start of a turn they come from set aside
		r.play(e, p, cards, []string{ZoneHand, ZoneSetAside, ZoneDiscard})
	case t == "PLAY_WITH" && gains[e.CauseOf]: // Sailor plays what a Blockade set aside, or Develop put onto the deck
		r.play(e, p, cards, []string{ZoneSetAside, ZoneDeck, ZoneDiscard, ZoneHand})
		if e.CauseOf == "GAIN_SETTING_ASIDE" {
			r.leftAside(i)
		}
	case t == "PLAY_WITH":
		r.play(e, p, cards, []string{ZoneSetAside, ZoneHand, ZoneDiscard})
	case t == "PLAY" && cause == "Gamble" && r.playedOnDiscard(i): // played again: Gamble plays the Weaver it discarded, which played itself
		r.replay(e, p, cards, cause)
	case t == "PLAY" && playsFromElsewhere[cause] && !gains[e.CauseOf]: // a Falconer reacting to a gained Overlord is played from hand
	case t == "PLAY" && i > 0 && r.events[i-1].Template == "REACTS_PLAYING" && r.events[i-1].Player == p && r.events[i-1].Level == e.Level:
		// "reacts with a Guard Dog, playing it." and then "plays a Guard Dog." are one play
	case t == "REACTS_PLAYING" && gains[e.CauseOf] && len(cards) == 1 && cards[0].Card != nil && playsFromHandOnGain[cards[0].Card.Name]:
		// a Sheepdog plays from hand, not the Sheepdog just gained; a gained Trail plays itself
		r.play(e, p, cards, []string{ZoneHand})
	case t == "REACTS_PLAYING" && e.CauseOf == "GAIN_SETTING_ASIDE": // "gains a Trail setting it aside.", "reacts with a Trail, playing it."
		r.play(e, p, cards, []string{ZoneSetAside, ZoneHand, ZoneDiscard, ZoneDeck})
		r.leftAside(i)
	case t == "PLAY" && r.traitOf(cause) == "Inspiring" && strings.HasPrefix(e.CauseOf, "PLAY") && !r.movedAbove(i, cards):
		r.play(e, p, cards, []string{ZoneHand}) // an Action from hand, not the copy the Inspiring card topdecked (but a Vassal's own)
	case t == "PLAY" && len(cards) == 1 && cards[0].Card != nil && r.reactedWith(i, cards[0].Card.Name):
		r.play(e, p, cards, []string{ZoneHand}) // "reacts with a Stowaway.", "plays a Stowaway.": from hand, not the one just gained
	case t == "PLAY" && e.Cause == "Way of the Mouse" && e.CauseOf == "PLAY_USING_WAY":
		// the set-aside card, played where it is: the card that used the Way stays in play like it
		if x := r.playAt[e.Level-1]; x != nil && len(cards) == 1 && cards[0].Card != nil {
			x.none, x.inherits = false, cards[0].Card
			r.playAt[e.Level] = x
		}
	case t == "PLAY", t == "PLAY_TREASURES_FOR", t == "PLAY_USING_WAY", t == "PLAY_ENLIGHTENED", t == "PLAY_NULLIFIED", t == "REACTS_PLAYING":
		want := []string{ZoneHand, ZoneSetAside, ZoneDiscard, ZoneDeck}
		shadow := len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Is("Shadow")
		if shadow {
			want = []string{ZoneHand, ZoneDeck, ZoneSetAside, ZoneDiscard} // played from the deck as if in the hand, not one Rapid Expansion set aside
		}
		r.shadowPlay = shadow // not a take from the deck's top: a replay plays that very card
		r.play(e, p, cards, want)
		r.shadowPlay = false
		if x := r.playAt[e.Level]; t == "PLAY_USING_WAY" && x != nil && arg2 != "Way of the Chameleon" {
			x.none = true // a Duration played with a Way does nothing of its own; with Chameleon it follows its own text
		}
	case t == "DISCARD", t == "DISCARD_WITH", t == "DISCARD_REVEALED":
		want := self(ZoneHand)
		rest := []string{ZonePlay, ZoneSetAside, ZoneTavern} // Wine Merchant from the Tavern mat
		if p != r.turnOf {
			rest = []string{ZoneSetAside} // an attack's victim discards from hand, not the Guard Dog it just played
			if e.Level == 0 && e.Cause == "" {
				want, rest = []string{ZonePlay}, []string{ZoneHand} // the turn's Clean-up: the Mapmakers they played on it
			}
		}
		switch {
		case r.reactedWith(i, "Sleigh"): // "reacts with a Sleigh.", "discards a Sleigh.": from hand, whatever was gained
			want, rest = []string{ZoneHand}, nil
		case topDiscarders[cause] && len(cards) == 1 && cards[0].N == 1 && (arg2 == "" || arg2 == cause): // more is a Diplomat reacting to it; "(Beggar)" a Beggar reacting
			want, rest = []string{ZoneDeck}, nil // never the card in play: a Vassal that discards a Vassal
		case cause == "Library":
			want, rest = []string{ZoneSetAside}, nil // the Actions it skipped, not copies it drew
			if p == r.turnOf && r.othersPlay(p, "Frigate") {
				rest = []string{ZoneHand} // or down to 4 after it
			}
		case r.parent(i) == "ENDS_BUYPHASE":
			want, rest = []string{ZoneTavern}, nil // Wine Merchants, not a copy in hand
		case r.parent(i) == "BEGINS_BUYPHASE":
			want, rest = []string{ZoneHand}, nil // Arena's Action, not one an Archive set aside
		case r.parent(i) == "BEGINS_CLEANUP_PHASE":
			want = []string{ZoneHand} // a Friendly card, not one a Grotto set aside
		case cause == "Tactician", fromHandOnly[cause]:
			want, rest = []string{ZoneHand}, nil // the hand, with any other Tactician in it
		case p == r.turnOf && r.othersPlay(p, "Frigate") && strings.HasPrefix(e.CauseOf, "PLAY") && (i+1 == len(r.events) || r.events[i+1].Level < e.Level):
			want, rest = []string{ZoneHand}, nil // down to 4 after the Action, not the Action itself
			if cause == "Herald" {
				r.moveAll(p, ZoneRevealed, ZoneDeck) // nor what it revealed on the deck; a Survivors discards what it looked at
			}
		case e.Cause == "" && cause == "" && e.Level > 0 && (t == "DISCARD" || arg2 == "Prepare") && !r.shyDiscard(i): // at the start of a turn: Grotto and Prepare discard what they set aside (Tide Pools names itself), not a Boon Lost in the Woods gives
			want = []string{ZoneSetAside, ZoneHand}
			if n := 0; r.gang {
				for _, cn := range cards {
					n += cn.N
				}
				if h := r.z(p)[ZoneHand]; h != nil && h.Size()-n == 4 {
					want = []string{ZoneHand, ZoneSetAside} // down to 4, not the cards a Cage set aside
				}
			}
		}
		r.move(e, p, cards, append(want, rest...), ZoneDiscard)
		if n := 0; e.Cause == "Secret Cave" {
			for _, c := range cards {
				n += c.N
			}
			if n != 3 {
				break // a Frigate's discard after it, not its three cards
			}
		}
		if x := r.playAt[e.Level-1]; e.Level > 0 && x != nil && x.name == e.Cause && staysIfDiscards[x.name] {
			x.aside = true // this Secret Cave stays, not one a Ghost played that discarded nothing
			break
		}
		r.note("discard", e.Cause)
	case t == "DISCARD_FROM_EXILE":
		r.move(e, p, cards, []string{ZoneExile}, ZoneDiscard)
	case t == "PUT_IN_DISCARD":
		// Avoid and Order of Masons log the cards they keep out of a shuffle
		// before it; those are in the discard pile already
		var rest []CardN
		for _, cn := range cards {
			name := ""
			if cn.Card != nil {
				name = cn.Card.Name
			}
			k := 0
			for ; k < cn.N && r.takeOne(p, ZoneDiscard, name, name == ""); k++ {
				if r.avoid[p] == nil {
					r.avoid[p] = Deck{}
				}
				r.avoid[p][r.took]++
			}
			if k < cn.N {
				rest = append(rest, CardN{cn.Card, cn.N - k})
			}
		}
		r.move(e, p, rest, []string{ZoneSetAside, ZoneDeck, ZoneHand, ZonePlay}, ZoneDiscard)
	case t == "DECK_TO_DISCARD":
		r.moveAll(p, ZoneDeck, ZoneDiscard)
	case t == "SHUFFLES" && i+1 < len(r.events) && r.events[i+1].Template == "SHUFFLE_INTO_DECK" && r.events[i+1].Player == p:
		// "shuffles their deck." for "shuffles an Inn into their deck." (Inn, Famine, Donate): the discard pile stays
	case t == "SHUFFLES":
		// a shuffle for a draw means the deck had fewer cards than the draw
		if i+1 < len(r.events) {
			if next := r.events[i+1]; drawnT[next.Template] && next.Player == p {
				n := 0
				for _, cn := range next.Args[1].Cards {
					n += cn.N
				}
				// Shadow cards put under the deck just before go under the new one
				for j := i - 1; j >= 0 && r.events[j].Template == "BOTTOMDECKS" && r.events[j].Player == p; j-- {
					for _, cn := range r.events[j].Args[1].Cards {
						n += cn.N
					}
				}
				empty := r.z(p)[ZoneDiscard] == nil || r.z(p)[ZoneDiscard].Size() == 0 // Order of Masons kept every card out of it: the draw takes the deck
				if d := r.z(p)[ZoneDeck]; d != nil && d.Size() >= n && !(empty && d.Size() == n) {
					r.misses = append(r.misses, ZoneMiss{Line: e.Line, Player: p, Template: t, Cause: e.Cause, Want: "a deck smaller than the next draw", Found: ZoneDeck})
				}
			}
		}
		r.moveAll(p, ZoneDiscard, ZoneDeck)
		for n, k := range r.avoid[p] {
			r.put(p, ZoneDiscard, n, k)
		}
		delete(r.avoid, p)
		r.deckPos = "top"
		for _, n := range r.fated[p] {
			r.put(p, ZoneDeck, n, 1)
		}
		delete(r.fated, p)
	case t == "TOPDECK" && fatedTop(r.events, i):
		for _, cn := range cards {
			for k := 0; cn.Card != nil && k < cn.N; k++ {
				if r.takeOne(p, ZoneDiscard, cn.Card.Name, false) {
					r.fated[p] = append(r.fated[p], cn.Card.Name)
				}
			}
		}
	case t == "SET_ASIDE" && r.parent(i) == "BEGINS_CLEANUP_PHASE" && len(cards) == 1 && cards[0].Card != nil && r.traitOf(cards[0].Card.Name) == "Tireless":
		r.move(e, p, cards, []string{ZonePlay, ZoneHand}, ZoneSetAside) // discarded from play, aside until the turn ends
	case t == "TOPDECK" && e.Level == 0 && len(cards) == 1 && cards[0].Card != nil && r.traitOf(cards[0].Card.Name) == "Tireless":
		r.move(e, p, cards, []string{ZoneSetAside, ZonePlay, ZoneHand}, ZoneDeck) // after the Clean-up draw, also one played on another's turn
	case t == "BUY" && len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Name == "Journey":
		r.note("bought", "Journey") // this turn's Clean-up leaves the cards in play
	case t == "BEGINS_CLEANUP_PHASE":
		r.cleaning = p
	case (t == "LOOK_AT" || t == "TOPDECK") && cause == "Pearl Diver" && strings.HasPrefix(e.CauseOf, "PLAY"): // the bottom card of the deck, maybe put on top
	case t == "TOPDECK" && r.cleaning == p && len(cards) == 1 && cards[0].Card == nil:
		// "topdecks a card" in one's own Clean-up hides a card leaving play: Merchant
		// Camp, Treasury, or what Scheme chose; the next hand shows which
		for k := 0; k < cards[0].N; k++ {
			name := r.guessTopdeck(p)
			r.move(e, p, []CardN{{r.cardOf(p, name), 1}}, []string{ZonePlay, ZoneHand}, ZoneDeck)
		}
	case t == "TOPDECK" && r.cleaning == p && e.Cause == "": // a named one: Scheme's pick, a Walled Village, an Alchemist
		r.move(e, p, cards, []string{ZonePlay, ZoneHand}, ZoneDeck)
	case t == "TOPDECK" && cause == "Replace":
		// the card it gained, from the discard pile, not the Replace in play; no
		// line when the gain's effects lost it (a Footpad's shuffle, a Hostelry's discards)
		if len(cards) == 1 && cards[0].Card == nil { // "topdecks a card"
			for k := len(r.pend[p]) - 1; k >= 0; k-- {
				if x := r.pend[p][k]; x.gained && x.level == e.Level && x.zone == ZoneDiscard && r.takeOne(p, ZoneDiscard, x.card, false) {
					r.put(p, ZoneDeck, x.card, 1)
					return
				}
			}
		}
		r.move(e, p, cards, []string{ZoneDiscard, ZoneDeck}, ZoneDeck)
	case t == "SHUFFLE_INTO_DECK" && (cause == "Inn" && gains[e.CauseOf] || cause == "Annex"): // the discard pile's cards, not those in hand
		r.move(e, p, cards, []string{ZoneDiscard}, ZoneDeck)
	case t == "TOPDECK" && cause == "Transport": // "buys Transport.", "topdecks a Library." from Exile
		r.move(e, p, cards, []string{ZoneExile}, ZoneDeck)
	case t == "TOPDECK" && (e.CauseOf == "BUY_AND_GAIN" || r.overpaid(i)) && r.othersPlay(p, "Haunted Woods") && !(i > 0 && (r.events[i-1].Template == "REACTS_WITH" || r.afterSleigh(i-1))): // not the card a Sleigh topdecks
		want := []string{ZoneHand} // the hand, not the card just bought
		if h := r.z(p)[ZoneHand]; len(cards) == 1 && cards[0].Card != nil && (h == nil || h.Known[cards[0].Card.Name] == 0) {
			want = append(want, ZoneDiscard) // a Tiara's topdeck after it, of that card
		}
		r.move(e, p, cards, want, ZoneDeck)
	case t == "TOPDECK" && r.landingTop(i, p, cards): // from play: not one the Treasure gained (Hammer) or looked at (Orb)
		r.move(e, p, cards, []string{ZonePlay}, ZoneDeck)
	case t == "TOPDECK" && discardLookers[cause]: // Harbinger's pick from the discard pile, not the Harbinger in play
		r.move(e, p, cards, []string{ZoneDiscard}, ZoneDeck)
	case t == "PUT_ON_BOTTOM_OF" && cause == "" && len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Name == "Jewels":
		r.move(e, p, cards, []string{ZonePlay}, ZoneDeck) // the last turn's Jewels at the start of this one
	case t == "TOPDECK", t == "TOPDECK_WITH", t == "BACK_ON_DECK", t == "BOTTOMDECKS", t == "INSERT_IN_DECK", t == "SHUFFLE_INTO_DECK",
		t == "PUT_ON_BOTTOM_OF": // "on the bottom of Draw Pile"
		// Inn shuffles first
		want := self(ZoneHand, ZoneDiscard, ZonePlay, ZoneSetAside, ZoneDeck)
		if t == "TOPDECK" && topdecksFromHand[cause] && (strings.HasPrefix(e.CauseOf, "PLAY") || cause != e.Cause) { // not a Watchtower's; Haunting a gained Cursed Village gave
			want = []string{ZoneHand} // a Courtyard topdecks a Courtyard from hand, not itself
		}
		if t == "BOTTOMDECKS" && i+1 < len(r.events) && r.events[i+1].Template == "SHUFFLES" && r.events[i+1].Player == e.Player {
			want = []string{ZoneDiscard} // Shadow cards go under the deck as the discard pile is shuffled
		}
		r.move(e, p, cards, want, ZoneDeck)
		if e.CauseOf == "GAIN_SETTING_ASIDE" { // Watchtower topdecked what Blockade set aside
			r.leftAside(i)
		}
	case t == "PUT_IN_HAND_FROM":
		r.move(e, p, cards, []string{ZoneSetAside, ZoneNativeVillage, ZonePlay}, ZoneHand) // Blockade's gained card; Conjurer itself from play
	case t == "PUT_IN_HAND" && cause == "Native Village" && !gains[e.CauseOf]: // the mat's cards, not the Native Village just played; a Sleigh puts in hand the one gained
		r.move(e, p, cards, []string{ZoneNativeVillage}, ZoneHand)
	case t == "PUT_IN_HAND" && cause == "Mountain Village" && strings.HasPrefix(e.CauseOf, "PLAY"): // from the discard pile it looked at, even a Mountain Village
		r.move(e, p, cards, []string{ZoneDiscard}, ZoneHand)
	case t == "PUT_IN_HAND" && cause == "" && (r.parent(i) == "NEW_TURN" || r.parent(i) == "STARTS_TURN"): // Donate's deck and discard pile, not what an Archive set aside
		r.move(e, p, cards, []string{ZoneDeck, ZoneDiscard}, ZoneHand)
	case t == "PUT_IN_HAND":
		r.move(e, p, cards, []string{ZoneSetAside, ZoneNativeVillage, ZoneDiscard, ZoneDeck, ZonePlay}, ZoneHand)
		if e.CauseOf == "GAIN_SETTING_ASIDE" { // a Villa Blockade set aside puts itself into hand
			r.leftAside(i)
		}
	case t == "SET_ASIDE_WITH" && arg2 == "Native Village":
		r.move(e, p, cards, []string{ZoneDeck}, ZoneNativeVillage)
	case t == "SET_ASIDE" && setsAsideFromDeck[cause]:
		r.move(e, p, cards, []string{ZoneDeck}, ZoneSetAside)
		if x := r.playAt[e.Level-1]; e.Level > 0 && x != nil && (x.name == cause || x.inherits != nil && x.inherits.Name == cause) {
			x.aside = true // this Research: a Throne Room's that set cards aside, not the one before it that didn't
		} else {
			r.note("set aside", cause)
		}
	case t == "SET_ASIDE", t == "SET_ASIDE_WITH":
		want := []string{ZoneHand, ZonePlay, ZoneDeck, ZoneDiscard} // Faithful Hound from the discard pile
		switch {
		case t == "SET_ASIDE" && cause == "Farmhands" && gains[e.CauseOf]:
			want = []string{ZoneHand} // a gained Farmhands sets aside a card from hand, not itself
		case t == "SET_ASIDE":
			want = self(want...)
		case arg2 != "Trickster": // only Trickster takes from play: Haven sets aside from hand, not the Throne Room that played it, nor itself
			want = []string{ZoneHand, ZoneDeck, ZoneDiscard}
		}
		r.move(e, p, cards, want, ZoneSetAside)
		if arg2 == "Biding Time" {
			if r.biding == nil {
				r.biding = map[string][]string{}
			}
			for _, cn := range cards {
				name := ""
				if cn.Card != nil {
					name = cn.Card.Name
				}
				for range cn.N {
					r.biding[p] = append(r.biding[p], name)
				}
			}
		}
		holder := e.Cause
		if arg2 != "" {
			holder = arg2
		}
		if x := r.playAt[e.Level-1]; e.Level > 0 && x != nil && (x.name == holder || x.inherits != nil && x.inherits.Name == holder) {
			x.aside = true
			n := 0
			for _, c := range cards {
				n += c.N
			}
			x.nAside = max(x.nAside, n) // a Crown's two plays of one Archive each put their own cards into hand
		} else {
			r.note("set aside", holder)
		}
	case t == "EXILE":
		want := self(ZoneHand, ZonePlay)
		if gains[e.CauseOf] { // "gains an Upgrade.", "exiles an Upgrade.": Gatekeeper's Attack takes the card just gained
			want = []string{ZoneDiscard, ZoneHand, ZoneDeck}
		}
		r.move(e, p, cards, want, ZoneExile)
	case t == "PUT_ON_MAT":
		mat := ZoneTavern
		if strings.Contains(e.Args[2].Text, "Island") {
			mat = ZoneIsland
		}
		r.move(e, p, cards, self(ZoneHand, ZonePlay), mat)
	case t == "CALL":
		r.play(e, p, cards, []string{ZoneTavern})

	// looking at cards
	case t == "WISH_WRONG": // "wishes for Bandit but reveals Will-o'-Wisp.": it stays on the deck
		r.move(e, p, e.Args[2].Cards, []string{ZoneDeck}, ZoneRevealed)
	case t == "REVEAL", t == "LOOK_AT":
		switch {
		case t == "REVEAL" && len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Is("Loot") && i+1 < len(r.events) && gains[r.events[i+1].Template]:
			// "reveals a Hammer.", "gains a Hammer.": the top of the Loot pile
		case t == "REVEAL" && i > 0 && r.events[i-1].Template == "REACTS_WITH" && r.events[i-1].Player == e.Player && r.events[i-1].Level == e.Level:
			// "reacts with a Tunnel.", "reveals a Tunnel.": it shows itself where it is, not a Vagrant's top card when a Storeroom discarded both
		case t == "LOOK_AT" && r.afterSleigh(i): // "reacts with a Sleigh.", "discards a Sleigh.", and it looks at the discard pile
			r.mark(e, p, cards, ZoneDiscard)
		case t == "LOOK_AT" && cause == "Library": // Library draws the cards it looks at
			r.move(e, p, cards, []string{ZoneDeck}, ZoneHand)
		case discardLookers[cause] && !strings.HasPrefix(e.CauseOf, "PLAY"): // Herald's overpay, Inn when gained
			r.mark(e, p, cards, ZoneDiscard)
		case cause == "Chariot Race" && p != r.turnOf: // the player to the left reveals their top card; the player's own is the one it drew
			r.move(e, p, cards, []string{ZoneDeck}, ZoneRevealed)
		case deckLookers[cause], cause == "" && e.Level > 0: // at the start of a turn: Piazza, Forest Dwellers
			r.move(e, p, cards, []string{ZoneDeck}, ZoneRevealed)
		case discardLookers[cause]:
			r.mark(e, p, cards, ZoneDiscard)
		default:
			r.mark(e, p, cards, ZoneHand)
		}
	case t == "KEEPS_IN_HAND":
		for _, cn := range cards {
			if cn.Card != nil {
				if r.kept[p] == nil {
					r.kept[p] = Deck{}
				}
				r.kept[p][cn.Card.Name] += cn.N
			}
		}
	case t == "REVEAL_HAND":
		r.mark(e, p, cards, ZoneHand)
	}
}

// afterSleigh: the line comes right after a Sleigh's reaction at its level.
func (r *zoneReplay) afterSleigh(i int) bool {
	return i >= 2 && r.events[i-1].Template == "DISCARD" && r.events[i-1].Player == r.events[i].Player && r.reactedWith(i-1, "Sleigh")
}

// playedOnDiscard: the line plays the card the line one level under it just
// played: "discards a Weaver.", "plays a Weaver." (its reaction), "plays a Weaver." (Gamble).
func (r *zoneReplay) playedOnDiscard(i int) bool {
	e := r.events[i]
	for j := i - 1; j >= 0; j-- {
		if f := r.events[j]; f.Level <= e.Level+1 {
			return f.Level == e.Level+1 && f.Template == "PLAY" && f.Player == e.Player && f.Args[1].Text == e.Args[1].Text
		}
	}
	return false
}

// reactedWith: the line comes right after its player's reaction with the card, at its level.
func (r *zoneReplay) reactedWith(i int, name string) bool {
	if i < 1 {
		return false
	}
	e, x := r.events[i], r.events[i-1]
	cs := x.Args[1].Cards
	return x.Template == "REACTS_WITH" && x.Player == e.Player && x.Level == e.Level && len(cs) == 1 && cs[0].Card != nil && cs[0].Card.Name == name
}

// play puts cards into play and remembers what played them. A card the player
// doesn't own is played from somewhere else (the Supply, the trash) and stays there.
func (r *zoneReplay) play(e Event, p string, cards []CardN, want []string) {
	var mine []CardN
	for _, cn := range cards {
		if cn.Card == nil || r.owned[p][cn.Card.Name] > 0 {
			mine = append(mine, cn)
		}
	}
	n := len(r.inPlay[p])
	r.move(e, p, mine, want, ZonePlay)
	r.playAt[e.Level] = nil
	if len(r.inPlay[p]) == n+1 {
		r.playAt[e.Level] = r.inPlay[p][n]
		r.inPlay[p][n].by = e.Cause
		if e.Cause == "Royal Galley" && e.Template == "PLAY" && strings.HasPrefix(e.CauseOf, "PLAY") { // not a Stowaway reacting to its gain
			r.galleys = append(r.galleys, &galleyPlay{p: p, rg: r.playAt[e.Level-1], x: r.inPlay[p][n], level: e.Level})
		}
	}
	if strings.TrimSpace(e.Args[2].Text) == "Prince" {
		for _, x := range r.inPlay[p][n:] {
			x.back = true
		}
	}
}

// setAsideByGalley sets aside, without a line, the card a Royal Galley played, if it's still in play.
func (r *zoneReplay) setAsideByGalley(g *galleyPlay) {
	if !containsPlayed(r.inPlay[g.p], g.x) {
		return
	}
	r.inPlay[g.p] = slices.DeleteFunc(r.inPlay[g.p], func(y *played) bool { return y == g.x })
	if r.takeOne(g.p, ZonePlay, g.x.name, false) {
		r.put(g.p, ZoneSetAside, g.x.name, 1)
	}
	if g.rg != nil && g.rg.name == "Royal Galley" {
		g.rg.aside = true
	} else {
		r.note("set aside", "Royal Galley")
	}
}

// holdInherited: the Throne Room that played an Overlord that played a Duration,
// on either of its plays, stays in play with it, and so does what played that one.
func (r *zoneReplay) holdInherited(p string, x *played) {
	ps := r.inPlay[p]
	for _, h := range ps {
		if h.holds == x {
			return
		}
	}
	for i := len(ps) - 1; i >= 0; i-- {
		if h := ps[i]; (h.name == x.by || h.inherits != nil && h.inherits.Name == x.by) && h.holds == nil && h != x {
			h.holds = x
			if playsTwice[h.by] {
				r.holdInherited(p, h)
			}
			return
		}
	}
}

// replay notes a card played again: a Throne Room that plays a Duration twice
// stays in play as long as that Duration, and so does a Throne Room that plays
// that Throne Room.
func (r *zoneReplay) replay(e Event, p string, cards []CardN, holder string) {
	ps := r.inPlay[p]
	for _, cn := range cards {
		if cn.Card == nil {
			continue
		}
		var x *played
		for i := len(ps) - 1; i >= 0; i-- {
			if ps[i].name == cn.Card.Name && (x == nil || ps[i].by == holder && x.by != holder) {
				x = ps[i] // the one this holder played: not the Overlord its Overlord's Crown played
			}
		}
		if x != nil {
			x.again = true
			x.agains++
			x.none = false // played again with its own text: a Royal Carriage replays a Wharf played with Way of the Seal
			if x.card != nil && x.card.Is("Duration") {
				x.inherits = nil // nor as the Herbalist Way of the Mouse played: a Scepter replays the Amulet
			}
		}
		if x == nil || !cn.Card.Is("Duration") && x.holds == nil {
			continue
		}
		for i := len(ps) - 1; x != nil && i >= 0; i-- {
			if h := ps[i]; (h.name == holder || h.inherits != nil && h.inherits.Name == holder) && h.holds == nil && h != x { // an Overlord's Crown
				h.holds = x
				break
			}
		}
		// and the Throne Room that played that Throne Room, though it played it before the Duration
		for k := e.Level - 1; k > 0; k-- {
			in, out := r.playAt[k], r.playAt[k-1]
			if in != nil && in == out {
				continue // an Overlord is at its own level and at the one of the card it plays
			}
			if in == nil || out == nil || in.holds == nil || !in.again || out.holds != nil || in.by != out.name && (out.inherits == nil || out.inherits.Name != in.by) { // an Overlord's Crown
				break
			}
			out.holds = in
		}
	}
}

// mark makes revealed cards pending where they are, so that the next lines take
// those copies (a revealed Estate topdecked from hand).
func (r *zoneReplay) mark(e Event, p string, cards []CardN, prefer string) {
	for _, cn := range cards {
		if cn.Card == nil || !cn.Card.InDeck() {
			continue
		}
		name := cn.Card.Name
		zone := ""
		for _, z := range append([]string{prefer}, zoneOrder...) {
			if c := r.z(p)[z]; c != nil && c.Known[name] > 0 {
				zone = z
				break
			}
		}
		if zone != "" {
			r.pend[p] = append(r.pend[p], pending{name, zone, e.Level, false, source(e), e.Line})
		}
	}
}

func (r *zoneReplay) snapshot() map[string]Zones {
	out := map[string]Zones{}
	for p, z := range r.zones {
		out[p] = z.copy()
	}
	return out
}

// ZoneMisses replays the zones and returns every card that wasn't where its line
// implies. A complete, correctly read log of a seat whose cards are all named
// has none.
func (g *Game) ZoneMisses() []ZoneMiss {
	r := newZoneReplay(g)
	for i, e := range g.Events {
		r.step(i, e)
	}
	sort.SliceStable(r.misses, func(i, j int) bool { return r.misses[i].Line < r.misses[j].Line })
	return r.misses
}

// DeckTake is a card leaving a player's draw pile: drawn, revealed, looked at,
// discarded or trashed from it. Card is "" when the log doesn't name it.
type DeckTake struct {
	Line int
	Card string
}

// DeckTakes lists, for each player, every card taken from their draw pile, in
// log order. A replay of the game draws them in this order. It also lists the
// named cards each line took from somewhere other than a hand: Vagrant's
// "reveals a Copper." and "topdecks a Copper." never were in it.
func (g *Game) DeckTakes() (map[string][]DeckTake, map[int][]string) {
	r := newZoneReplay(g)
	r.takes, r.offHand = map[string][]DeckTake{}, map[int][]string{}
	for i, e := range g.Events {
		r.step(i, e)
	}
	return r.takes, r.offHand
}

// FinalZones replays the zones to the end of the log.
func (g *Game) FinalZones() map[string]Zones {
	r := newZoneReplay(g)
	for i, e := range g.Events {
		r.step(i, e)
	}
	r.end()
	return r.snapshot()
}

// end finishes what the log's last lines left open: the card a Royal Galley was playing.
func (r *zoneReplay) end() {
	for k := len(r.galleys) - 1; k >= 0; k-- {
		r.setAsideByGalley(r.galleys[k])
	}
	r.galleys = nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// cause is the card whose text the line follows: the line above it, or a Boon or
// Hex received just before at the same level ("receives War", then War's reveals).
func (r *zoneReplay) cause(e Event) string {
	if rs := r.received[e.Player]; len(rs) > 0 && rs[len(rs)-1].level == e.Level {
		return rs[len(rs)-1].card
	}
	if rs := r.shared[""]; e.Cause == "Sacred Grove" && len(rs) > 0 && rs[len(rs)-1].level == e.Level {
		return rs[len(rs)-1].card // the other player's share of its Boon follows with no "receives"
	}
	return e.Cause
}

// discardReceived ends a Boon's or Hex's lines at a level: the receiver's (a Hex's
// attacker discards it), with the cards they named (what The Moon's Gift looked at
// isn't what The Flame's Gift trashes), and starts the shared Boon's that Sacred Grove's other player gets.
func (r *zoneReplay) discardReceived(c *Card, level int) {
	for q, rs := range r.received {
		r.received[q] = slices.DeleteFunc(rs, func(x pending) bool { return x.card == c.Name && x.level == level })
	}
	for q, ps := range r.pend {
		r.pend[q] = slices.DeleteFunc(ps, func(x pending) bool { return x.level >= level })
	}
	if c.Is("Boon") {
		r.shared[""] = append(r.shared[""], pending{card: c.Name, level: level})
	}
}

// replayed reports whether a play line repeats the one before it at the same
// level: Mastermind's "plays a Governor. (Mastermind)" three times, Ghost's
// twice. The card may have left play in between: a Horse returns itself.
func (r *zoneReplay) replayed(i, times int) bool {
	e, n := r.events[i], 0
	for j := i - 1; j >= 0; j-- {
		f := r.events[j]
		if f.Level > e.Level {
			continue
		}
		if f.Level < e.Level || f.Template != e.Template || f.Player != e.Player || f.Args[1].Text != e.Args[1].Text || f.Args[2].Text != e.Args[2].Text {
			break
		}
		n++
	}
	return n%times != 0
}

// fire marks untilTriggered cards in play whose condition an event met.
func (r *zoneReplay) fire(e Event) {
	t := e.Template
	for q, ps := range r.inPlay {
		for _, x := range ps {
			if !untilTriggered[x.name] && x.name != "Taskmaster" || x.fired || x.spent {
				continue
			}
			switch x.name {
			case "Cutthroat":
				if cs := e.Args[1].Cards; gains[t] && q == e.Player && source(e) == x.name && len(cs) > 0 && cs[0].Card != nil && cs[0].Card.Is("Loot") {
					x.fired = true // its Loot: "gains a Jewels. (Cutthroat)", not a Rich Cutthroat's Silver; a Quarry can make a Crown cost $3
				}
			case "Secluded Shrine", "Abundance", "Taskmaster":
				if x.name == "Secluded Shrine" && t == "TRASH_WITH" && source(e) == x.name && q == e.Player {
					x.fired = true // "trashes an Estate. (Secluded Shrine)": also one a gained Gondola played; not Plan's token on a Shrine bought
					return
				}
				if !gains[t] || t == "RECEIVES" || t == "EXCHANGE_RECEIVE" { // a Traveller exchanged isn't gained
					continue
				}
				for _, cn := range e.Args[1].Cards {
					c := cn.Card
					switch {
					case c == nil:
					case x.name == "Secluded Shrine" && q == e.Player && c.Is("Treasure"),
						x.name == "Abundance" && q == e.Player && c.Is("Action"),
						x.name == "Taskmaster" && q == e.Player && q == r.turnOf && c.Coin == 5 && c.Potion == 0 && c.Debt == 0:
						x.fired = true
					}
				}
			case "Flagship":
				if t == "PLAY_WITH" && q == e.Player && strings.TrimSpace(e.Args[2].Text) == "Flagship" {
					x.fired = true
					// replaying a Duration keeps it in play as long as that Duration
					for _, cn := range e.Args[1].Cards {
						if cn.Card == nil || !cn.Card.Is("Duration") {
							continue
						}
						for i := len(ps) - 1; i >= 0; i-- {
							if ps[i].name == cn.Card.Name && ps[i] != x {
								x.holds = ps[i]
								ps[i].none = false // played again in full: an enchanted Enchantress stays after all
								break
							}
						}
					}
					return
				}
			case "Search":
				if t == "DURATION_STOPS_WORKING" && len(e.Args[1].Cards) > 0 && e.Args[1].Cards[0].Card != nil && e.Args[1].Cards[0].Card.Name == "Search" {
					x.fired = true
					return
				}
			}
		}
	}
}

// couldBeUnknown reports whether one of the player's unnamed cards can be this
// card: the player owns more copies than are seen in the zones.
func (r *zoneReplay) couldBeUnknown(p, card string) bool {
	if card == "" {
		return true
	}
	n := r.owned[p][card]
	for _, c := range r.z(p) {
		n -= c.Known[card]
	}
	return n > 0
}

// takeDeck takes a card from the deck, using what is known of its order: a
// named card on top is the next one drawn, cards put at the bottom come last.
func (r *zoneReplay) takeDeck(p string, c *Cards, card string, allowUnknown bool) bool {
	top, bottom := r.top[p], r.bottom[p]
	drop := func(xs []string, name string) ([]string, bool) {
		for i, x := range xs {
			if x == name {
				return append(xs[:i:i], xs[i+1:]...), true
			}
		}
		return xs, false
	}
	hinted := func(name string) int {
		n := 0
		for _, xs := range [][]string{top, bottom} {
			for _, x := range xs {
				if x == name {
					n++
				}
			}
		}
		return n
	}
	if card != "" {
		if c.Known[card] > 0 {
			c.Known[card]--
			if c.Known[card] == 0 {
				delete(c.Known, card)
			}
			var ok bool
			if r.top[p], ok = drop(top, card); !ok && c.Known[card]+1-hinted(card) <= 0 {
				r.bottom[p], _ = drop(bottom, card)
			}
			return true
		}
		if !allowUnknown || c.Unknown == 0 {
			return false
		}
		// an unnamed card turns out to be this one: from the middle if there are unnamed ones there
		c.Unknown--
		if c.Unknown < hinted("") {
			if r.top[p], _ = drop(top, ""); len(r.top[p]) == len(top) {
				r.bottom[p], _ = drop(bottom, "")
			}
		}
		return true
	}
	if !allowUnknown || c.Size() == 0 {
		return false
	}
	pop := func(name string) {
		r.took = name
		if name == "" {
			c.Unknown--
			return
		}
		c.Known[name]--
		if c.Known[name] == 0 {
			delete(c.Known, name)
		}
	}
	if len(top) > 0 {
		r.top[p] = top[1:]
		pop(top[0])
		return true
	}
	// the middle: its named cards can no longer be told apart
	middle := c.Unknown - hinted("")
	for name, k := range c.Known {
		if m := k - hinted(name); m > 0 {
			c.Known[name] -= m
			if c.Known[name] == 0 {
				delete(c.Known, name)
			}
			c.Unknown += m
			middle += m
		}
	}
	if middle > 0 {
		c.Unknown--
		r.took = ""
		return true
	}
	r.bottom[p] = bottom[1:]
	pop(bottom[0])
	return true
}

// topdeckFromPlay are cards that may put themselves onto the deck when they leave play.
var topdeckFromPlay = names("Alchemist", "Merchant Camp", "Tent", "Treasury", "Walled Village")

// guessTopdeck picks the card in play an unnamed Clean-up topdeck most likely
// was: a card that topdecks itself, else the last Action played that would be
// discarded (Scheme), else the last card played.
func (r *zoneReplay) guessTopdeck(p string) string {
	ps := r.inPlay[p]
	for i := len(ps) - 1; i >= 0; i-- {
		if topdeckFromPlay[ps[i].name] {
			return ps[i].name
		}
	}
	for i := len(ps) - 1; i >= 0; i-- {
		if c := ps[i].card; c != nil && c.Is("Action") && !c.Is("Duration") {
			return ps[i].name
		}
	}
	if len(ps) > 0 {
		return ps[len(ps)-1].name
	}
	return ""
}

// cardOf finds the card data for a name the player has in play.
func (r *zoneReplay) cardOf(p, name string) *Card {
	for _, x := range r.inPlay[p] {
		if x.name == name {
			return x.card
		}
	}
	return nil
}
