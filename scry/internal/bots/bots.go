// Package bots has simple agents for the engine: a random player for fuzzing
// and Big Money variants as baselines. A learned network replaces them.
package bots

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/yourconscience/scrying-pool/scry/internal/engine"
)

// New makes an agent from a name: "random", "bm", or "bm+Smithy".
func New(spec string, t *engine.Table, seed uint64) (engine.Agent, error) {
	switch {
	case spec == "random":
		return &Random{R: rand.New(rand.NewPCG(seed, 1))}, nil
	case spec == "bm":
		return &Money{}, nil
	case strings.HasPrefix(spec, "bm+"):
		d, ok := t.Card(spec[3:])
		if !ok || d.Rules == nil || d.Kind != "kingdom" {
			return nil, fmt.Errorf("%s: no playable kingdom card %q", spec, spec[3:])
		}
		return &Money{X: d.ID, HasX: true}, nil
	}
	return nil, fmt.Errorf("unknown bot %q: want random, bm or bm+<card>", spec)
}

// Random picks a random legal answer.
type Random struct{ R *rand.Rand }

func (b *Random) Decide(v *engine.View, d *engine.Decision) []int {
	n := d.Min + b.R.IntN(d.Max-d.Min+1)
	return b.R.Perm(len(d.Options))[:n]
}

// Money is Big Money: Province at $8, Gold at $6, Silver at $3, greening late;
// with X, it also buys X while it has few and plays it.
type Money struct {
	X    engine.CardID
	HasX bool
}

// nonTerminal lists cards that give +Actions, played before the others.
var nonTerminal = map[string]bool{"Cellar": true, "Harbinger": true, "Merchant": true, "Village": true,
	"Poacher": true, "Festival": true, "Laboratory": true, "Market": true, "Sentry": true,
	"Pawn": true, "Shanty Town": true, "Conspirator": true, "Mining Village": true, "Great Hall": true, "Nobles": true,
	"Lurker": true, "Diplomat": true, "Mill": true, "Secret Passage": true, "Upgrade": true, "Scout": true,
	"Bazaar": true, "Caravan": true, "Fishing Village": true, "Haven": true, "Lighthouse": true, "Lookout": true,
	"Native Village": true, "Pearl Diver": true, "Sailor": true, "Sea Chart": true, "Tide Pools": true,
	"Treasury": true, "Warehouse": true,
	"City": true, "Grand Market": true, "Peddler": true, "Worker's Village": true,
	"Border Village": true, "Cartographer": true, "Crossroads": true, "Highway": true, "Inn": true, "Oasis": true,
	"Scheme": true, "Trail": true, "Wheelwright": true}

// price is a card's cost as one number: a Potion counts $2, debt $1.
func price(d *engine.Def) int { return d.Cost.Coins + 2*d.Cost.Potion + d.Cost.Debt }

// worth orders cards for keeping: Curses and Victory cards last, Copper next.
func worth(d *engine.Def) int {
	switch {
	case d.Name == "Curse":
		return -3
	case d.Victory() && !d.Action() && !d.Treasure():
		return -2
	case d.Name == "Copper":
		return 0
	}
	return price(d) + 1
}

func junk(d *engine.Def) bool { return worth(d) < 0 }

// ranked lists option indexes, the best first.
func ranked(v *engine.View, d *engine.Decision, score func(*engine.Def) int) []int {
	idx := make([]int, len(d.Options))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool {
		return score(v.Def(d.Options[idx[i]].Card)) > score(v.Def(d.Options[idx[j]].Card))
	})
	return idx
}

// mineFirst ranks what Mine trashes: Silver into Gold, then Copper into Silver.
func mineFirst(x *engine.Def) int {
	switch x.Name {
	case "Silver":
		return 2
	case "Copper":
		return 1
	}
	return 0
}

// take answers with the first n ranked options, at least Min.
func take(d *engine.Decision, idx []int, n int) []int {
	n = max(n, d.Min)
	return idx[:min(n, d.Max, len(idx))]
}

func count(idx []int, d *engine.Decision, v *engine.View, ok func(*engine.Def) bool) int {
	n := 0
	for _, i := range idx {
		if ok(v.Def(d.Options[i].Card)) {
			n++
		}
	}
	return n
}

func (b *Money) Decide(v *engine.View, d *engine.Decision) []int {
	neg := func(f func(*engine.Def) int) func(*engine.Def) int { return func(x *engine.Def) int { return -f(x) } }
	switch d.Kind {
	case engine.PlayAction:
		return take(d, ranked(v, d, func(x *engine.Def) int {
			if nonTerminal[x.Name] {
				return 100 + price(x)
			}
			return price(x)
		}), 1)
	case engine.PlayTreasures:
		all := make([]int, len(d.Options))
		for i := range all {
			all[i] = i
		}
		return all
	case engine.Buy:
		return b.buy(v, d)
	case engine.ToGain:
		greenOK := v.Left(v.Table().MustCard("Province")) <= 3
		return take(d, ranked(v, d, func(x *engine.Def) int {
			if x.Name == "Curse" || x.Victory() && !greenOK && x.Name != "Province" {
				return -1
			}
			return price(x)
		}), 1)
	case engine.ToTrash:
		idx := ranked(v, d, neg(worth))
		if v.Def(d.Source).Name == "Mine" || v.Def(d.Source).Name == "Moneylender" {
			if v.Def(d.Source).Name == "Mine" {
				idx = ranked(v, d, mineFirst)
			}
			return take(d, idx, 1)
		}
		if v.Def(d.Source).Name == "Remodel" && v.Left(v.Table().MustCard("Province")) <= 4 {
			for i, o := range d.Options {
				if v.Def(o.Card).Name == "Gold" {
					return []int{i}
				}
			}
		}
		return take(d, idx, count(idx, d, v, junk))
	case engine.ToDiscard:
		idx := ranked(v, d, neg(worth))
		return take(d, idx, count(idx, d, v, junk))
	case engine.ToTopdeck, engine.ToOrder, engine.ToPlay:
		idx := ranked(v, d, worth)
		if d.Kind == engine.ToTopdeck && d.Min == 0 {
			return take(d, idx, count(idx, d, v, func(x *engine.Def) bool { return worth(x) > 1 }))
		}
		return take(d, idx, len(idx))
	case engine.ToSetAside:
		if v.Actions() == 0 {
			return []int{0}
		}
		return nil
	case engine.ToReveal, engine.ToCall:
		return take(d, []int{0}, 1)
	case engine.ToChoose:
		idx := make([]int, len(d.Options)) // the last modes: "yes", the biggest number
		for i := range idx {
			idx[i] = len(idx) - 1 - i
		}
		return take(d, idx, 1)
	case engine.UseCoffers:
		return []int{0}
	case engine.PlayNight:
		return take(d, []int{0}, 1)
	case engine.ToExile, engine.ToPass, engine.ToReturn:
		idx := ranked(v, d, neg(worth))
		return take(d, idx, d.Min)
	}
	return take(d, ranked(v, d, worth), d.Min)
}

func (b *Money) buy(v *engine.View, d *engine.Decision) []int {
	t := v.Table()
	coins, provinces := v.Coins(), v.Left(t.MustCard("Province"))
	want := func(name string) []int {
		for i, o := range d.Options {
			if v.Def(o.Card).Name == name {
				return []int{i}
			}
		}
		return nil
	}
	pick := func(names ...string) []int {
		for _, n := range names {
			if got := want(n); got != nil {
				return got
			}
		}
		return nil
	}
	if b.HasX {
		x := v.Def(b.X)
		limit := 1 + total(v.Owned(v.Me()))/15
		if nonTerminal[x.Name] {
			limit = 99
		}
		if v.Owned(v.Me())[b.X] < min(limit, 3) || nonTerminal[x.Name] {
			if c := v.Cost(b.X); c <= coins && coins < 8 && (coins < 6 || c >= 6) {
				for i, o := range d.Options {
					if o.Card == b.X {
						return []int{i}
					}
				}
			}
		}
	}
	switch {
	case coins >= 8:
		return pick("Province", "Gold")
	case coins >= 6 && provinces <= 4:
		return pick("Duchy", "Gold")
	case coins >= 6:
		return pick("Gold", "Silver")
	case coins == 5 && provinces <= 5:
		return pick("Duchy", "Silver")
	case coins >= 3 && provinces <= 2:
		return pick("Estate", "Silver")
	case coins >= 3:
		return pick("Silver")
	case coins == 2 && provinces <= 3:
		return pick("Estate")
	}
	return nil
}

func total(m map[engine.CardID]int) int {
	n := 0
	for _, k := range m {
		n += k
	}
	return n
}
