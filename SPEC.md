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
