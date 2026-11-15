# League review: your division side by side

Back to the [README](../README.md). It reads games the way the [match review](match-review.md) does, for every player of a league division at once.

Status: built on 2026-11-15 and checked against a hand review of Season 78, division D6, done the same day (below).

At the end of a league season you want to see how the whole division played, not only your matches: who wins going first, who keeps up on money, who closes games on piles and who loses leads on the last turn. `scry league` does that from DomBot's `!match` replies.

## Input

Paste the DomBot `!match` reply of every match of the division into one text file, one after another:

```
markus v. gamrevinu
4–2
Games beginning November 15, 2026 at 1:00 PM
185961251 (24m) — gamrevinu: 39, markus: 26
185961617 (7m) — markus: 14, gamrevinu: 7
...
185962490 (25m) — markus: 0, gamrevinu: 3 winner=markus
```

- The player named first on a game line went first.
- DomBot underlines the winner, and a copy loses that, so the higher score wins. A game won with fewer or equal points (a resignation, or a tie broken on turns) needs `winner=NAME` at the end of its line.
- When a reply's match score (`4–2`) doesn't add up from its game lines, the report says so under **Check**: that's how a missing `winner=` shows up.
- Other lines (times, decisions) are skipped, and a game pasted twice counts once.

## Commands

```
scry league FILE           # report on the saved games, opens nothing
scry league -load FILE     # first save the games not saved yet
scry league -json FILE     # the same as JSON
```

League games are unrated, so no list the server gives names them; `-load` saves them by number. It copies the games already in your games folder, prints the numbers of the rest, then loads those through the dominion.games tab in Brave, as `scry collect ID ...` does ([Loading games](capture.md)). A game that won't load is listed and the report still runs.

The games go to a league folder apart from your own games: `<user config>/scrying-pool/league-NAME`, NAME being FILE's name without its extension (`s79-e.txt` gives `league-s79-e`). `-dir` picks another folder.

## What the report shows

A line for the division: how many decided games the player who went first won, and how many pile endings the player who emptied the third pile won. Then one row per player, most wins first:

| Column | Where it comes from |
|---|---|
| Record, going first, going second | the replies, all games |
| Saved | games with a saved log; the columns after it cover these |
| $ t3-12 | mean `coins_mid`: $ per turn on turns 3-12 |
| vs opponent | mean of `coins_mid` minus the opponent's `them.coins_mid` in the same game |
| Piles ended: won | games that ended on piles where the player took the last turn (`view.end.by`), and how many of those they won |
| Lost to their piles | saved losses the opponent ended on piles |
| Lost the lead on their last turn | saved losses where the opponent took the last turn and the player was level or ahead before it (`view.gap`) |

Each player's games go through the code of `scry games -me PLAYER --ids ...`, so the numbers agree with that report. Endings are read as that report reads them; the log doesn't say who resigned.

## Checked

Season 78, D6: 6 players, 72 games, 66 of them saved. Run over the same games as the one-off script of the hand review, every row matches it, for example yikyak 16 games, $5.56, +0.51 against opponents, 6 pile endings won, and gamrevinu 28 games, $4.67, -0.43, 1 pile ending won and 10 lost to the opponent's. The first player won 51 of 72 games (71%), and the player who emptied the third pile won 24 of 26 pile endings.
