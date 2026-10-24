package cartographer

import (
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
var discardLookers = names("Annex", "Bury", "Bustling Village", "Counting House", "Courier", "Harbinger", "Herald",
	"Hermit", "Inn", "March", "Mountain Village", "Orb", "Scavenger", "Settlers", "The Moon's Gift")

// Cards that discard or trash the top card of a deck without revealing it first.
var (
	topDiscarders = names("Courier", "Gamble", "Jester", "Page", "Sea Hag", "Vassal", "Warrior")
	topTrashers   = names("Barbarian", "Locusts", "Swindler", "Zombie Mason")
)

// Cards gained somewhere other than the discard pile, without a line saying so:
// by the card gained, or by what gained it (the line above, or "(Sickness)").
var (
	gainedToHand   = names("Den of Sin", "Ghost Town", "Guardian", "Night Watchman")
	gainedOntoDeck = names("Nomad Camp")
	gainsToHand    = names("Artisan", "Beggar", "Falconer", "Joust", "Kind Emperor",
		"Mine", "Pickaxe", "Pirate", "Plague", "Sculptor", "Silver Mine", "Swap", "Torturer", "Trading Post",
		"Transmogrify", "Treasurer", "Wish")
	gainsOntoDeck = names("Armory", "Artificer", "Bureaucrat", "Demand", "Develop", "Fool's Gold",
		"Greed", "Invasion", "Sickness", "Stampede", "Supplies")
	gainsSettingAside = names("Quartermaster")
	// Research sets cards aside from the deck, not the hand
	setsAsideFromDeck = names("Research")
)

// Replays that play a card again without moving it, and plays of cards the player
// doesn't take (Captain plays a Supply card where it is).
var playsInPlace = names("Daimyo", "Flagship", "Captain")

// Cards that play a card from the Supply or the trash, leaving it there.
var playsFromElsewhere = names("Band of Misfits", "Captain", "Necromancer", "Overlord")

// Durations whose staying in play depends on what they did this turn.
var (
	staysIfSetAside = names("Archive", "Blockade", "Cargo Ship", "Church", "Contract", "Crypt", "Gear", "Grotto", "Haven", "Research", "Royal Galley")
	staysIfDiscards = names("Secret Cave", "Tactician")
	// Durations that work now or next turn, by choice: the line that shows "now"
	leavesIfNow = map[string]string{"Amphora": "GETS_BUY", "Barge": "GETS_BUY", "Gondola": "GETS_COIN", "Village Green": "GETS_ACTIONS"}
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
	name  string
	card  *Card
	turn  int     // the turn it was played on (count of NEW_TURN lines)
	stays int     // the owner's Clean-ups it still stays through; -1 until decided
	holds *played // the Duration a Throne Room-like card played
	by    string  // the card whose line played it: a Throne Room
	again bool    // played more than once: by a Throne Room, not a Shop
	fired bool    // an untilTriggered card whose time came
	back  bool    // played by Prince: set aside again instead of discarded
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
	zones    map[string]Zones
	avoid    map[string]Deck     // cards Avoid keeps out of the next shuffle
	kept     map[string]Deck     // cards kept in hand through this Clean-up
	top      map[string][]string // cards known to be on top of the deck, top first ("" for an unnamed one)
	bottom   map[string][]string // cards known to be at the bottom, the one nearest the middle first
	deckPos  string              // where the current line puts cards in the deck: "top", "bottom" or ""
	took     string              // the card the last takeOne took, when the deck's order names an unnamed one
	cleaning string              // the player whose Clean-up has begun
	owned    Decks
	received map[string][]pending // Boons and Hexes being resolved: their lines follow at the same level
	reacted  map[string][]pending // reactions being resolved: "reacts with a Tunnel", then its Gold
	playAt   map[int]*played      // the card the latest play line at each level played
	inPlay   map[string][]*played
	pend     map[string][]pending
	turn     int            // NEW_TURN lines so far
	turnOf   string         // whose turn it is
	evidence map[string]int // this turn: "set aside:Gear", "discard:Tactician", "draw:Barge" counts
	cleanup  map[int]bool   // event indices that start a Clean-up
	events   []Event
	misses   []ZoneMiss
	line     int                   // the line being replayed
	latest   map[int]int           // the last line at each level: the current line's parents
	takes    map[string][]DeckTake // every card taken from each player's deck
}

func newZoneReplay(g *Game) *zoneReplay {
	r := &zoneReplay{zones: map[string]Zones{}, avoid: map[string]Deck{}, kept: map[string]Deck{}, top: map[string][]string{}, bottom: map[string][]string{}, owned: Decks{}, received: map[string][]pending{}, reacted: map[string][]pending{}, playAt: map[int]*played{}, inPlay: map[string][]*played{}, pend: map[string][]pending{}, evidence: map[string]int{}, latest: map[int]int{}}
	for _, p := range g.Players {
		r.zones[p] = Zones{}
	}
	r.cleanup = cleanupPoints(g)
	r.events = g.Events
	return r
}

var beforeShuffle = names("PUT_IN_DISCARD", "SPENDS_FAVOR", "SPENDS_FAVORS", "BOTTOMDECKS")

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
		at := end
		for i := end - 1; i > start; i-- {
			e := g.Events[i]
			if e.Template == "DRAW" && e.Level == 0 && e.Player == p {
				at = i
				// and before the shuffle, and what was done while shuffling: Order of
				// Masons and Avoid keep cards in the discard pile, Shadow cards go under the deck
				for at-1 > start {
					x := g.Events[at-1]
					if x.Player != p || !(x.Template == "SHUFFLES" && x.Level == 0 || beforeShuffle[x.Template]) {
						break
					}
					at--
				}
				break
			}
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
	// the log's last turn is usually cut off mid-turn: no Clean-up
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
func (r *zoneReplay) takeOne(p, zone, card string, allowUnknown bool) bool {
	r.took = card
	c := r.z(p)[zone]
	if c == nil {
		return false
	}
	if zone == ZoneDeck {
		if !r.takeDeck(p, c, card, allowUnknown) {
			return false
		}
		if r.takes != nil {
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
			func(x pending) bool {
				return x.level < e.Level && r.latest[x.level] == x.line && contains(want, x.zone)
			},
			func(x pending) bool {
				return contains(want, x.zone) && x.src == source(e) && !(x.gained && x.level == e.Level && strings.HasPrefix(e.Template, "TRASH"))
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
			if t.zone == ZonePlay {
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
			if e.CauseOf == "RETURN_TO" || gains[e.CauseOf] {
				by = "" // a Trader's Silver for a Fool's Gold: the card returned or gained didn't gain it
			}
			if a := e.Args[2]; e.Template == "GAIN_WITH" && a.Text != "" {
				by = strings.TrimSpace(a.Text)
			}
			if rs := r.reacted[p]; len(rs) > 0 && rs[len(rs)-1].level == e.Level {
				by = "" // a reaction's: the Tunnel's Gold, not one onto the deck for the Fool's Gold discarded with it; "(Beggar)" Silver
				if rs[len(rs)-1].card == "Urchin" {
					r.reacted[p] = rs[:len(rs)-1] // its one Mercenary: the Bureaucrat's Silver after it goes onto the deck
				}
			}
			switch {
			case gainedToHand[name] || gainsToHand[by]:
				dest = ZoneHand
			case gainedOntoDeck[name] || gainsOntoDeck[by]:
				dest = ZoneDeck
			case gainsSettingAside[by]:
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
		// in Clean-up Scheme topdecks the Monkey that's done, not the one staying out;
		// but Herbalist's Treasure is one still in play, played after it, not last turn's Astrolabe, discarded first
		if r.cleaning == p && ps[i].stays == 0 && !(ps[i].card != nil && ps[i].card.Is("Treasure")) {
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

// note records the parts of a turn that decide whether a Duration stays.
func (r *zoneReplay) note(kind, card string) {
	if card != "" {
		r.evidence[kind+":"+card]++
	}
}

// expire drops pending cards whose line has ended, and puts revealed cards
// nothing took back onto the deck.
func (r *zoneReplay) expire(e Event) {
	for _, m := range []map[string][]pending{r.pend, r.received, r.reacted} {
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
		if !used || e.Template == "NEW_TURN" {
			r.moveAll(p, ZoneRevealed, ZoneDeck)
		}
	}
}

// cleanUp discards the turn player's hand and the cards in play that don't stay.
func (r *zoneReplay) cleanUp(p string) {
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
	if permanent[x.name] {
		return true
	}
	if x.holds != nil {
		return true // decided by the card it holds
	}
	if untilTriggered[x.name] {
		return !x.fired
	}
	if owner != p {
		// another player's card: a Duration played on this turn waits for its owner's turn
		return x.turn < r.turn || (x.card != nil && x.card.Is("Duration"))
	}
	if x.holds != nil {
		return true // decided by the card it holds
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
		switch {
		case staysIfSetAside[x.name] && !use("set aside"):
			return false
		case staysIfDiscards[x.name] && !use("discard"):
			return false
		case leavesIfNow[x.name] != "" && use("now"):
			return false
		case x.name == "Outpost" && r.evidence["extra turn"]+r.evidence["stays:Outpost"] > 0:
			return false // it only works as the first Outpost of a turn after another player's
		}
		if x.name == "Outpost" {
			r.evidence["stays:Outpost"]++
		}
		x.stays = 0
		switch x.name {
		case "Archive":
			x.stays = 1 // two more turns of putting a card into hand
		case "Crypt":
			x.stays = max(0, r.evidence["set aside:Crypt"]-1)
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
	defer r.owned.Apply(e)
	r.line = e.Line
	r.latest[e.Level] = e.Line
	r.deckPos = ""
	switch e.Template {
	case "TOPDECK", "TOPDECK_WITH", "BACK_ON_DECK", "GAIN_ON_DRAWPILE":
		r.deckPos = "top"
	case "BOTTOMDECKS", "PUT_ON_BOTTOM_OF":
		r.deckPos = "bottom"
	}
	if r.cleanup[i] {
		r.cleanUp(r.turnOf)
	}
	r.expire(e)
	r.fire(e)
	if e.Template == "NEW_TURN" {
		if e.Extra != "" {
			r.evidence["extra turn"]++
		}
		r.turn++
		r.turnOf = e.Player
		r.pend = map[string][]pending{}
		r.received = map[string][]pending{}
		r.reacted = map[string][]pending{}
		r.playAt = map[int]*played{}
		r.cleaning = ""
		return
	}
	p := e.Player
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
		for _, cn := range cards {
			if cn.Card != nil && (cn.Card.Name == e.Cause || cn.Card.Name == arg2) {
				return append([]string{ZonePlay}, want...)
			}
		}
		return want
	}
	cause := r.cause(e)
	if leavesIfNow[cause] == e.Template {
		r.note("now", cause)
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
		strings.HasPrefix(r.events[i-1].Template, "TRASH") && r.events[i-1].Player == p:
		r.gain(e, p, cards, ZoneDeck) // another player's Province: "trashes a Fool's Gold.", "gains a Gold." onto the deck
	case gains[t]:
		r.gain(e, p, cards, ZoneDiscard)
	case t == "EXILE" && supplyExilers[e.Cause]:
		r.gain(e, p, cards, ZoneExile)

	// cards that leave the player
	case strings.HasPrefix(t, "TRASH"):
		if trashesSupply(e) {
			break
		}
		want := self(ZoneHand, ZonePlay, ZoneDiscard)
		if cause == "Jack of All Trades" && strings.HasPrefix(e.CauseOf, "PLAY") { // not a Watchtower trashing the Jack just bought
			want = []string{ZoneHand} // not the card it just discarded from the deck
		}
		if topTrashers[cause] && !gains[e.CauseOf] { // a Swindler played, not one a Watchtower trashes as it's gained
			want = []string{ZoneDeck}
		}
		if n := i + 1; len(cards) == 1 && cards[0].Card != nil && cards[0].Card.Name == "Urchin" && n < len(r.events) &&
			r.events[n].Template == "GAIN" && r.events[n].Player == p && r.events[n].Level == e.Level && r.events[n].Args[1].Text == "a Mercenary" {
			want = []string{ZonePlay} // an Urchin in play, trashed for a Mercenary as another Attack is played
			r.reacted[p] = append(r.reacted[p], pending{card: "Urchin", level: e.Level})
		}
		itself := false // the gained card, not what a gained Souk trashes from hand
		for _, cn := range cards {
			itself = itself || cn.Card == nil || cn.Card.Name == e.Cause
		}
		if strings.Contains(e.CauseOf, "GAIN") && itself {
			want = append(want, ZoneDeck, ZoneSetAside) // Watchtower trashing what Develop gained onto the deck, or Blockade set aside
		}
		r.lose(e, p, cards, want)
		if e.CauseOf == "GAIN_SETTING_ASIDE" {
			r.leftAside(i)
		}
	case t == "PUT_ON_BOTTOM_OF" && !strings.EqualFold(strings.TrimSpace(e.Args[2].Text), "Draw Pile") && !toSupplyPile(e.Args[2].Text):
		// the Black Market deck: not the player's card
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
		r.replay(e, p, cards)
	case t == "PLAY" && cause == "Estate" && strings.HasPrefix(e.CauseOf, "PLAY"): // an Estate playing the card it inherited, not a Weaver discarded with one
	case t == "PLAY_NULLIFIED": // "plays Copper (nullified by Highwayman)" describes the play above
	case t == "PLAY_WITH" && (playsInPlace[arg2] || (arg2 == "Mastermind" || arg2 == "Ghost") && r.replayed(p, cards, e.Level)): // they log each play
	case t == "PLAY_WITH" && (arg2 == "Mastermind" || (arg2 == "Royal Galley" || arg2 == "") && e.Cause != ""): // at the start of a turn they come from set aside
		r.play(e, p, cards, []string{ZoneHand, ZoneSetAside, ZoneDiscard})
	case t == "PLAY_WITH" && gains[e.CauseOf]: // Sailor plays what a Blockade set aside, or Develop put onto the deck
		r.play(e, p, cards, []string{ZoneSetAside, ZoneDeck, ZoneDiscard, ZoneHand})
		if e.CauseOf == "GAIN_SETTING_ASIDE" {
			r.leftAside(i)
		}
	case t == "PLAY_WITH":
		r.play(e, p, cards, []string{ZoneSetAside, ZoneHand, ZoneDiscard})
	case t == "PLAY" && playsFromElsewhere[cause]:
	case t == "PLAY" && i > 0 && r.events[i-1].Template == "REACTS_PLAYING" && r.events[i-1].Player == p && r.events[i-1].Level == e.Level:
		// "reacts with a Guard Dog, playing it." and then "plays a Guard Dog." are one play
	case t == "REACTS_PLAYING" && e.CauseOf == "GAIN_SETTING_ASIDE": // "gains a Trail setting it aside.", "reacts with a Trail, playing it."
		r.play(e, p, cards, []string{ZoneSetAside, ZoneHand, ZoneDiscard, ZoneDeck})
		r.leftAside(i)
	case t == "PLAY", t == "PLAY_TREASURES_FOR", t == "PLAY_USING_WAY", t == "PLAY_ENLIGHTENED", t == "PLAY_NULLIFIED", t == "REACTS_PLAYING":
		r.play(e, p, cards, []string{ZoneHand, ZoneSetAside, ZoneDiscard, ZoneDeck})
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
		case topDiscarders[cause] && len(cards) == 1 && cards[0].N == 1 && (arg2 == "" || arg2 == cause): // more is a Diplomat reacting to it; "(Beggar)" a Beggar reacting
			want, rest = []string{ZoneDeck}, nil // never the card in play: a Vassal that discards a Vassal
		case cause == "Library":
			want, rest = []string{ZoneSetAside}, nil // the Actions it skipped, not copies it drew
		case cause == "Tactician":
			want, rest = []string{ZoneHand}, nil // the hand, with any other Tactician in it
		case e.Cause == "" && e.Level > 0 && t == "DISCARD": // at the start of a turn: Grotto discards what it set aside (Tide Pools names itself)
			want = []string{ZoneSetAside, ZoneHand}
		}
		r.move(e, p, cards, append(want, rest...), ZoneDiscard)
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
				if d := r.z(p)[ZoneDeck]; d != nil && d.Size() >= n {
					r.misses = append(r.misses, ZoneMiss{Line: e.Line, Player: p, Template: t, Cause: e.Cause, Want: "a deck smaller than the next draw", Found: ZoneDeck})
				}
			}
		}
		r.moveAll(p, ZoneDiscard, ZoneDeck)
		for n, k := range r.avoid[p] {
			r.put(p, ZoneDiscard, n, k)
		}
		delete(r.avoid, p)
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
	case t == "SHUFFLE_INTO_DECK" && cause == "Inn" && gains[e.CauseOf]: // the Actions in the discard pile, not those in hand
		r.move(e, p, cards, []string{ZoneDiscard}, ZoneDeck)
	case t == "TOPDECK" && discardLookers[cause]: // Harbinger's pick from the discard pile, not the Harbinger in play
		r.move(e, p, cards, []string{ZoneDiscard}, ZoneDeck)
	case t == "TOPDECK", t == "TOPDECK_WITH", t == "BACK_ON_DECK", t == "BOTTOMDECKS", t == "INSERT_IN_DECK", t == "SHUFFLE_INTO_DECK",
		t == "PUT_ON_BOTTOM_OF": // "on the bottom of Draw Pile"
		// Inn shuffles first
		r.move(e, p, cards, self(ZoneHand, ZoneDiscard, ZonePlay, ZoneSetAside, ZoneDeck), ZoneDeck)
		if e.CauseOf == "GAIN_SETTING_ASIDE" { // Watchtower topdecked what Blockade set aside
			r.leftAside(i)
		}
	case t == "PUT_IN_HAND_FROM":
		r.move(e, p, cards, []string{ZoneSetAside, ZoneNativeVillage, ZonePlay}, ZoneHand) // Blockade's gained card; Conjurer itself from play
	case t == "PUT_IN_HAND" && cause == "Native Village": // the mat's cards, not the Native Village just played
		r.move(e, p, cards, []string{ZoneNativeVillage}, ZoneHand)
	case t == "PUT_IN_HAND":
		r.move(e, p, cards, []string{ZoneSetAside, ZoneNativeVillage, ZoneDiscard, ZoneDeck, ZonePlay}, ZoneHand)
	case t == "SET_ASIDE_WITH" && arg2 == "Native Village":
		r.move(e, p, cards, []string{ZoneDeck}, ZoneNativeVillage)
	case t == "SET_ASIDE" && setsAsideFromDeck[cause]:
		r.move(e, p, cards, []string{ZoneDeck}, ZoneSetAside)
		r.note("set aside", cause)
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
		holder := e.Cause
		if arg2 != "" {
			holder = arg2
		}
		r.note("set aside", holder)
	case t == "EXILE":
		r.move(e, p, cards, self(ZoneHand, ZonePlay), ZoneExile)
	case t == "PUT_ON_MAT":
		mat := ZoneTavern
		if strings.Contains(e.Args[2].Text, "Island") {
			mat = ZoneIsland
		}
		r.move(e, p, cards, self(ZoneHand, ZonePlay), mat)
	case t == "CALL":
		r.play(e, p, cards, []string{ZoneTavern})

	// looking at cards
	case t == "REVEAL", t == "LOOK_AT":
		switch {
		case t == "REVEAL" && i > 0 && r.events[i-1].Template == "REACTS_WITH" && r.events[i-1].Player == e.Player && r.events[i-1].Level == e.Level:
			// "reacts with a Tunnel.", "reveals a Tunnel.": it shows itself where it is, not a Vagrant's top card when a Storeroom discarded both
		case t == "LOOK_AT" && cause == "Library": // Library draws the cards it looks at
			r.move(e, p, cards, []string{ZoneDeck}, ZoneHand)
		case discardLookers[cause] && !strings.HasPrefix(e.CauseOf, "PLAY"): // Herald's overpay, Inn when gained
			r.mark(e, p, cards, ZoneDiscard)
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
	if e.Cause == "Royal Galley" && e.Template == "PLAY" { // it sets the card aside without a line
		r.move(e, p, mine, want, ZoneSetAside)
		r.note("set aside", "Royal Galley")
		return
	}
	r.move(e, p, mine, want, ZonePlay)
	r.playAt[e.Level] = nil
	if len(r.inPlay[p]) == n+1 {
		r.playAt[e.Level] = r.inPlay[p][n]
		r.inPlay[p][n].by = e.Cause
	}
	if strings.TrimSpace(e.Args[2].Text) == "Prince" {
		for _, x := range r.inPlay[p][n:] {
			x.back = true
		}
	}
}

// replay notes a card played again: a Throne Room that plays a Duration twice
// stays in play as long as that Duration, and so does a Throne Room that plays
// that Throne Room.
func (r *zoneReplay) replay(e Event, p string, cards []CardN) {
	ps := r.inPlay[p]
	for _, cn := range cards {
		if cn.Card == nil {
			continue
		}
		var x *played
		for i := len(ps) - 1; i >= 0; i-- {
			if ps[i].name == cn.Card.Name {
				x = ps[i]
				break
			}
		}
		if x != nil {
			x.again = true
		}
		if x == nil || !cn.Card.Is("Duration") && x.holds == nil {
			continue
		}
		for i := len(ps) - 1; x != nil && i >= 0; i-- {
			if h := ps[i]; h.name == e.Cause && h.holds == nil && h != x {
				h.holds = x
				break
			}
		}
		// and the Throne Room that played that Throne Room, though it played it before the Duration
		for k := e.Level - 1; k > 0; k-- {
			in, out := r.playAt[k], r.playAt[k-1]
			if in == nil || out == nil || in.holds == nil || !in.again || out.holds != nil || in.by != out.name {
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
// log order. A replay of the game draws them in this order.
func (g *Game) DeckTakes() map[string][]DeckTake {
	r := newZoneReplay(g)
	r.takes = map[string][]DeckTake{}
	for i, e := range g.Events {
		r.step(i, e)
	}
	return r.takes
}

// FinalZones replays the zones to the end of the log.
func (g *Game) FinalZones() map[string]Zones {
	r := newZoneReplay(g)
	for i, e := range g.Events {
		r.step(i, e)
	}
	return r.snapshot()
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
	return e.Cause
}

// replayed reports whether the cards were just put into play by a line at the
// same level: Mastermind's "plays a Governor. (Mastermind)" three times.
func (r *zoneReplay) replayed(p string, cards []CardN, level int) bool {
	for _, cn := range cards {
		if cn.Card == nil {
			return false
		}
		found := false
		for _, x := range r.pend[p] {
			if x.card == cn.Card.Name && x.zone == ZonePlay && x.level == level {
				found = true
			}
		}
		if c := r.z(p)[ZonePlay]; !found || c == nil || c.Known[cn.Card.Name] == 0 {
			return false
		}
	}
	return len(cards) > 0
}

// fire marks untilTriggered cards in play whose condition an event met.
func (r *zoneReplay) fire(e Event) {
	t := e.Template
	for q, ps := range r.inPlay {
		for _, x := range ps {
			if !untilTriggered[x.name] || x.fired {
				continue
			}
			switch x.name {
			case "Secluded Shrine", "Abundance", "Cutthroat":
				if !gains[t] {
					continue
				}
				for _, cn := range e.Args[1].Cards {
					c := cn.Card
					switch {
					case c == nil:
					case x.name == "Secluded Shrine" && q == e.Player && c.Is("Treasure"),
						x.name == "Abundance" && q == e.Player && c.Is("Action"),
						x.name == "Cutthroat" && c.Is("Treasure") && c.Coin >= 5:
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
var topdeckFromPlay = names("Alchemist", "Merchant Camp", "Treasury", "Walled Village")

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
