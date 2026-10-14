package cartographer

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func loadClient(t *testing.T) *Client {
	t.Helper()
	p := DefaultClientPath()
	if p == "" {
		t.Skip("no exported dominion.games client tables (set SCRY_CLIENT or export them with scry)")
	}
	c, err := LoadClient(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Every saved log must be read completely, and must end with the decks its
// score screen shows. The bot that took the opponent's seat keeps its name in
// both files.
func TestLogsMatchScoreScreens(t *testing.T) {
	c := loadClient(t)
	logs, _ := filepath.Glob("../../testdata/logs/*.log")
	if len(logs) == 0 {
		t.Fatal("no test logs")
	}
	for _, lp := range logs {
		id := strings.TrimSuffix(filepath.Base(lp), ".log")
		t.Run(id, func(t *testing.T) {
			b, _ := os.ReadFile(lp)
			sb, err := os.ReadFile(strings.TrimSuffix(lp, ".log") + ".score.txt")
			if err != nil { // older logs have no score screen: they must still parse completely
				g, err := c.Parse(string(b), nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(g.Unknown) > 0 {
					t.Errorf("%d unknown lines, first: line %d %q", len(g.Unknown), g.Unknown[0].Line, g.Unknown[0].Text)
				}
				return
			}
			for _, p := range c.Check(string(b), string(sb)).Problems {
				t.Error(p)
			}
		})
	}
}

// Game 184518886 (gamrevinu vs UtagawaLk, 2026-10-10) has no saved score screen;
// its expected decks are DomBot's !game picture for that game.
func TestFinalDecksMatchDomBot(t *testing.T) {
	c := loadClient(t)
	b, err := os.ReadFile("../../testdata/logs/184518886.log")
	if err != nil {
		t.Fatal(err)
	}
	g, err := c.Parse(string(b), map[string]string{"Lord Rattington": "UtagawaLk"})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Unknown) != 0 {
		t.Errorf("%d unknown lines", len(g.Unknown))
	}
	if !reflect.DeepEqual(g.Players, []string{"gamrevinu", "UtagawaLk"}) {
		t.Errorf("players %v", g.Players)
	}
	want := Decks{
		"gamrevinu": {"Hostelry": 7, "Destrier": 3, "Shaman": 3, "Groom": 7, "Estate": 6, "Duchy": 1, "Copper": 3, "Silver": 1, "Horse": 2},
		"UtagawaLk": {"Hostelry": 3, "Innkeeper": 2, "Destrier": 7, "Shaman": 3, "Menagerie": 2, "Fortress": 1, "Groom": 3, "Province": 1, "Gold": 2, "Horse": 5},
	}
	if got := g.FinalDecks(); !reflect.DeepEqual(got, want) {
		t.Errorf("decks:\n got  %v\n want %v", got, want)
	}
}

// Every template must be reachable: a line built from it must match it, or a
// template with the same English, and not some other template.
func TestEveryTemplateMatchesItsOwnLines(t *testing.T) {
	c := loadClient(t)
	initials := map[string]string{"g": "gamrevinu", "L": "Lord Rattington"}
	sample := map[int]string{0: "g", 1: "a Copper", 2: "L", 3: "a Silver"}
	checked := 0
	for _, tm := range c.Templates {
		if tm.generic() {
			continue
		}
		line := reArg.ReplaceAllStringFunc(tm.English, func(s string) string {
			var n int
			fmt.Sscan(strings.TrimPrefix(s, "Argument"), &n)
			return " " + sample[n] + " "
		})
		line = reCardRef.ReplaceAllStringFunc(line, func(s string) string {
			if card := c.byKeyName(s[1 : len(s)-1]); card != nil {
				return " " + card.Name + " "
			}
			return " " + s[1:len(s)-1] + " "
		})
		// like a copied log: spans joined by single spaces
		line = strings.Join(strings.Fields(line), " ")
		got, _ := c.matchLine(line, initials)
		if got == nil {
			t.Errorf("%s: %q matches no template", tm.Name, line)
			continue
		}
		if got.English != tm.English {
			t.Errorf("%s: %q is read as %s (%q)", tm.Name, line, got.Name, got.English)
		}
		checked++
	}
	t.Logf("%d templates checked", checked)
}

// Every card must be readable in every form the log uses.
func TestEveryCardForm(t *testing.T) {
	c := loadClient(t)
	bad := 0
	for _, card := range c.Cards {
		if card.Key == "Back" {
			continue
		}
		for _, f := range []struct {
			text string
			n    int
		}{{card.Name, 1}, {card.Noun, 1}, {"2 " + card.Plural, 2}} {
			if strings.TrimSpace(f.text) == "" || f.text == "2 " {
				continue
			}
			cs, ok := c.CardList(f.text)
			if !ok || len(cs) != 1 || cs[0].Card == nil || cs[0].N != f.n || cs[0].Card.Name != card.Name {
				if bad < 15 {
					got := "nothing"
					if ok && len(cs) > 0 {
						got = cs[0].String()
						if len(cs) > 1 {
							got += fmt.Sprintf(" and %d more", len(cs)-1)
						}
					}
					t.Errorf("%q (%s) reads as %s", f.text, card.Name, got)
				}
				bad++
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d card forms misread in all", bad)
	}
	t.Logf("%d cards checked", len(c.Cards))
}

// Logs to replay: the test logs, plus every saved game when SCRY_GAMES points at their folder.
func corpus(t *testing.T) []string {
	logs, _ := filepath.Glob("../../testdata/logs/*.log")
	if dir := os.Getenv("SCRY_GAMES"); dir != "" {
		more, _ := filepath.Glob(filepath.Join(dir, "*.log"))
		for _, f := range more {
			if !strings.HasSuffix(f, ".bot.log") { // moves a bot played after loading, not a game
				logs = append(logs, f)
			}
		}
	}
	return logs
}

// The turn-by-turn replay must end where FinalDecks does, and no balance may go
// below zero at the end of any turn (a negative one means a missed gain).
func TestTurnsAreConsistent(t *testing.T) {
	c := loadClient(t)
	for _, lp := range corpus(t) {
		b, _ := os.ReadFile(lp)
		g, err := c.Parse(string(b), nil)
		if err != nil {
			t.Fatal(err)
		}
		turns := g.Turns()
		if len(turns) == 0 {
			continue
		}
		id := filepath.Base(lp)
		if last := turns[len(turns)-1].Decks; !reflect.DeepEqual(last, g.FinalDecks()) {
			t.Errorf("%s: last turn's decks differ from FinalDecks", id)
		}
		for _, tu := range turns {
			for p, cs := range tu.Counters {
				if cs.Coffers < 0 || cs.Villagers < 0 || cs.Favors < 0 || cs.Debt < 0 {
					t.Errorf("%s turn %d (%s): %s has a negative balance %+v", id, tu.Number, tu.Player, p, cs)
				}
			}
			for p, d := range tu.Decks {
				for card, n := range d {
					if n < 0 {
						t.Errorf("%s turn %d: %s owns %d %s", id, tu.Number, p, n, card)
					}
				}
			}
		}
	}
}

// Game 184518886 at its end, as the dominion.games client showed it (screenshot, 2026-10-10).
func TestSupplyMatchesClient(t *testing.T) {
	c := loadClient(t)
	b, err := os.ReadFile("../../testdata/logs/184518886.log")
	if err != nil {
		t.Fatal(err)
	}
	g, _ := c.Parse(string(b), nil)
	s := g.Supply(c)
	want := map[string]int{
		"Province": 7, "Gold": 28, "Duchy": 7, "Silver": 39, "Estate": 3, "Copper": 46, "Horse": 23,
		"Hostelry": 0, "Innkeeper": 8, "Destrier": 0, "Shaman": 4, "Menagerie": 8, "Fortress": 9, "Groom": 0,
	}
	for pile, n := range want {
		got, ok := s.Left[pile]
		if !ok { // never touched: still full
			got = c.InitialSize(pile, 2)
		}
		if got != n {
			t.Errorf("%s: %d left, the client showed %d", pile, got, n)
		}
	}
	for _, p := range []string{"Curse", "Giant", "Silver Mine", "Pawn"} { // untouched, still full
		if _, ok := s.Left[p]; ok && s.Left[p] != s.Initial[p] {
			t.Errorf("%s: %d left of %d, the client showed it full", p, s.Left[p], s.Initial[p])
		}
	}
	if got := s.EmptyPiles(); len(got) != 3 {
		t.Errorf("empty piles %v, the client showed Hostelry, Destrier and Groom", got)
	}
}

// After every turn each player's zones hold exactly the cards they own, and no
// zone names more copies of a card than the player has.
func TestZonesAddUpToDecks(t *testing.T) {
	c := loadClient(t)
	for _, lp := range corpus(t) {
		b, _ := os.ReadFile(lp)
		g, err := c.Parse(string(b), nil)
		if err != nil {
			t.Fatal(err)
		}
		id := filepath.Base(lp)
		for _, tu := range g.Turns() {
			for p, d := range tu.Decks {
				owned, in := 0, 0
				for _, k := range d {
					owned += k
				}
				known := Deck{}
				for zone, cs := range tu.Zones[p] {
					in += cs.Size()
					for n, k := range cs.Known {
						if k < 0 {
							t.Errorf("%s turn %d: %s has %d %s in %s", id, tu.Number, p, k, n, zone)
						}
						known[n] += k
					}
				}
				if in != owned {
					t.Errorf("%s turn %d: %s's zones hold %d cards, they own %d", id, tu.Number, p, in, owned)
				}
				for n, k := range known {
					if k > d[n] {
						t.Errorf("%s turn %d: %s's zones name %d %s, they own %d", id, tu.Number, p, k, n, d[n])
					}
				}
			}
		}
	}
}

// Cards must be where their lines take them from. The log doesn't always say
// where a card comes from, so a few guesses go wrong; this keeps them rare.
// A player whose draws are all named is checked on its own: for that seat the
// replay knows every card.
func TestZonesFollowTheLog(t *testing.T) {
	c := loadClient(t)
	events, named, all := 0, 0, 0
	var worst []string
	for _, lp := range corpus(t) {
		b, _ := os.ReadFile(lp)
		g, err := c.Parse(string(b), nil)
		if err != nil {
			t.Fatal(err)
		}
		hidden := map[string]bool{}
		for _, e := range g.Events {
			for _, cn := range e.Args[1].Cards {
				if drawnT[e.Template] && cn.Card == nil {
					hidden[e.Player] = true
				}
			}
		}
		events += len(g.Events)
		for _, m := range g.ZoneMisses() {
			all++
			if !hidden[m.Player] {
				named++
				if len(worst) < 10 {
					worst = append(worst, fmt.Sprintf("%s line %d: %s %s (cause %q) wanted in %s, found in %q", filepath.Base(lp), m.Line, m.Template, m.Card, m.Cause, m.Want, m.Found))
				}
			}
		}
	}
	t.Logf("%d events: %d cards out of place, %d of them for seats whose cards are all named", events, all, named)
	if named*1000 > events || all*1000 > 3*events {
		t.Errorf("too many cards out of place: %d (%d for fully named seats) in %d events; first:\n%s", all, named, events, strings.Join(worst, "\n"))
	}
}

// A seat whose draws are named ends each turn holding exactly what its Clean-up drew.
func TestNextHandIsTheCleanupDraw(t *testing.T) {
	c := loadClient(t)
	checked, wrong := 0, 0
	for _, lp := range corpus(t) {
		b, _ := os.ReadFile(lp)
		g, err := c.Parse(string(b), nil)
		if err != nil {
			t.Fatal(err)
		}
		draws := map[int]Deck{} // turn index -> the named cards of its Clean-up draw
		turn, kept := -1, map[int]bool{}
		for _, e := range g.Events {
			if _, drew := draws[turn]; drew && e.Template != "NEW_TURN" && e.Player == e.TurnOf {
				kept[turn] = true // something after the draw (Save puts its card into hand)
			}
			switch {
			case e.Template == "NEW_TURN":
				turn++
			case e.Template == "KEEPS_IN_HAND" || e.Template == "SET_ASIDE_WITH" && strings.Contains(e.Args[2].Text, "Biding Time"):
				kept[turn] = true
			case e.Template == "DRAW" && e.Level == 0 && e.Player == e.TurnOf && turn >= 0:
				d := Deck{}
				for _, cn := range e.Args[1].Cards {
					if cn.Card == nil {
						d = nil
						break
					}
					d[cn.Card.Name] += cn.N
				}
				draws[turn] = d
			}
		}
		for i, tu := range g.Turns() {
			want, ok := draws[i]
			if !ok || want == nil || kept[i] {
				continue
			}
			checked++
			got := Deck{}
			if h := tu.Zones[tu.Player][ZoneHand]; h != nil {
				got = h.Known
			}
			if !reflect.DeepEqual(got, want) {
				wrong++
				if wrong <= 5 {
					t.Logf("%s turn %d (%s): hand %v, Clean-up drew %v", filepath.Base(lp), tu.Number, tu.Player, got, want)
				}
			}
		}
	}
	t.Logf("%d turns checked, %d wrong", checked, wrong)
	if wrong*100 > checked {
		t.Errorf("%d of %d next hands are wrong", wrong, checked)
	}
}

// Game 184518886 ended 9 to 6 by DomBot's !game: six Estates and a Duchy against a Province.
func TestScoreMatchesDomBot(t *testing.T) {
	c := loadClient(t)
	b, err := os.ReadFile("../../testdata/logs/184518886.log")
	if err != nil {
		t.Fatal(err)
	}
	g, _ := c.Parse(string(b), map[string]string{"Lord Rattington": "UtagawaLk"})
	vp := c.Score(g, ScoreInput{})
	if vp["gamrevinu"].Total() != 9 || vp["UtagawaLk"].Total() != 6 {
		t.Errorf("scores %v and %v, DomBot says 9 and 6", vp["gamrevinu"], vp["UtagawaLk"])
	}
}

// The loading account sits in seat 1 (the first original player's), the bot in seat 2.
func TestSeatNames(t *testing.T) {
	for _, c := range []struct {
		log, original []string
		want          map[string]string
	}{
		{[]string{"gamrevinu", "Lord Rattington"}, []string{"gamrevinu", "Wtrrch"}, map[string]string{"gamrevinu": "gamrevinu", "Lord Rattington": "Wtrrch"}},
		{[]string{"gamrevinu", "Lord Rattington"}, []string{"Nikhec", "gamrevinu"}, map[string]string{"gamrevinu": "Nikhec", "Lord Rattington": "gamrevinu"}},
		{[]string{"gamrevinu", "Lord Rattington"}, []string{"agoo03", "jers"}, map[string]string{"gamrevinu": "agoo03", "Lord Rattington": "jers"}},
		{[]string{"gamrevinu", "Lord Rattington"}, []string{"agoo03"}, nil},
	} {
		if got := SeatNames(c.log, c.original); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SeatNames(%v, %v) = %v, want %v", c.log, c.original, got, c.want)
		}
	}
}
