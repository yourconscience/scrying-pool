# Loading games

How Scrying Pool gets your finished games out of dominion.games. Back to the [README](../README.md).

## First-time setup

Needs Go 1.26+ and a browser where you're logged in to dominion.games.

```bash
go install github.com/yourconscience/scrying-pool/scry@latest
scry web
```

Then open http://127.0.0.1:8765. From a clone, `go run ./scry web` does the same.

1. **One time: export the client's log grammar.** Run `scry bookmarklet -client`, save the printed `javascript:` link as a bookmark, click it on dominion.games, then run `pbpaste | scry client save`. The log parser reads lines with these tables. They belong to 2nd Shuffle, so they stay on your machine instead of in this repo.
2. **Open Analyze a game** and type a game number. It's on your dominion.games profile, in DomBot's `!game` reply, or in a log's first line (`Game #184518886`).
3. **If the game isn't saved yet,** drag the page's **Scry game** link to your bookmarks bar. In your dominion.games tab, on the lobby screen, click it and enter the number.

The tab then loads the game, plays it out against a bot and sends it to the app, and the analysis page opens it.

- The bookmark posts to `http://127.0.0.1:8765/api/games`. The first time, the browser may ask whether dominion.games may connect to apps on this device: allow it. Until you answer, the request waits. (In Brave 155 on 2026-10-11 the request from the dominion.games tab hung like this.)
- If the post doesn't get through in a minute, the bookmark copies the game to the clipboard instead. Paste it into the page's **Or paste a game** box.

## How loading a game works

As tested on 2026-10-11:

- **Seats.** The account that loads a game always takes the first player's seat, and the bot takes the second. The table has no way to swap. Sitting down from Spectate, or adding the bot first, still puts you first, and Randomize Player Order drops the loaded game. The score screen's "Original Player" lines say whose cards each seat had, and the replay is named after those players.
- **Where it stops.** "Load from End" gives the highest decision index the server takes (one more is reset to it). The game is replayed up to that decision, and the seat that owns it decides next.
- **Bot moves.** When the next decision is the bot's seat's (the game ended with a resignation), the bot answers and plays on. The client's log model marks every decision (`decisionIndex`, `playerIndex`), so the capture keeps the log up to the loaded decision and saves the bot's lines apart, as `<id>.bot.log`. The checks compare the score screen with both together, since the score screen counts the bot's moves.
- **The last move.** When the next decision is yours, nothing is played, and the game's last move is missing from the log.
- **Games that don't load.** For some finished games the server answers "Internal error." and Load from End sets the decision index to -1. The field keeps that -1 until the next answer, so the capture resets it before each load; without the reset, a game after a failed one looked failed too. The 18 games counted as not loading on 2026-10-11 may include such false failures; all are being tried again. Games saved the day before still loaded, so it depends on the game, not its age.
- **Kingdom.** Once a loaded game starts, the client's kingdom viewer (`kingdomViewer.cardGroups`) holds the kingdom, the landscapes and the cards outside the Supply. The capture saves them as `# kingdom`, `# landscapes` and `# other cards` lines in the score file. For games saved before that, `run(ids, { mode: "kingdom" })` starts each game, reads the kingdom and resigns: about 10 seconds a game.
- **Profiles.** A player's Profile tab lists their 25 latest games (`ProfileResult`): game number, time, whether it was rated, both players' rating levels, the scores and the kingdom. The search on that tab opens anyone's profile.

The capture clicks only the client's own buttons (My Table, Load Old Game, Load from End, the bot seat, Ready, Resign, Yes, Ok, Leave Table, the Profile tab and its player search) and reads the page. Use it on finished games only: the dominion.games Terms of Service ban tools that give a competitive advantage (§2A). Loading hundreds of games a night is heavy use of the client, and the account carries that risk.

## Your latest games

The app's **Recent games** page sums up your 25 latest rated two-player games the way DomBot's `!results` does: your score against the score your rating levels predicted, how strong your opponents were, your level, your wins against your losses, and each game. To fill it from dominion.games:

1. Drag the page's **Recent games** link to your bookmarks bar.
2. In your dominion.games tab, on the lobby screen, click it. It opens the Recent games page in a tab of its own (if the browser blocks it, allow pop-ups for dominion.games and click again), reads your Profile tab and sends the page your latest games with their dates, scores and both players' levels.
3. It then loads the games the app hasn't saved, one at a time, and the page saves each one as it arrives. Leave the dominion.games tab be meanwhile. On 2026-10-14, 25 games took 8 minutes: 21 saved, 2 AFK, 1 the server couldn't load, and 1 timed out twice waiting for its log.

The live site has a copy at https://yourconscience.github.io/scrying-pool/recent.html. Its **Update** button opens dominion.games; click the Recent games bookmark there (drag it from that page: it reports to the live page, not to the app), and your profile's latest games come back to the page, which keeps them in that browser. **Publish** writes them to the site for every device ([site-data/README.md](../site-data/README.md)).

The two tabs talk by `postMessage`, so the browser's question about dominion.games connecting to apps on this device doesn't come up. If a game fails, it's tried once more after the others. Clicked again (after a reload, say), the bookmark sends games it loaded before instead of loading them again.

## Many games at once

1. Paste `scry/web/capture.js` into the console of your dominion.games tab while logged in.
2. Queue games:
   - `scryCapture.run(ids)` captures games by number, dropping AFK games (a player with fewer than 3 turns);
   - `scryCapture.run(ids, { mode: "kingdom" })` only reads the kingdoms of games saved earlier;
   - `scryCapture.harvest({ minLevel: 50, skip })` visits profiles, starting from your own and following opponents above `minLevel`. It queues their rated two-player games where a player is above `minLevel`, except the numbers in `skip`. Each game keeps the profile's date, scores and levels.

   `run` adds to a running queue, and a game that fails is tried once more after the rest. `scryCapture.status()` shows progress.
3. Run `scryCapture.exportNext(25)`, press Cmd+C in the page, then `pbpaste > batch.jsonl`.
4. `scry import batch.jsonl` saves each game to `<user config>/scrying-pool/games` and checks it against its score screen. On macOS that's `~/Library/Application Support/scrying-pool/games`. Kingdom-only lines update the saved games' score files.
5. `scry check` checks all saved games again. It compares the computed final VP with the scores in each game's row, or in a saved DomBot `!prior` list with `-prior FILE`.

For an unattended run (overnight):

- Results and the queue live in the page's IndexedDB. After a reload, paste the script again and call `scryCapture.resume()`.
- Each game has a 6-minute limit, after which the page is reset and the next game starts. A logout or a lost connection is waited out for 15 minutes.
- On 2026-10-11 the server closed the connection after about 230 kingdom-only games ("Server connection was closed or could not be established."). Retry led to the login page, so the run waits for you to log in again; then paste the script and call `resume()`.
- In a hidden tab each game took about a minute while the waits inside it took seconds, which matches Chrome's throttling of chained timers in hidden tabs (at most one wake-up a minute). The script now waits on a worker's timers, and a kingdom-only game takes about 10 seconds.
- The machine must not sleep (on macOS, `caffeinate -i` or an app like Amphetamine keeps it awake). Captures can run in a hidden tab.
- With `scryCapture.server = "http://127.0.0.1:8765"`, each game is posted to a `scry web` on the same machine as soon as it's captured. The browser may first ask whether dominion.games may connect to apps on the device.

For a single game without the app, the **Copy log** bookmark (`scry bookmarklet`) copies the log of a game you loaded yourself: `pbpaste > game.log`, then `scry review game.log`.

## Every day on a Mac

`scry collect` does the above for your own latest games without the console. It drives the dominion.games tab in Brave through AppleScript (it opens one if none is open), asks the server for your 100 latest two-player games, loads the ones not saved yet with the capture script and imports them.

- Once: in Brave, turn on View > Developer > Allow JavaScript from Apple Events. The first run asks whether the program may control Brave: allow it.
- It never logs in or kicks another session. When the tab isn't logged in, or you're at a table, it stops and says so (`-notify` shows it as a notification).
- The server lists only the 100 latest games, so a weekly run misses games once you play more than about 14 a day. Run it daily.
- `-since 2160h` also loads the games a `manifest-*.json` in the games folder (DomBot's `!prior` list, with what became of each game) marks `not-tried`, played within the last 90 days. Each keeps its row from the manifest, so it has its date and scores. Games already saved are skipped, so the flag can stay on a daily run.
- `-publish REPO` then rebuilds `site-data/` in a clean checkout of this repository on a new branch from `origin/main`, pushes it and opens a pull request with `gh`. With `-merge` it waits for the pull request's checks and merges it, so the site redeploys on its own.
- Card images go into the published profiles only when `card-images/` is in the working directory or `$SCRY_CARD_IMAGES` names it; a launchd agent runs from `/`, so set `SCRY_CARD_IMAGES` in its `EnvironmentVariables`.

To run it every morning and publish on Sundays, put two launchd agents in `~/Library/LaunchAgents` (`scry collect -notify` daily, `scry collect -notify -publish REPO -merge` weekly, with `StartCalendarInterval`) and load them with `launchctl bootstrap gui/$(id -u) FILE`. A Mac that was asleep at that time runs the job when it wakes. The agents run the installed binary: rebuild it (`go build -o ~/.local/bin/scry ./scry`) after changes to what the site reads, or the weekly run publishes data in the old shape.

## Scouting an opponent

Before a league match, `scry scout PLAYER` loads the player's latest games and compares their play with yours:

```bash
scry scout -o scout.md PLAYER
```

- It drives the dominion.games tab in Brave the way `collect` does (same setup, same checks: logged in, not at a table). It opens the player's profile and asks the server for their latest two-player games: it lists up to 100. Games against bots are left out (the client gives bots player ids from 2^30).
- It then loads those games with the capture script, newest first, until 25 are saved (`-n`): a game the server can't load, an AFK game or one whose log times out makes room for an older one. They're saved with yours. A recent game takes about 20 seconds, so a player who plays a lot needs about 10 minutes the first time and little after; for a player whose games are months old, many fail and the run goes further back (`-budget`, an hour by default).
- What their profile says is kept in `scout-<player>.json` in the games folder: level, μ and φ, rank, wins, losses and ties, the longest streaks, your head-to-head record and the expected score of each of their games. Your own profile's summary goes next to theirs.
- `-offline` makes the report from the saved games and the last fetch, without the browser. Without a fetch it takes the player's latest saved games, which are usually only the ones against you.
- The report (Markdown, or `-json`) sets the player's games beside yours of the last 90 days: results against the levels' prediction, seat, rating gap, endings and game length; Copper splits and opening buys; what they built (engine, draw without a village, trashing, attacking, cursing, +Buy, judged from the roles of the cards they took) and what the kingdom offered; the kingdom cards they take and skip; the splits where they differ most; and your saved games against them.
- 25 games are few: a split with 4 games on a side swings 25 points on one result, so the report shows each rate with its 80% range and counts.

In the local app (`scry web`), the Scout page does the same from the browser: type the player's name and press Scout. It runs `scry scout` in the background (one run at a time, since it drives the one dominion.games tab), shows its progress and log, and keeps the report next to the fetch as `scout-<player>.md`. The page lists every player scouted before and shows their report, made again from the saved games. The page is local only: the published site can't drive your browser.

The games are finished ones, as for `collect`.
