package engine

// extraCards are the cards scry/cards/cards.json lacks: the members of mixed
// piles, the Loot, and the States it lists in pairs.
var extraCards = func() []cardFacts {
	var out []cardFacts
	add := func(cost string, types []string, names ...string) {
		for _, n := range names {
			out = append(out, cardFacts{Name: n, Kind: "other", Types: types, Cost: cost})
		}
	}
	knight := []string{"Action", "Attack", "Knight"}
	add("5", knight, "Dame Anna", "Dame Molly", "Dame Natalie", "Dame Sylvia", "Sir Bailey", "Sir Destry", "Sir Michael", "Sir Vander")
	add("5", []string{"Action", "Attack", "Knight", "Victory"}, "Dame Josephine")
	add("4", knight, "Sir Martin")
	add("0", []string{"Action", "Ruins"}, "Abandoned Mine", "Ruined Library", "Ruined Market", "Ruined Village", "Survivors")
	add("3", []string{"Treasure", "Victory", "Castle"}, "Humble Castle")
	add("4", []string{"Victory", "Castle"}, "Crumbling Castle")
	add("5", []string{"Action", "Victory", "Castle"}, "Small Castle")
	add("6", []string{"Victory", "Castle"}, "Haunted Castle")
	add("7", []string{"Action", "Victory", "Castle"}, "Opulent Castle")
	add("8", []string{"Victory", "Castle"}, "Sprawling Castle")
	add("9", []string{"Victory", "Castle"}, "Grand Castle")
	add("10", []string{"Victory", "Castle"}, "King's Castle")
	add("7", []string{"Treasure", "Duration", "Loot"}, "Amphora", "Endless Chalice", "Figurehead", "Jewels")
	add("7", []string{"Treasure", "Loot"}, "Doubloons", "Hammer", "Insignia", "Orb", "Prize Goat", "Puzzle Box", "Sextant", "Staff")
	add("7", []string{"Treasure", "Reaction", "Loot"}, "Shield")
	add("7", []string{"Action", "Treasure", "Loot"}, "Spell Scroll")
	add("7", []string{"Treasure", "Attack", "Loot"}, "Sword")
	add("6", []string{"Treasure", "Victory"}, "Farm")
	add("", []string{"State"}, "Miserable", "Twice Miserable", "Envious", "Deluded")
	return out
}()

type member struct {
	name string
	n    int
}

// pileMembers says what goes in a kingdom pile that isn't 10 copies of its
// card, top first; shuffled piles are shuffled at setup.
var pileMembers = map[string][]member{
	"Knights": {{"Dame Anna", 1}, {"Dame Josephine", 1}, {"Dame Molly", 1}, {"Dame Natalie", 1}, {"Dame Sylvia", 1},
		{"Sir Bailey", 1}, {"Sir Destry", 1}, {"Sir Martin", 1}, {"Sir Michael", 1}, {"Sir Vander", 1}},
	"Castles": {{"Humble Castle", 1}, {"Crumbling Castle", 1}, {"Small Castle", 1}, {"Haunted Castle", 1},
		{"Opulent Castle", 1}, {"Sprawling Castle", 1}, {"Grand Castle", 1}, {"King's Castle", 1}},
	"Catapult / Rocks":            {{"Catapult", 5}, {"Rocks", 5}},
	"Encampment / Plunder":        {{"Encampment", 5}, {"Plunder", 5}},
	"Gladiator / Fortune":         {{"Gladiator", 5}, {"Fortune", 5}},
	"Patrician / Emporium":        {{"Patrician", 5}, {"Emporium", 5}},
	"Settlers / Bustling Village": {{"Settlers", 5}, {"Bustling Village", 5}},
	"Sauna / Avanto":              {{"Sauna", 5}, {"Avanto", 5}},
	"Harem / Farm":                {{"Farm", 8}},
	"Augurs":                      {{"Herb Gatherer", 4}, {"Acolyte", 4}, {"Sorceress", 4}, {"Sibyl", 4}},
	"Clashes":                     {{"Battle Plan", 4}, {"Archer", 4}, {"Warlord", 4}, {"Territory", 4}},
	"Forts":                       {{"Tent", 4}, {"Garrison", 4}, {"Hill Fort", 4}, {"Stronghold", 4}},
	"Odysseys":                    {{"Old Map", 4}, {"Voyage", 4}, {"Sunken Treasure", 4}, {"Distant Shore", 4}},
	"Townsfolk":                   {{"Town Crier", 4}, {"Blacksmith", 4}, {"Miller", 4}, {"Elder", 4}},
	"Wizards":                     {{"Student", 4}, {"Conjurer", 4}, {"Sorcerer", 4}, {"Lich", 4}},
}

// shuffledPiles are mixed piles shuffled at setup.
var shuffledPiles = map[string]bool{"Knights": true}

// Pile is a pile of cards: a Supply pile, or a non-Supply one such as the Spoils.
type Pile struct {
	Card   CardID // the pile's card: the kingdom card, "Knights", "Ruins", or the card all of it is
	Cards  []Inst // the top is the end
	Supply bool
	Size   int // how many it started with

	Embargo  int    // Embargo tokens
	Trade    bool   // a Trade Route token, until a card is gained from it
	VP       int    // VP tokens on it (Gathering cards, Aqueduct...); debt tokens (Tax) are Debt
	Debt     int    //
	Trait    CardID // the Trait on it
	Coins    int    // coin tokens (Aqueduct-like landmarks keep VP; Silver/Gold piles for Aqueduct use VP)
	Bane     bool   // Young Witch's or Ferryman's pile
	members  []CardID
}

// AddPile adds a pile of n copies of a card; it returns the pile's index.
func (g *Game) AddPile(c CardID, n int, supply bool) int {
	if i, ok := g.pileOf[c]; ok {
		return i
	}
	i := len(g.Piles)
	g.Piles = append(g.Piles, Pile{Card: c, Supply: supply, Size: n})
	g.pileOf[c] = i
	for range n {
		g.newCard(c, Loc{Supply, int16(i)})
	}
	return i
}

// addMixed adds a pile of several cards, the first listed on top; keep > 0
// keeps that many of them, after a shuffle.
func (g *Game) addMixed(c CardID, ms []member, supply, shuffle bool, keep int) int {
	i := len(g.Piles)
	g.Piles = append(g.Piles, Pile{Card: c, Supply: supply})
	g.pileOf[c] = i
	var xs []CardID
	for k := len(ms) - 1; k >= 0; k-- {
		id := g.T.MustCard(ms[k].name)
		g.pileOf[id] = i
		g.Piles[i].members = append(g.Piles[i].members, id)
		for range ms[k].n {
			xs = append(xs, id)
		}
	}
	if shuffle {
		g.rng.Shuffle(len(xs), func(a, b int) { xs[a], xs[b] = xs[b], xs[a] })
	}
	if keep > 0 {
		xs = xs[len(xs)-keep:]
	}
	for _, id := range xs {
		g.newCard(id, Loc{Supply, int16(i)})
	}
	g.Piles[i].Size = len(xs)
	return i
}

// AddRuins adds the Ruins pile: 10 random Ruins, for the Looters.
func (g *Game) AddRuins() {
	c := g.T.MustCard("Ruins")
	if _, ok := g.pileOf[c]; ok {
		return
	}
	var ms []member
	for _, n := range []string{"Abandoned Mine", "Ruined Library", "Ruined Market", "Ruined Village", "Survivors"} {
		ms = append(ms, member{n, 10})
	}
	g.addMixed(c, ms, true, true, 10)
}

// Need adds a non-Supply pile a card uses, e.g. Spoils for Bandit Camp.
func (g *Game) Need(name string, n int) {
	g.AddPile(g.T.MustCard(name), n, false)
}

// PileOf is the index of the pile a card comes from, or -1.
func (g *Game) PileOf(c CardID) int {
	if i, ok := g.pileOf[c]; ok {
		return i
	}
	return -1
}

// Top is the card on top of a pile, or 0 when it is empty.
func (g *Game) Top(i int) CardID {
	if s := g.Piles[i].Cards; len(s) > 0 {
		return g.cards[s[len(s)-1]]
	}
	return 0
}

// kinds lists the card kinds a pile holds or started with.
func (p *Pile) kinds(g *Game) []CardID {
	if len(p.members) > 0 {
		return p.members
	}
	return []CardID{p.Card}
}
