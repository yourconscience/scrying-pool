# Match review: what went well and what went badly (design)

Back to the [README](../README.md). It extends the [match report](match-report.md); `scry games` and the Last games page carry it.

Status: built on 2026-11-12. The rules were measured the same day over 549 saved games of one player (509 decided) with one-off scripts that aren't in the repo. The rendered page has not been seen by the author: the checks below ran the page script against real output with a stub DOM, not in a browser.

After a match you want to know, game by game and across the match, what you did well and what cost you, with numbers from the logs, and nothing specific to one opponent. The report states what happened and how it compares. It never says what to buy.

## What the report shows

- **Match header.** Score against what the levels predicted (the existing expected score over the games with both levels) and results by seat, going first and going second.
- **One card per game.**
  - Seat, result and score, how it ended, turns, kingdom, levels and win chance (all existing).
  - **Points by source** for both players: Province, Colony, Duchy, Estate, other Victory cards, VP tokens, landscapes and states (Wall, Museum, Misery and the like), Curses. Other Victory cards and landscapes list their cards with the VP each gave.
  - **Final decks** of both players, as counts, Actions first, then Treasures, Victory cards, Curses.
  - **Ending:** Provinces (Colonies), three piles with the piles named, or a resignation, and who took the last turn.
  - **Swing:** the single turn where the VP gap moved most, who played it and what they gained (the existing three-round turning point stays under "Other numbers"), and a line chart of the gap after every turn.
  - **Went well** and **went badly:** the points below, each a fact with its numbers.
- **Across the match.** Points that came up on the same side in 2 or more games, with the games' numbers and how many of them you won.

`scry games OPPONENT --ids ...` (or `--last N`) prints the same in Markdown, and `-json` carries it in `view` per game and `findings`, `first_seat`, `second_seat` for the match.

## Where each field comes from

| Field | Source |
|---|---|
| Points by source | `Client.Score` over the whole log (the `scry review` breakdown), its keys grouped by the table above. Tokens come as one `VP tokens` total: the code doesn't know which card or landscape gave them. |
| Final decks | `Game.FinalDecks()` |
| Ending, piles, Provinces left | `Features.Ending` and `Game.Supply()` |
| Who took the last turn | the player of the last turn in `Game.Turns()` |
| VP gap after each turn, swing | `scoresByTurn`, the same replay `scry review` uses |
| Coppers and Estates trashed | the Trashed column of each of the player's own turns |
| Curses given and received | each Curse that comes into a player's deck between two turn snapshots (the opponent's received count is what you gave) |
| $ per turn, opponent's side | `Features.Coins` of the opponent's side of the same game |

## The rules

A rule fires only past its threshold. The win rates are yours over the decided games where it fired. Each game was reported in a batch of 25, so "your usual" was the other saved games of the last 90 days. Base rate: 50%.

| Rule | Went well when | Went badly when | Won when well | Won when bad | Verdict |
|---|---|---|---|---|---|
| Thinning (Coppers and Estates trashed) | you trashed 4 or more than they did | 4 or more fewer | 73% of 56 | 29% of 79 | kept |
| Points from other sources (Victory cards other than Province, Duchy, Estate, VP tokens, landscapes) | 10 VP or more above them | 10 VP or more below | 77% of 30 | 21% of 24 | kept |
| Curses (given against received) | you gave 3 or more than you took | took 3 or more than you gave | 74% of 38 | 22% of 27 | kept |
| Action cards in the final deck | 6 or more above them | 6 or more below | 70% of 93 | 27% of 44 | kept |
| $ per turn on turns 3-12, against the opponent in the same game | 0.5 above, both sides under 10 VP of other sources | 0.5 below | 69% of 138 | 32% of 157 | kept |
| Province split | 2 or more Provinces above them | 2 or more below | 93% of 89 | 6% of 122 | kept: it is the score, so it restates the result, but it says by how much; the first Provinces' turns are added only when neither side has 10 VP of other sources |
| The last turn | you won and trailed before your last turn, which ended the game | you lost, led before their last turn, and it ended the game | by construction | by construction | kept as a fact: it needs the log's last decision |

Rules that were measured and dropped:

| Rule | Why it went |
|---|---|
| Opening split against your own record | Over 509 decided games the splits that have 30 or more games sit within 4 points of the overall 50% (4/3 53%, 3/4 49%, other 53%), so a 10-point gap never fires. The one that stands out, 2/5, has 25 games. It is luck of the draw, not a choice. |
| Opening buys against your own record | The commonest pair of buys appears in 4 games. Nothing to compare with. |
| Kingdom cards the opponent built 3 or more of and you never took | The sign is wrong: games where the opponent did won 60% of 43 for you, against 52% of 95 when you built the unmatched ones. It misleads. |
| Cards owned after turn 10 against the opponent | Flat: 49% of 35 when 6 or more behind, 47% of 55 when 6 or more ahead. |
| First Gold and first Province | Not a rule on their own, as the accepted position-analysis design shows (not yet on main). The Province split line carries the first Province's turn only on boards with little other VP. |
| $ per turn against your own usual | 58% of 157 above, 45% of 216 below: the same-game comparison separates better (69% and 32%), and the usual stays as context in the text when 20 or more games are in the window. |

Read the win rates as how often a point matched the result, not as a cause. A game you won can show a "went badly" point: 27% of the games with fewer Action cards than the opponent were still won, and the report keeps them because the fact is true.

The rule thresholds sit at the top of `scry/matchview.go`. The measurement is for one player's games, so it is a floor for how well a rule holds for a stranger.

## What a resignation can and cannot tell

A game with no Province or three-pile ending in the log is read as a resignation, as the rest of `scry` reads it, and about a third of the games (174 of 509 decided) are. It may also be a log that stops before the last decision. The log doesn't say who resigned, so the ending line gives the last turn's number and the score and does not name anyone. The two rules that read the last turn skip these games, and any game whose computed score differs from the row's (the last decision is missing): such a game carries a note with both pairs of numbers.

## Input

Game ids only: `--ids` on the command line, the game numbers box on the page. A pasted match chat doesn't carry them (checked on the sample chat: no game number appears), and [match notes](match-notes.md) doesn't need them. DomBot's `!match` reply lists them, one per game, but the page doesn't read the reply.

## Not in this change

- Advice on what to buy. The points say what happened.
- `scry match OPPONENT`, still in the [match report](match-report.md) build order.
- VP tokens by source, a kingdom read and a win chance by turn: the accepted position-analysis design, not built.
- Matches with more than two players.

## Known gaps

- Tokens aren't split by the card that gave them.
- The first-seat log names its own hands only; the second seat's draws are hidden, so nothing here uses hands except the existing draw luck.
- The rules were measured on 509 decided games of one player. A different style of play may move the win rates.
- Some points overlap: a deck that trashed more also tends to have more Action cards. The report shows each; it doesn't net them.
- The gap chart and the VP bars were checked against the page script and a stub DOM, not rendered in a browser.
