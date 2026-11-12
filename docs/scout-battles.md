# Battles on the Scout page (design)

Back to the [README](../README.md).

Status: built, on top of the Scout page (PR #40). Feedback welcome on the ending labels and the deck grouping.

After scouting a player you want to look at their games one by one, against anyone: "yikyak against Lynet": what the kingdom was, who won and by how much, how long it ran, how it ended, and what both decks looked like at the end. No turn by turn. This is the Battles panel of the Scout page.

## What the page shows

A filter by opponent (type a few letters, pick a name from the list, or press the button for your own name to isolate the head-to-head), then one card per game, newest first. A game is one of the up to 100 two-player games against people that the scout fetch lists for the player (`games` in `scout-<player>.json`).

- Result for the scouted player (won, lost or tied), the score (34–28), both players' levels then, the date, and who went first.
- How many turns, and how it ended: Provinces, Colonies, three piles or a resignation.
- The kingdom: the cards and the landscapes, as images when `card-images/` has them, else as names.
- Both final decks, as counts grouped Victory, Treasure, Action and Curse, largest first.
- A link to the analysis page for the game.

A listed game with no saved log (most of them, until the scout run has loaded them) still has its card with what the list knows: date, players, levels and score, marked "log not loaded". It has no first player, turns, ending, kingdom or decks.

On a phone it is one column, with the decks stacked.

## Where each field comes from

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

**Reuse.** Reused: `scoutData`/`scoutGame`, `loadCorpus`, `webClient`, `advisor.LoadGames`, `Features`, `FinalDecks`, `/card-image/`, the `analyze.html#ID` link and the Last games page's wording for endings. New: one file of Go (`battles.go`, about 120 lines), the Battles panel in `scout.html`, `scout.js`, `scout.css`.

## Not in this change

- Turn by turn detail, draw luck, openings (the Last games page and the analysis page have them).
- Who resigned, and reading resignations out of the scores.
- Loading the missing logs from the Battles panel: the Scout run does that.
- A kingdom for an unsaved game: the list doesn't carry it.

## Known gaps

- A player with a fetch but no saved game gets the Scout page's existing error, not a list of stubs.
- `complete` compares VP only: a missing late move that changes no VP (a Copper, say) is not caught, so a deck can be a card short with the warning off.
- Dates are the list's UTC day.
- The rendered page has not been looked at in a browser; the data was checked through the API against the saved files.
