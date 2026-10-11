//go:build !js

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourconscience/scrying-pool/cartographer"
)

func TestParseSampleChat(t *testing.T) {
	c, err := parseChat(sampleChat)
	if err != nil {
		t.Fatal(err)
	}
	if c.MatchID != "8605" || c.Players != [2]string{"Lynet", "gamrevinu"} {
		t.Fatalf("match %q players %v", c.MatchID, c.Players)
	}
	var winners []string
	n := 0
	for _, g := range c.Games {
		winners = append(winners, g.Winner)
		n += len(g.Lines)
	}
	if got := strings.Join(winners, ","); got != "Lynet,gamrevinu,Lynet,gamrevinu,gamrevinu,gamrevinu" {
		t.Fatalf("winners %s", got)
	}
	if n != 366 || len(c.Tail) != 0 {
		t.Fatalf("got %d chat lines + %d tail, want 366 + 0", n, len(c.Tail))
	}
	// players' gg after a result stays with that game; spectators' talk moves on
	g1 := c.Games[0].Lines
	if last := g1[len(g1)-1]; last.Who != "Lynet" || last.Text != "gg" {
		t.Fatalf("game 1 ends with %+v", last)
	}
}

func TestParseSampleSummary(t *testing.T) {
	s, err := parseSummary(sampleDomBot)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Games) != 6 || s.Begin != "October 2, 2026 at 8:00 PM" || s.Total != "2 hours, 15 minutes" {
		t.Fatalf("%d games, begin %q, total %q", len(s.Games), s.Begin, s.Total)
	}
	g4 := s.Games[3]
	if g4.Points["Lynet"] != -2 || g4.End != "Lynet resigned" || g4.ID != "184205175" || len(g4.Cards) != 13 || g4.Empty != "Daimyo" {
		t.Fatalf("game 4: %+v", g4)
	}
}

func TestNamesWithSpaces(t *testing.T) {
	chat := "Match #1 created\nFinicky Villager: glhf\nMatch: gamrevinu 1, Finicky Villager 0\nFinicky Villager: gg\nMatch completed: gamrevinu 1, Finicky Villager 1 — g, F\n"
	bot := "gamrevinu v. Finicky Villager\n1–1\n" +
		"gamrevinu: 30, Finicky Villager: 20 (Provinces, 15.5 turns)\n1 (20m) — Moat, Witch — Empty piles: Province\n" +
		"Finicky Villager: 25, gamrevinu: 12 (Resigned, 12 turns)\n2 (10m) — Village, Smithy\n"
	res, err := process(chat, bot, false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	md := res.Markdown
	for _, want := range []string{
		"# gamrevinu vs Finicky Villager: 1-1",
		"| 2 | Finicky Villager | 12 | 25 | gamrevinu resigned | 12 | 10m | `2` |",
		"- **Finicky Villager:** gg",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if len(res.Warnings) != 0 {
		t.Errorf("warnings: %v", res.Warnings)
	}
}

func TestAlignSkipsOlderGames(t *testing.T) {
	c, _ := parseChat("Match: A 1, B 0\nMatch: A 1, B 1\n")
	s := &Summary{Games: []gameResult{{Winner: "B", ID: "old"}, {Winner: "A", ID: "1"}, {Winner: "B", ID: "2"}}}
	got, ok := align(c, s)
	if !ok || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("align = %v %v", got, ok)
	}
	if _, ok := align(c, &Summary{Games: []gameResult{{Winner: "B"}, {Winner: "B"}}}); ok {
		t.Fatal("aligned mismatched winners")
	}
	if r := buildReport(c, &Summary{Games: []gameResult{{Winner: "B"}}}); len(r.Warnings) != 1 {
		t.Fatalf("want a mismatch warning, got %v", r.Warnings)
	}
}

func TestChatIsEscaped(t *testing.T) {
	c, _ := parseChat("x: ***breaths sigh*** <script>alert(1)</script> a|b\nMatch: A 1, B 0\n")
	md := buildReport(c, nil).Markdown
	if !strings.Contains(md, `- x: \*\*\*breaths sigh\*\*\* &lt;script&gt;alert(1)&lt;/script&gt; a\|b`) {
		t.Fatalf("not escaped:\n%s", md)
	}
}

func TestWebFormatAndCSRF(t *testing.T) {
	h := newHandler(time.Second)
	post := func(origin, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/format", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"chat":"Match: A 1, B 0","dombot":""}`
	if w := post("http://127.0.0.1:8765", body); w.Code != 200 || !strings.Contains(w.Body.String(), `"source":"none"`) {
		t.Fatalf("same-origin: %d %s", w.Code, w.Body)
	}
	if w := post("https://evil.example", body); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin: %d", w.Code)
	}
	if w := post("", `{"chat":"no results here"}`); w.Code != 422 {
		t.Fatalf("bad chat: %d", w.Code)
	}
	for path, want := range map[string]string{"/": "Scrying Pool", "/notes.html": "Dominion Match Notes", "/analyze.html": "Analyze a Game"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

// The Scry game bookmark posts a game from the dominion.games tab; the analysis
// page then reads it back turn by turn.
func TestWebGames(t *testing.T) {
	cpath := cartographer.DefaultClientPath()
	if cpath == "" {
		t.Skip("no exported dominion.games client tables")
	}
	t.Setenv("SCRY_CLIENT", cpath)
	t.Setenv("HOME", t.TempDir()) // the saved games go to a temporary user config folder
	t.Setenv("XDG_CONFIG_HOME", "")
	h := newHandler(time.Second)
	call := func(method, path, origin, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8765"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "text/plain")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	log, _ := os.ReadFile("testdata/logs/184426907.log")
	saved, _ := os.ReadFile("testdata/logs/184426907.score.txt")
	var score []string // the score screen as the capture copies it, without the lines import adds
	for _, l := range strings.Split(string(saved), "\n") {
		if !strings.HasPrefix(l, "# ") {
			score = append(score, l)
		}
	}
	rec, _ := json.Marshal(exportedGame{ID: "184426907", Original: []string{"gamrevinu", "GarlicRamen"}, Log: string(log), Score: strings.Join(score, "\n")})
	if w := call("GET", "/api/games/184426907", "", ""); w.Code != 404 {
		t.Fatalf("unsaved game: %d %s", w.Code, w.Body)
	}
	if w := call("POST", "/api/games", "https://evil.example", string(rec)); w.Code != 403 {
		t.Fatalf("cross-origin post: %d", w.Code)
	}
	if w := call("POST", "/api/games", "", `{"id":"../x","log":""}`); w.Code != 400 {
		t.Fatalf("bad id: %d", w.Code)
	}
	w := call("POST", "/api/games", "https://dominion.games", string(rec))
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "https://dominion.games" {
		t.Fatalf("post from dominion.games: %d %s", w.Code, w.Body)
	}
	w = call("GET", "/api/games/184426907", "", "")
	var v struct { // the fields the test reads; cards in turns are written as names
		Players []string
		FinalVP map[string]cartographer.VPBreakdown `json:"final_vp"`
		Setup   []logLine
		Turns   []struct {
			VP  map[string]int
			Log []logLine
		}
		Cards map[string]Card
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || w.Code != 200 {
		t.Fatalf("game view: %d %v", w.Code, err)
	}
	if len(v.Turns) == 0 || len(v.Turns) != len(turnEnds(mustParse(t, string(log)))) {
		t.Fatalf("%d turns in the view", len(v.Turns))
	}
	last := v.Turns[len(v.Turns)-1]
	for _, p := range v.Players {
		if last.VP[p] != v.FinalVP[p].Total() {
			t.Errorf("%s: %d VP after the last turn, %d final", p, last.VP[p], v.FinalVP[p].Total())
		}
	}
	lines := len(v.Setup)
	for _, t := range v.Turns {
		lines += len(t.Log)
	}
	if want := strings.Count(string(log), "\n") - 2; lines < want*9/10 {
		t.Errorf("only %d log lines in the view", lines)
	}
	if _, ok := v.Cards["Copper"]; !ok {
		t.Error("the view doesn't describe Copper")
	}
}

func mustParse(t *testing.T, log string) *cartographer.Game {
	c, err := loadClient()
	if err != nil {
		t.Fatal(err)
	}
	g, err := c.Parse(log, nil)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// A kingdom read later is written into the saved game's score file, replacing an old one.
func TestAddKingdom(t *testing.T) {
	dir := t.TempDir()
	score := "# row: 2026-Oct-09 | Nikhec: 23, gamrevinu: 32\n# original players: Nikhec / gamrevinu\n# kingdom: Old\nYou Win\n"
	if err := os.WriteFile(filepath.Join(dir, "184479482.score.txt"), []byte(score), 0o644); err != nil {
		t.Fatal(err)
	}
	k := map[string][]string{"Cards in Supply": {"Patron", "Smithy"}, "Events, Landmarks, Projects, Ways, Allies, Traits, and Prophecies": {"Rice"}}
	if err := addKingdom(dir, "184479482", k); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "184479482.score.txt"))
	want := "# row: 2026-Oct-09 | Nikhec: 23, gamrevinu: 32\n# original players: Nikhec / gamrevinu\n# kingdom: Patron, Smithy\n# landscapes: Rice\nYou Win\n"
	if string(b) != want {
		t.Fatalf("score file:\n%s\nwant:\n%s", b, want)
	}
	if err := addKingdom(dir, "../x", k); err == nil {
		t.Fatal("a bad id was accepted")
	}
	if got, ok := rowScores(scoreHeader(want, "row")); !ok || got["Nikhec"] != 23 || got["gamrevinu"] != 32 {
		t.Fatalf("row scores %v", got)
	}
	if got, ok := resultScores("R4D4: ?5, gamrevinu: 9"); !ok || got["R4D4"] != -5 {
		t.Fatalf("DomBot minus: %v", got)
	}
}
