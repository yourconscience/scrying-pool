# Battles on the Scout page (design)

Back to the [README](../README.md).

Status: built, on top of the Scout page (PR #40). Feedback welcome on the ending labels and the deck grouping.

After scouting a player you want to look at their games one by one, against anyone: "yikyak against Lynet": what the kingdom was, who won and by how much, how long it ran, how it ended, and what both decks looked like at the end. No turn by turn. This is the Battles view of the Scout page.

## What the page shows

After picking a scouted player the page shows two views, switched by two buttons next to the list of scouted players: **Analysis** (the matchup dashboard, shown first; described below) and **Battles**. Only one is visible at a time, so the long list never pushes the report off screen. A link with `?player=NAME&vs=OPPONENT` opens on Battles with the filter filled.

Battles has a filter by opponent (type a few letters, pick a name from the list, or press the button for your own name to isolate the head-to-head), then one card per game, newest first. A game is one of the up to 100 two-player games against people that the scout fetch lists for the player (`games` in `scout-<player>.json`). Cards show:

- Result for the scouted player (won, lost or tied), the score (34–28), both players' levels then, the date, and who went first.
- How many turns, and how it ended: Provinces, Colonies, three piles or a resignation.
- The kingdom: the cards and the landscapes, as images when `card-images/` has them, else as names.
- Both final decks, as counts grouped Victory, Treasure, Action and Curse, largest first.
- A link to the analysis page for the game.

Only games with a saved log are listed at first. The listed games without one (most of them, until the scout run has loaded them) wait behind a button that carries their count; they show what the list knows (date, players, levels, score) marked "log not loaded", and no first player, turns, ending, kingdom or decks. The note line always says how many games are listed and how many have a log. The filter applies to whichever set is shown; when the typed name matches only games without a log, those are shown rather than an empty list.

On a phone it is one column, with the decks stacked and the buttons sized to tap.

## The Analysis view

A matchup dashboard that fits one screen at 1280x800 and stacks into one column on a phone. It is drawn from the report as data: `GET /api/scout?player=` returns `report` (the `scoutReport` that `scry scout -json` prints) next to `markdown` and `battles`. No new endpoint and no new counting. The same colours, type, panels and ECharts version as the Profiles page; the scouted player is blue and you are amber, which mean no better or worse (green and orange are kept for that).

| Block | What it shows | Report fields |
|---|---|---|
| Matchup strip | level and rank of both, the expected score as one split bar, the head-to-head record, recent form: their win rate over the scouted games and yours over the last 90 days, each with its game count | `dominion_games.summary`, `.my_summary` (level, rank), `.headToHead` (record, `expected`), `them.record`, `me.record`, `them.games`, `me.games` |
| Where they differ | up to four findings as sentences with their numbers ("Opened with a trasher: won 10 of 12, otherwise 3 of 11"), the gap in points as a badge; the same splits the Markdown's "Where X differs most" lists (4 games a side, 20 points), the strongest per theme, those with 8 games or more on both sides first; a side under 8 games is marked "few games" | `them.seat`, `.stronger`, `.opened_as`, `.styles`, `.kingdoms` |
| How they open | win rate by how the first two turns went (two kingdom cards, a card and Silver, ...), both players; the kingdom cards they open with most | `shapes`, `opened` |
| What they build | the games per style (of their games) and the win rate with it, both players | `styles` |
| How games end and run | win rate by ending and by length, both players | `endings`, `length` |
| Kingdom cards | the cards they take most and leave alone, as chips with the games (taken or left alone / in the kingdom), with art where `card-images/` has it | `took`, `skipped`, `battles.images` |

Each bar is a win rate with its 80% range as a line and its count as text ("64%  7/11", won of played) at the right, so a phone needs no hover; the tooltip has the same. Rows under 5 games, and cards that were in fewer than 3 kingdoms, are drawn faded. A 50% line marks even. The "Full report" button under the blocks opens the Markdown tables, collapsed by default.

Not on it: the dominion.games game list, turns of the first Gold and Province, margin and the per-card win rates (all in the full report).

## Where each field comes from (Battles)

| Field | Source |
|---|---|
| Games, date, players, levels, score, result | the player's `scout-<player>.json` (`scoutGame`), so unsaved games have them too |
| First player, turns, ending | `advisor.Features` of the saved game, as the Last games page uses them; turns are the larger of the two players' turns |
| Colonies | the replayed supply (`Game.Supply`): Colony left at most 1 while Province is not |
| Kingdom, landscapes | `Features.Kingdom`, `Features.Landscapes` (the score file's `# kingdom`, `# landscapes`) |
| Final decks | `Game.FinalDecks()` of the saved log, as `scry review` prints them |
| Deck groups | the card's types in the client tables (`Card.Types`) |
| Images | `/card-image/NAME`, the route the Profiles page uses |

No new endpoint: `GET /api/scout?player=` answers with a `battles` field next to `markdown`. The page code is `scout.js` and `scout.css`; the server side is `battles.go`.

## Questions the design had to settle

**Where the final decks come from.** The saved `ID.score.txt` holds both players' whole final decks, but it is not the real final state: the capture resigns or lets the bot answer after the last loaded decision, and the score screen counts those moves. The log replay stops at the loaded decision. Over the 580 games saved when this was written: the log plus the bot's lines gives the score screen's decks in 579 (the exception, 185118440, has 1 Estate against the score screen's 7), so both decks are fully named in every game, from one seat too, since gains are public. The log alone reproduces the listed final scores in 454 games and the log with the bot's lines in 407: in 70 games the bot's moves spoil a log that was right, in 23 they complete one that wasn't. So the page uses the log replay and says when it can't vouch for it: `complete` is `Features.Complete`, the computed VP of both players equal the listed scores. Where they don't (103 of 580; 12 of Flodominion's 25 games), a late move is missing or a VP source isn't counted, and the card says "the deck may be a card short". Games that load from both seats would not change this.

**The ending.** `Features.Ending` is `provinces`, `piles` or `resign`, by the markus rule in `advisor/features.go`: at most one Province or Colony left is `provinces`, at most one card from three empty piles is `piles`, anything else is `resign`. So `provinces` covers Colony; the page separates them with the supply: Colony left at most 1 and Province not. If both piles are nearly gone it says Provinces. Colony was in 18 saved games and ran out in 8. The page can say a game ended on Provinces, Colonies or piles. It cannot say who resigned: that isn't saved. `resign` is also what a timeout, a disconnect or a log missing its last two moves would look like, and some resignations read as ties (docs/position-analysis.md on branch position-eval). The score screen's first line, "You Resigned" in 444 of 580 games, is the capture's own resignation and says nothing about the game.

**Reuse.** Reused: `scoutData`/`scoutGame`, `loadCorpus`, `webClient`, `advisor.LoadGames`, `Features`, `FinalDecks`, `/card-image/`, the `analyze.html#ID` link and the Last games page's wording for endings. New: one file of Go (`battles.go`, about 120 lines), the views and the Battles panel in `scout.html`, `scout.js`, `scout.css`.

## Not in this change

- Turn by turn detail, draw luck, openings (the Last games page and the analysis page have them).
- Who resigned, and reading resignations out of the scores.
- Loading the missing logs from the Battles panel: the Scout run does that.
- A kingdom for an unsaved game: the list doesn't carry it.

## Known gaps

- A player with a fetch but no saved game gets the Scout page's existing error, not a list of stubs.
- `complete` compares VP only: a missing late move that changes no VP (a Copper, say) is not caught, so a deck can be a card short with the warning off.
- Dates are the list's UTC day.
- The Analysis view was looked at in a headless browser at 1280x800 and 390 px wide, not on a phone; dark mode and touch have not been checked.
