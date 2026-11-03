//go:build !js

package main

import (
	"fmt"
	"slices"
	"strings"
)

// mergeSeats combines a game's logs from its two seats into one that reads as the
// first seat's. A log names only the cards its own seat can see: to it, the other
// player "draws 5 cards". The two logs are the same game line by line, so each line
// of the second seat's player comes from its own log, under the first log's names.
// It returns how many lines that changed.
func mergeSeats(first, second string) (merged string, named int, err error) {
	a, b := strings.Split(first, "\n"), strings.Split(second, "\n")
	sa, sb := seatNames(a), seatNames(b)
	if sa == nil || sb == nil {
		return "", 0, fmt.Errorf("can't tell the seats from a log's opening lines")
	}
	if sa[0] != sb[1] || sa[1] != sb[0] {
		return "", 0, fmt.Errorf("the second log isn't from the other seat: seats %v and %v", sa, sb)
	}
	out := slices.Clone(a)
	for i := 0; i < min(len(a), len(b)); i++ {
		ka, ta, _ := strings.Cut(a[i], "\t")
		kb, tb, _ := strings.Cut(b[i], "\t")
		if ka != kb {
			return "", 0, fmt.Errorf("the logs part at line %d: %q and %q", i+1, a[i], b[i])
		}
		fa, fb := strings.Split(ta, "|"), strings.Split(tb, "|")
		if len(fa) < 2 || (fa[0] != sa[0] && fa[0] != sa[1]) {
			continue
		}
		if len(fb) < 2 || fb[1] != fa[1] || (fa[0] == sa[0]) != (fb[0] == sb[0]) {
			return "", 0, fmt.Errorf("the logs part at line %d: %q and %q", i+1, a[i], b[i])
		}
		if fa[0] == sa[1] {
			for j, f := range fb { // the same player has the other name in the other log
				switch f {
				case sb[0]:
					fb[j] = sa[0]
				case sb[1]:
					fb[j] = sa[1]
				}
			}
			if line := ka + "\t" + strings.Join(fb, "|"); line != a[i] {
				out[i] = line
				named++
			}
		}
	}
	return strings.Join(out, "\n"), named, nil
}

// seatNames reads a log's short names for the first and second seat, from the
// lines that say what each player starts with.
func seatNames(lines []string) []string {
	var seats []string
	for _, l := range lines {
		_, text, _ := strings.Cut(l, "\t")
		if name, rest, ok := strings.Cut(text, "|"); ok && strings.HasPrefix(rest, "starts with|") && !slices.Contains(seats, name) {
			seats = append(seats, name)
		}
	}
	if len(seats) != 2 {
		return nil
	}
	return seats
}
