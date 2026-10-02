//go:build !js

package main

import (
	"net/http/httptest"
	"testing"
)

func TestSampleCardsAreAllKnown(t *testing.T) {
	s, err := parseSummary(sampleDomBot)
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range s.Games {
		k := classify(g.Cards)
		if len(k.Unknown) != 0 || len(k.Cards) != 10 {
			t.Errorf("game %d: %d kingdom piles, unknown %v", i+1, len(k.Cards), k.Unknown)
		}
	}
	g1 := classify(s.Games[0].Cards)
	var lands []string
	for _, c := range g1.Landscapes {
		lands = append(lands, c.Name+"/"+c.Kind)
	}
	if got := fmtList(lands); got != "Raid/event, Obelisk/landmark, Cave Dwellers/ally" {
		t.Errorf("game 1 landscapes: %s", got)
	}
}

func TestLookupAndCosts(t *testing.T) {
	for name, want := range map[string]string{
		"Encampment/Plunder": "kingdom $2",
		"King’s Court":       "kingdom $7",
		"king's court":       "kingdom $7",
		"Daimyo":             "kingdom 6D",
		"Alchemist":          "kingdom $3P",
		"Way of the Owl":     "way ",
	} {
		c, ok := lookupCard(name)
		if got := c.Kind + " " + costLabel(c); !ok || got != want {
			t.Errorf("%s: %q (found %v), want %q", name, got, ok, want)
		}
	}
	if _, ok := lookupCard("Not A Card"); ok {
		t.Error("found a card that doesn't exist")
	}
}

func TestArtRouteOnlyServesCardArt(t *testing.T) {
	t.Setenv("DOMBOT_ART_DIR", t.TempDir())
	h := newHandler(0)
	for _, path := range []string{"/art/..%2F..%2Fetc%2Fpasswd", "/art/NotACardArt.jpg"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Errorf("%s: %d, want 404", path, w.Code)
		}
	}
}

func fmtList(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}
