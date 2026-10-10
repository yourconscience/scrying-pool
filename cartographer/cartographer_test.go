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
	logs, _ := filepath.Glob("../testdata/logs/*.log")
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
	b, err := os.ReadFile("../testdata/logs/184518886.log")
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
			cs, ok := c.cardList(f.text)
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
	logs, _ := filepath.Glob("../testdata/logs/*.log")
	if dir := os.Getenv("SCRY_GAMES"); dir != "" {
		more, _ := filepath.Glob(filepath.Join(dir, "*.log"))
		logs = append(logs, more...)
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
