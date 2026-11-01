# Match reports (design)

Back to the [README](../README.md).

Status: design, not built. Feedback welcome on the commands, the template and the draw-luck rule.

After a league match you want one command that loads the match's games and says, compactly: who was favoured, what each kingdom offered, what happened in each game, and whether a loss came from the draw or from the deck. Today that takes `scry match`, copying ids by hand, `scry collect ID...`, then `scry review` per game and reading three reports.

## Commands

```
scry match OPPONENT                     # ask DomBot for the match, load its games, print the report
scry games OPPONENT --ids ID[,ID...]    # the same report for games you name; no Discord
scry match P1 P2                        # unchanged: prints DomBot's !match reply
```

- **Me** is chosen as `scry scout` chooses it: `-me`, else the logged-in account, else the account that loaded most games.
- `scry match OPPONENT` sends `!match ME, OPPONENT` (the existing `askDomBot`), parses the reply with `parseSummary`, and takes the game ids from its detail lines (`185400051 (12m) — Cards...`). DomBot covers only the last day or two; outside that, use `scry games`.
- Both commands then run `collect ID...` (this branch) for the ids not saved yet, which needs the dominion.games tab, and build the report from the saved games. `-offline` skips loading and reports on what is saved.
- Flags shared with `scout`: `-dir`, `-me`, `-json`, `-o FILE`, `-min-turns`, `-budget`. One new one: `-scout N` adds the opponent's N latest games (the `scout` fetch and its Side section), off by default.
- Go's `flag` stops at the first positional argument, so `games` parses flags, takes OPPONENT, then parses the rest again. `--ids` takes a comma list; ids after it are added too, so `--ids 1 2 3` and `--ids 1,2,3` both work.

Both commands are one function, `matchReport(opponent, me string, ids []string, opt)`; `match` only adds the DomBot lookup in front. No new package.

## Where the data comes from

Everything below exists today.

| Report field | Source |
|---|---|
| Game ids, result, ending, turns, empty piles | DomBot reply (`Summary.Games`), else the saved `ID.score.txt` |
| Levels then, first player, VP, ending, my turns | `advisor.Features` per game (`MyLevel`, `OppLevel`, `First`, `MyVP`, `OppVP`, `Ending`, `Turns`) |
| Win chance per game | `advisor.WinChance(MyLevel, OppLevel)` |
| μ, φ, rank, head-to-head, expected score | `scoutData.Summary`, `MySummary`, `HeadToHead` (needs a scout fetch; omitted offline without one) |
| Kingdom, landscapes, roles present | `Features.Kingdom`, `Landscapes`, `Roles` |
| Opening, first Gold, first Province, $ per turn | `Features.Opening`, `OpeningBuys`, `FirstGold`, `FirstProvince`, `Coins` |
| My record on this kingdom's cards | `advisor.Scout(me, ...)` card takes, filtered to the kingdom |
| Decks and zones per turn | `g.Turns()`, as `scry review -json` prints them |

## Report template

Markdown by default, the same fields as JSON with `-json`. Profile information stays to one comparison table; the rest is per game.

```markdown
# gamrevinu vs heth, 2026-11-01: 1-2

|            | gamrevinu | heth  |
|------------|-----------|-------|
| Level      | 61.2      | 63.0  |
| μ / φ      | 1.62 / 0.09 | 1.80 / 0.08 |
| Head-to-head | 3-5     |       |
| Last 30 days | 54% of 120 | 58% of 90 |

Expected: 44% per game, 1.3 of 3. Actual: 1.

## Game 1, 185400051: lost 28-41, Provinces, 17 turns, heth first

Kingdom: Caravan, Tea House, ... | Landscapes: ... | Roles: village, draw, trash
Your record with these cards: Caravan 60% of 10; Tea House 40% of 5
Opening 4/3: Caravan / Silver (heth: 5/2 Witch)
First Gold t6 (heth t5), first Province t11 (heth t9)
Draw luck: -1.4 $/turn over t3-t12, 3 bad hands (heth +0.2, 1)
Turning point: t9-t11 below $5 three times; heth's Witch gave 5 Curses

## Game 2 ...

## Takeaways
- Draw or deck: game 1 draw (luck far below expectation), game 3 deck (luck normal, greening late)
- Openings that worked / didn't against this player
```

The numbers above are placeholders, not the heth match.

"Turning point" is the stretch of turns where the VP gap grew most (`scoresByTurn`, which `review` already uses), with the cards gained then. The Takeaways come from fixed rules, not free text: the draw-luck verdict per game, and openings ranked by result. Commentary beyond that stays with whoever reads the report (me or an agent), outside `scry`.

## Draw luck: were the hands bad?

The question after a loss is often "bad hands or bad deck?". The report answers it per game with one number and one verdict.

- For each of my turns from t3 to t12, **expected $** is 5 × (treasure $ in the deck ÷ cards in the deck) at the start of the turn, and **actual $** is the treasure $ in the starting hand. Draw luck is the mean of actual − expected.
- A **bad hand** is one at least $2 below expected.
- The same numbers for the opponent, so the comparison is fair.
- Verdict: *draw* when my luck is clearly below the opponent's and below zero; *deck* when it isn't, meaning the hands matched what the deck could give. The cut-offs need measuring: compute luck over the saved games and set the thresholds from that spread, not from a guess.

Limits, said in the report itself: it counts treasure only, so a deck that pays with Caravan, coin tokens or action $ looks unlucky when it isn't, and draw cards change the hand size. It needs the starting hand per turn: `Features.Opening` already reads it for turns 1 and 2, and building this starts by checking that `g.Turns()` gives it for every turn.

## Not in this change

- A web page for the report. Later, the same JSON can feed a page in `scry/web` (HTML and JS kept out of Go source).
- Free-text commentary from a model.
- Matches with more than two players.

## Build order

1. `matchReport` with the Markdown and JSON output, and `scry games OPPONENT --ids`. Check it on the three heth games.
2. `scry match OPPONENT` through DomBot.
3. Draw luck, with thresholds measured on the saved games.
4. `-scout N`.
