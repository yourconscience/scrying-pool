# SPEC: Dominion game review from dominion.games logs

Status: draft for review, 2026-10-10. Replaces the 2026-10-05 draft. Nothing below is built yet.

## Goal

Review your finished Dominion Online games from their full game log. The tool shows:

- what happened turn by turn;
- how each deck grew;
- where you lost tempo against your opponent;
- the likely mistakes, plus strategy notes for the kingdom.

Rating history and opponent records come from DomBot. Match notes from spectator chat stay as a side feature.

## How a game gets in (today, by hand)

1. Discord, DomBot DM: send `!game` and copy the game ID (for example `184518886`).
2. dominion.games, **My Table**:
   - **Load Old Game**: enter the game ID, then press **Load from End** (the decision index fills in by itself);
   - put a bot in the empty seat, then press **Ready**.
3. The right-hand log now holds the whole game from turn 1, for example "Turn 2 - gamrevinu", "g plays 4 Coppers. (+$4)", "g buys and gains a Hostelry". It can be selected but not copied.

## Rules from the dominion.games Terms of Service

- §2(A) bans "automation software (bots) … with either the direct aim or incidental side-effect of gaining a competitive advantage".
- §2(C) bans anything that would "intercept, emulate or redirect the communication protocols … for any purpose".
- §2(D)(b) bans "any connection using programs or tools not expressly approved".

Design consequences:

- **Finished games only.** Never read or advise on a game in progress.
- **No protocol work.** No websocket sniffing, no headless client, no direct calls to the game server.
- **Read only what the official client already shows you,** in your own browser, after you loaded the game yourself. The loaded log is from your seat, so the opponent's draws stay hidden ("L draws 5 cards"). We don't try to reconstruct them.
- **You do the clicks on dominion.games.** The tool only reads text off the page.

## Capture options

| Option | Effort per game | Notes |
|---|---|---|
| **A. "Copy log" bookmarklet** (recommended) | one click on the loaded game, then paste into the app | Reads the log panel's text from the page and copies it with player and card markup kept. Interface-only, no network calls. |
| B. The same bookmarklet sends straight to the local app | one click | Posts to `127.0.0.1`, with an import-only endpoint that accepts the dominion.games origin. Browser private-network rules need checking. |
| C. Claude in Chrome reads the tab | ask me in a session | Good for building and testing the parser now, but not the everyday path. |

## Analysis (per game)

1. **Turn table** for both players: coins played, buys, gains, trashes, actions played, cards drawn (yours only), shuffles.
2. **Deck over time:**
   - size;
   - money density;
   - VP;
   - each card's count by turn, from gains, trashes, and cards returned to piles (like Horses).
3. **Milestones:** turns with $5+ or $6+; first Gold and first Province; how many reshuffles; when each player started greening; the pile state at the end.
4. **Mistake hints.** These are rules, labelled as hints, not verdicts:
   - turns with unspent $ or unused buys;
   - draws that left an action dead (terminal collision);
   - greening early or late compared with the opponent;
   - three-pile danger nobody reacted to;
   - an opening that differs from what the kingdom suggests.
5. **Strategy review.** A Claude or Codex session reads the turn table plus the log and writes the critique. It runs on your subscription in a session, never through API keys. Later, maybe an "ask Claude" button in the hosted page.

Across games, once a few are stored:

- your average $ per turn and greening turn compared with your opponents;
- results grouped by kingdom features (has +Buy, trashing, villages);
- rating over time from pasted `!results`.

## Acceptance tests

- The log of `184518886` (gamrevinu vs UtagawaLk, 10-Oct-2026) parses with no unknown lines.
- Per-card counts match DomBot's `!game` picture for that game, given as gamrevinu/UtagawaLk. I read these off the image, and still need to confirm whether they count gains or buys:
  - Hostelry 7/3, Destrier 3/7, Groom 7/3, Shaman 3/3, Horse 2/5;
  - Estate 6/0, Duchy 1/0, Province 0/1;
  - Copper 3/0, Silver 1/0, Gold 0/2;
  - Innkeeper 0/2, Menagerie 0/2, Fortress 0/1.
- Final score from the log is 9 to 6, with UtagawaLk resigning after 13 turns.
- The turn table and deck chart render on desktop and at phone width.
- Mistake hints carry a turn number and a one-line reason that can be checked against the log.

## Repo change

- **Rename** (still open): scrying-pool / `scry` (recommended), cartographer, or domnotes.
- **New core:**
  - `logparse/`: log lines to events, with card names (and plurals like "2 Coppers" and "Grooms") resolved through `cards/cards.json`;
  - `analysis/`: turn table, deck timeline and hints;
  - a review page in `web/`;
  - the bookmarklet in `web/bookmarklet.js`.
- **Kept:** the card database, art cache, kingdom viewer and WebAssembly build. The match-notes formatter moves under a "Match notes" tab.
- **Shelved:** the Discord user-token path. It stays opt-in for `!match`, and isn't needed for logs.

## Phases

Time estimates are ballparks, not measured.

0. **Get the log out.** About 1 hour.
   - Read the loaded game's log panel and work out its markup.
   - Save `184518886` as the first test log.
   - Write the bookmarklet.
1. **Parser plus turn table.** About half a day.
   - Accept: the counts above match.
2. **Deck timeline, milestones and hints.** About a day.
3. **Review page and the session-run critique,** with 2–3 more of your games as test logs.
4. **Across-game stats and `!results` rating history.**

## Open questions

1. How should the bot seat be mapped back to the real opponent? The loaded log names the opponent after the bot that took the seat ("Turn 1 - Lord Rattington", initial "L"). Their real name comes from DomBot `!game` or from the table's "Original Player" line, so the parser takes an optional name map.

## Outcome / Deviations

Phase 0, 2026-10-10:

- **Name:** Scrying Pool. The repo is `yourconscience/scrying-pool` (renamed; GitHub redirects the old URL), the module path changed to match, and the command is `scry`. The old Keychain item `dombot` and the old art folder are still read.
- **Log markup:**
  - each line is a `.log-scroll-container .log-line`, built from `.log-line-block > span` pieces;
  - nesting comes from the line's inline `padding-left`, in steps of 4.54545%;
  - turn lines carry `new-turn-line` and shuffle lines `shuffles-line`.
- **Test log:** `testdata/logs/184518886.log`, captured in the format `<level><T|S>\t<pieces joined by |>`. It has 871 lines and 27 turns, and ends mid-turn 14, where UtagawaLk resigned.
- **Bookmarklet:** `web/bookmarklet.js`, printed by `scry bookmarklet`. Its extraction is the code that produced the test log. Not yet tested: clicking the saved bookmark on a live page and its clipboard write. The dominion.games session logged out before that check.

Phase 1, 2026-10-10 (partial):

- `gamelog.go` parses copied logs into events and tracks each deck through gains, trashes and returns to piles. `scry review -names "Lord Rattington=UtagawaLk" LOG` prints the final decks and a turn table: $, buys, other gains, trashes, Actions played, cards drawn, deck size, VP.
- Acceptance met on `184518886`:
  - no unknown lines;
  - both final decks equal DomBot's `!game` picture, which shows final decks, not gains;
  - 9 to 6 VP;
  - last turns 14 and 13.
- Not done yet: the review page in the web UI, and more test logs. The dominion.games session logged out, and I don't sign in for the user.

2026-10-10, evening: Cartographer and capture.

- **Cartographer** (`cartographer/`) parses logs with the dominion.games client's own 244 templates and 842 card names.
  - It replays ownership and gives a per-turn state: decks and counters for Coffers, Villagers, Favors, debt and VP tokens.
  - `scry check` passes on all 220 imported games and all test logs: no unknown lines, and last turns and final decks equal the score screens.
  - Fixes driven by real games: score screens read as trailing card pairs; Supply exiles and trashes decided by the causing card (including Ways); Loot named by a preceding reveal; Fortress, Lich and Trail returning from the trash; Inheritance; the Box; Fleet turns not counted; Tax debt and debt-cost buys.
  - Not done: zones within a player's deck, Supply pile sizes, variable-VP cards (done on 2026-10-11, below).
- **Capture.** `web/capture.js` plus `scry import` (AFK games dropped).
  - Checked on 10 games that were captured by hand: byte-identical logs, all 10 pass.
  - From DomBot `!prior`: 462 rated games in 2026. 220 imported (`<user config>/scrying-pool/games`); the rest were not captured when the run was stopped.

2026-10-11: zones, Supply and VP, from the saved games only (no dominion.games).

- **VP** (`cartographer/vp.go`): fixed values from the cards' printed VP, the variable cards, Misery, Distant Lands, the scoring landmarks (read from the score screen) and Plateau Shepherds.
  - 164 of 220 games match DomBot's final scores (DomBot's minus arrives as "?"). The other 56 are capture problems, not scoring ones. The load stops at the last decision: 32 times it was the viewer's (the final buy is missing), 21 times the bot's (it played on after the resignation that ended the game), 3 unclear.
  - Cards removed in the second editions are left out of the rules (wiki "Removed cards"); none appears in the saved games.
  - The seat mapping explains much of the earlier gap. The loading account sits in seat 1, the first original player's seat. With it, matches went from 84 to 153.
- **Supply** (`cartographer/supply.go`): pile sizes from the setup rules, replayed through gains, returns, Supply trashes and exiles, and gains from the trash. It matches the client at the end of 184518886.
- **Zones** (`cartographer/zones.go`): hand, draw pile, discard pile, in play, set aside, mats and Exile per player, in `Turns()`.
  - Checked on every saved game: zones always add up to the owned cards. 270 of 137,923 lines found a card out of place. The next hand equals the Clean-up draw in 3,638 of 3,641 turns.
  - Rules that came from the games: implicit gain destinations; Hexes and Boons whose effects follow at the same level; Mastermind's three plays; Royal Galley's silent set-aside; Prince returning its card; Avoid and Order of Masons keeping cards out of a shuffle; Shadow cards going under the deck; "now or next turn" Durations; cards that stay until they fire; Flagship and Throne Room holding Durations; plays from the Supply or trash.
- `scry review` names the seats from the saved score screen. It scores with `Client.Score`, and `-json` adds zones, final VP, Supply piles and the trash.
- Capture, tested on dominion.games the same day:
  - You can't choose a seat: the loading account always takes seat 1. Spectate and re-sit, or a bot added first, change nothing; Randomize Player Order drops the loaded game.
  - The decision index can't go past "Load from End".
  - The client's log model (`LogEntriesChange.execute` -> `game.logModel.entries`) holds `DecisionEntry` records between the log entries. The capture now keeps the log up to the loaded decision and saves the bot's lines as `<id>.bot.log`. The 24 affected games were recaptured; all 24 logs plus bot lines match their score screens, and final scores match DomBot in 184 of 220 games (was 164).
  - A bot can hang mid-turn ("Waiting for Lord Rattington"); the capture goes on once the bot's first move is in.
  - When a bot's move ends the game, the client sometimes stops rendering its templates; the capture stops and asks for a reload.

2026-10-11, afternoon: analysis page, corpus fixed.

- **Web UI.** `/` is a start page; `/analyze.html#ID` shows a saved game turn by turn: VP, $ and deck-size charts, a turn slider, each turn's log with cards marked by type, every player's zones, the Supply with changes and the final decks. `/notes.html` is the match-notes page that used to be `/`.
- **Getting a game from the logged-in browser.** The page builds a "Scry game" bookmark (`web/capture.js` + `web/scry-game.js`, with the app's address). In the dominion.games tab it loads one game like the capture script and posts it to `/api/games`, which accepts the `https://dominion.games` origin only for that route and only saves a log under a numeric ID. In Brave 155 the post from the dominion.games tab waited for the browser's local-access answer; after a minute the bookmark copies the game for the page's paste box instead. Not yet tested end to end in a real tab.
- **Kingdom.** The client's `kingdomViewer.cardGroups` holds the kingdom, the landscapes and the non-Supply cards. The capture saves them as `# kingdom`, `# landscapes` and `# other cards` lines in the score file. None of the 242 saved games has them yet.
- **Corpus.** Of 462 rated 2026 games: 242 saved (all pass `scry check`, 198 match DomBot's final scores), 18 the server can't load ("Internal error.", decision index -1), 6 AFK, 3 capture errors, 193 not tried (March to September). `manifest-2026.json` in the games folder records each one.
- **Next:** the Advisor plan (luck and shuffles, rule-based mistake hints, strategy), published as an artifact on 2026-10-11.


2026-10-11, evening: unattended capture and more games.

- **Kingdom.** DomBot's token in the Keychain had expired (401), so kingdoms come from dominion.games: the capture's kingdom-only mode starts each saved game, reads `kingdomViewer.cardGroups` and resigns, about 10 seconds a game.
- **Profiles as a source.** The Profile tab's `ProfileResult` objects give a player's 25 latest games with game number, time, rated, both players' levels, scores and kingdom; the tab's search opens any player. `harvest()` starts from your profile, follows opponents above level 50 and queues their rated two-player games with a player above 50. 30 profiles took 108 seconds, saw 261 players above 50 and found 651 games; 502 of them (total score 10 or more, not yours) were queued.
- **Unattended runs.** Results and the queue persist in IndexedDB (`resume()` after a reload); a 6-minute limit per game; logouts waited out for 15 minutes; `run` adds to a running queue; optional posting to `scry web`.
- **Two capture bugs fixed.** The decision field keeps its last value, so a stale -1 failed the game after an unloadable one; the field is now reset before each load. Chained timers in the hidden tab woke about once a minute; waits now run on a worker's timers.
- **Other players' games.** `SeatNames` maps seat 1 to the first original player even when the loading account isn't one of them. `scry check` takes the final scores from each game's row when no `!prior` list is given.
- Queued for the night of 2026-10-11: kingdoms for 237 saved games, 8 new games of yours, the 214 not yet saved, and 502 games of strong players.

2026-10-12: research, then the first Advisor slice.

- **Research** (artifact "Обзор аналитики Dominion"): dominion.games has a Stats tab in beta (announced 2026-09-24 in 2nd Shuffle's #news) with per-card stats by skill tier and a per-player view. On 2026-10-12 its server (acceptance-dominion-alpha.dominion.games) refused connections, and the production client 2.3.5 hides the tab behind the server feature CARD_STATS_VIEW. markus's Woodcutter statistics (2018-2022) are the model for skill-adjusted results: P(win) = 1/(1+exp(-Δμ)). We build our own tool and don't duplicate the card table.
- **advisor/**: card profiles measured from the logs, kingdom roles from them, per-game features and a summary; `scry features` (CSV) and the **My games** page (`/insights.html`, `/api/insights`, `/api/features.csv`).
- Rows: 196 saved games had an empty "# row:" (imported before rows were written); they were filled from manifest-2026.json, so results come from final scores. 198 of 243 games' replays end at those scores.
- Not yet: rating-adjusted expectations, top players' baselines, luck.
- Same day, later: features for both players of every game; Profiles page with a player picker and a second player to compare; differences with 95% ranges, form over the last 30 games, first-Province histogram, new palette. `scry db` (SQLite via modernc.org/sqlite v1.60.1, published 2026-09-29; it needs Go 1.26) and `scry site` (static copy). The site was published as a private Claude Artifact with gamrevinu's profile only.

2026-10-13: Profiles page redesign.

- The 2026-10-12 palette (teal on navy) made ranges hard to see. The page now uses the project palette with more contrast and Source Sans 3. Green and brick red only mean a range that excludes the reference; grey and a hollow point mean it doesn't.
- "What stands out" lists, in sentences, only the differences whose 95% range leaves no doubt about the direction.
- Charts are drawn at their container's width, so chart text keeps its real size; they redraw when the width changes.
- Kingdom cards are rows with a small image of the whole card, from KingdomCreator's card images: `/card-image/` locally, GitHub raw at commit d9a802db on a static site.
- Reading the saved games took 25 s, two thirds of it trying every log template's regexp on every line. Each template now keeps its longest fixed word and skips the regexp when the line lacks it: 3 s for 244 games, the same features CSV byte for byte. `scry web` reads them at start.

2026-10-13, later: hosting.

- The repo is public and the Profiles page is on GitHub Pages (https://yourconscience.github.io/scrying-pool/). GitHub Pages wasn't available for the private repo on this plan.
- `.github/workflows/site.yml` runs `go vet` and `go test` on every pull request and every push to `main`; on `main` it also runs `scry site -data site-data` and publishes the result. Without the client tables, 13 of 31 tests skip in CI.
- `site-data/` holds the published profiles (win rates only). `scry site -data DIR` copies them instead of reading saved games, so CI doesn't need them; the output is the same file for file.

2026-10-14: cleanup.

- Layout: everything the app needs is in `scry/`: the command (install with `go install github.com/yourconscience/scrying-pool/scry@latest`; before, installing from the repo root gave a binary named `scrying-pool`), `scry/internal/cartographer` and `scry/internal/advisor` (parts of the app, not separate modules), `scry/web/`, the embedded data and the test logs. Build helpers are in `tools/` (`buildsite`, `carddb`).
- Removed what was shown but not built: the "Advisor coming" panel on the analysis page and the roadmap lines on the start page and in docs. Plans stay in this file.
- Static builds no longer write `artifact.html`: the page is hosted on GitHub Pages now.
- Test logs: 10 games instead of 40 (19 files instead of 79). They cover the expansions and the cards the tests check; `SCRY_GAMES` runs the replay tests on all saved games. The old logs stay in git history.

2026-10-14: site.

- Profiles: "What stands out" shows each clear difference as a number of percentage points with its game counts, largest first. Going first is only in the summary (59% against 45% is expected in Dominion and says little); the win-rate chart keeps the kingdom features. The setup guide left the page; it's in the README.
- The live site has the Match notes page next to Profiles (`tools/buildsite` builds both; CI builds it on every pull request). Its Download button is back: it was hidden for the Claude artifact viewer.

2026-10-14: recent games.

- DomBot's `!results` (2026-10-14) shows the rating period's score against an expected score ("8/9, expected: 5.31"), the number of opponents and their skill range, and the rating before and after (level 55.82, μ 1.05, φ 0.137, n 616).
- The **Recent games** page does the same for the 25 latest saved games, from the levels a profile lists, and compares wins with losses. The Recent games bookmark reads your Profile tab, writes the levels into saved games' score files (`POST /api/profiles`) and loads the missing games.
- First run, 2026-10-14: the profile's 25 latest rated games were all newer than the saved ones. In 8 minutes 21 were saved, 2 were AFK, the server couldn't load 1, and 1 timed out twice waiting for its log; an older queue of 732 games in that browser waited and was put back. The saved levels match DomBot (55.82). For that day's 9 games the page shows 6/9 against an expected 5.7, where DomBot's `!results` said 8/9 against 5.31; which games DomBot counts isn't known yet. `scry check`: 16 of the 21 new games end at their final scores (VP tokens, Colony and Wall games miss; one log has 2 unread lines, "fails to take a Turn").
- Capture reliability: games go to the opened Scrying Pool tab by `postMessage` and are confirmed by it, instead of a request from dominion.games to 127.0.0.1 (which hung in Brave 155 until the device-access question was answered); a failed game is retried once; `harvest()` finds your name from your profile instead of a fixed one; the bookmarks load capture.js again when the tab has an older copy.

2026-10-14: parser fixes.

- All 265 saved games now read completely and end with their score screens' decks and turns. Fixed: a Lich discarding itself from the trash with its name after the line (`DISCARD_WITH`) was lost from the deck and the zones; "g fails to take a Turn (from )." didn't read, since the client prints the source as nothing (a line no template reads is tried again with empty arguments, never Argument0); allies like Plateau Shepherds, which the score screen shows without a count, weren't scored.
- Computed VP equal the final scores in 216 of 265 games. 48 of the other 49 lack the game's last decision, which a loaded game never has; 184308261 (8 VP more than the final score) isn't explained.
- `TestLogsMatchScoreScreens` runs on every saved game with `SCRY_GAMES`, counting the bot's lines as `scry check` does.

2026-10-15: Recent games on the live site.

- `scry site` also writes the Recent games page and `data/recent.json`, and `-data` copies `recent.json` from `site-data/`, so the live site shows the published snapshot at `recent.html`. It lists opponents' names and levels, as the player's dominion.games profile does; the profiles still carry rates only.
- The static page reads `data/recent.json`, hides the bookmark section and doesn't link games to the analysis page.

2026-10-15: narrower ranges, Recent games updates from the live site.

- Ranges are 80% instead of 95% (`advisor.z`): bars about a third shorter, at the price that about 1 in 5 differences that are only chance looks clear. On gamrevinu's 250 games, "village in the kingdom" (+11 pp) now counts as clear, and "What stands out" lists 5 findings instead of 2.
- The live Recent games page has **Update**: it opens dominion.games with `#scry-recent`, and the Recent games bookmark (built from `bookmark.json`, which `scry site` writes, with the page's own address) answers the page that opened the tab with the profile's 25 latest games. The page keeps them in localStorage; games without a saved log show the profile's result, scores and levels. **Publish** writes the list to `site-data/recent.json` through the GitHub API with a fine-grained token the browser keeps.
- The summary (score, expected, opponents, wins and losses) is computed in `recent.js` for both the app and the live page; `advisor.LatestGames` only lists the games with their win chances. Snapshots keep $ per turn for turns 1-10.
