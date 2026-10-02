package main

import (
	"fmt"
	"testing"
	"time"
)

// Synthetic fixture: DomBot's real markdown is not known yet; this mirrors what
// the Discord app shows (bold score lines, underlined winner, italic game id).
func TestRenderMatchEmbed(t *testing.T) {
	tbilisi := time.FixedZone("+04", 4*3600)
	start := time.Date(2026, 10, 2, 20, 0, 0, 0, tbilisi).Unix()
	e := discordEmbed{
		Title: "gamrevinu v. Lynet",
		Description: fmt.Sprintf("4–2\nGames beginning <t:%d:f>\nTotal time: 2 hours, 15 minutes\n"+
			"Time: <:prop_90:111> <:prop_100:222> Decisions: <:gprop_100:333> <:gprop_100:333>", start),
	}
	e.Fields = append(e.Fields, struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}{
		"**__Lynet__: 11, gamrevinu: 5 (Resigned, 16.5 turns)**",
		"*184202859 (16m)* — Artist, Cabin Boy, [King’s Court](https://wiki.dominionstrategy.com/x) — *Empty piles: Curse, Artist*",
	})
	got := renderMessages([]message{{Embeds: []discordEmbed{e}}}, tbilisi)
	want := `gamrevinu v. Lynet
4–2
Games beginning October 2, 2026 at 8:00 PM
Total time: 2 hours, 15 minutes
Time: :prop_90: :prop_100: Decisions: :gprop_100: :gprop_100:
Lynet: 11, gamrevinu: 5 (Resigned, 16.5 turns)
184202859 (16m) — Artist, Cabin Boy, King’s Court — Empty piles: Curse, Artist
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCleanKeepsUnderscoresInNames(t *testing.T) {
	for in, want := range map[string]string{
		"nave_the_muser: 3":     "nave_the_muser: 3",
		`nave\_the\_muser: 3`:   "nave_the_muser: 3",
		`**a\*b**`:              "a*b",
		"  padded  \n  lines  ": "padded\nlines",
	} {
		if got := clean(in, time.UTC); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}
