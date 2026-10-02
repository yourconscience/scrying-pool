package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Dominion Match Notes") {
		t.Fatalf("index: %d", w.Code)
	}
}
