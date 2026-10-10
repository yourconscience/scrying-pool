package main

import (
	"os"
	"reflect"
	"testing"
)

func loadTestGame(t *testing.T) *Game {
	t.Helper()
	b, err := os.ReadFile("testdata/logs/184518886.log")
	if err != nil {
		t.Fatal(err)
	}
	g, err := parseGameLog(string(b), map[string]string{"Lord Rattington": "UtagawaLk"})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// Game 184518886, gamrevinu vs UtagawaLk, 10-Oct-2026. The expected decks are
// the per-player counts in DomBot's !game picture for that game; the scores are
// DomBot's "9 points" and "6 points".
func TestFinalDecksMatchDomBot(t *testing.T) {
	g := loadTestGame(t)
	if len(g.Unknown) != 0 {
		t.Errorf("%d unknown lines, first: %+v", len(g.Unknown), g.Unknown[0])
	}
	if !reflect.DeepEqual(g.Players, []string{"gamrevinu", "UtagawaLk"}) {
		t.Errorf("players %v", g.Players)
	}
	want := map[string]map[string]int{
		"gamrevinu": {"Hostelry": 7, "Destrier": 3, "Shaman": 3, "Groom": 7, "Estate": 6, "Duchy": 1, "Copper": 3, "Silver": 1, "Horse": 2},
		"UtagawaLk": {"Hostelry": 3, "Innkeeper": 2, "Destrier": 7, "Shaman": 3, "Menagerie": 2, "Fortress": 1, "Groom": 3, "Province": 1, "Gold": 2, "Horse": 5},
	}
	got := g.finalDecks()
	for p, deck := range want {
		if !reflect.DeepEqual(got[p], deck) {
			t.Errorf("%s deck:\n got  %v\n want %v", p, got[p], deck)
		}
	}
	for p, vp := range map[string]int{"gamrevinu": 9, "UtagawaLk": 6} {
		if got, _ := deckVP(got[p]); got != vp {
			t.Errorf("%s VP %d, want %d", p, got, vp)
		}
	}
}

func TestTurnsAndHeader(t *testing.T) {
	g := loadTestGame(t)
	last := map[string]int{}
	for _, e := range g.Events {
		if e.Verb == "turn" {
			last[e.Player] = e.Turn
		}
	}
	if last["gamrevinu"] != 14 || last["UtagawaLk"] != 13 {
		t.Errorf("last turns %v, want gamrevinu 14, UtagawaLk 13", last)
	}
	if len(g.Header) == 0 || g.Header[0] != "Game #184519122, unrated." {
		t.Errorf("header %q", g.Header)
	}
}

func TestReadCardPiece(t *testing.T) {
	for in, want := range map[string]cardN{
		"2 Coppers":     {"Copper", 2},
		"an Estate":     {"Estate", 1},
		"3 Hostelries":  {"Hostelry", 3},
		"2 Fortresses":  {"Fortress", 2},
		"Sinister Plot": {"Sinister Plot", 1},
		"5 cards":       {"", 5},
		"2 other cards": {"", 2},
		"a card":        {"", 1},
	} {
		if got, ok := readCardPiece(in); !ok || got != want {
			t.Errorf("%q -> %+v (%v), want %+v", in, got, ok, want)
		}
	}
}
