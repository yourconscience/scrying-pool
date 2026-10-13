# Scrying Pool

Look back into your finished [Dominion Online](https://dominion.games) games. Type a game's number, and Scrying Pool loads it in your own logged-in dominion.games tab, replays every line of its log and shows the game turn by turn: hands, draw and discard piles, the Supply, money and VP. Next comes an advisor that reads the replay against strategy for the kingdom. The command is `scry`.

Work in progress; the plan is in [SPEC.md](SPEC.md). Not affiliated with Shuffle iT, Rio Grande Games, DomBot or the Dominion League.

## What's inside

| Module | Where | What it does |
|---|---|---|
| **Capture** | `web/capture.js`, `web/scry-game.js` | In your logged-in dominion.games tab, loads a finished game by number (**My Table** > **Load Old Game** > **Load from End**), seats a bot in the other seat, and copies the log and the score screen. |
| **Cartographer** | `cartographer/` | Reads every log line with the client's own templates and replays the game: each player's deck, zones and tokens after every turn, the Supply piles, the trash and the VP. Checked against score screens and DomBot's final scores. |
| **Analysis page** | `web/analyze.*`, `analyze.go` | `scry web` shows a saved game: VP, $ and deck-size charts, a turn slider, each turn's log with cards marked by type, every player's zones after the turn, the Supply and the final decks. |
| **Advisor** | `advisor/`, `web/insights.*` | `scry web`'s **Profiles** page: for any player with 20+ saved games, what moves their win rate (going first, kingdom roles), form over the last 30 games, $ per turn, first Province, endings, openings and kingdom cards (with their art), with 95% ranges and a few plain-language takeaways; a second player can be shown next to them. `scry features` writes the per-game table as CSV, `scry db` an SQLite database, `scry site` a static copy of the page. |
| **Match notes** | `web/notes.html`, `notes.go` | Turns a match's spectator chat into readable notes split by game, with each game's kingdom and result from [DomBot](http://dominionleague.org/player-guide). |

## Quick start

Needs Go 1.26+ and a browser where you're logged in to dominion.games.

```bash
go install github.com/yourconscience/scrying-pool@latest
scry web
```

Then open http://127.0.0.1:8765. From a clone, `go run . web` does the same.

1. **One time: export the client's log grammar.** Run `scry bookmarklet -client`, save the printed `javascript:` link as a bookmark, click it on dominion.games, then run `pbpaste | scry client save`. The log parser reads lines with these tables. They belong to 2nd Shuffle, so they stay on your machine instead of in this repo.
2. **Open Analyze a game** and type a game number. It's on your dominion.games profile, in DomBot's `!game` reply, or in a log's first line (`Game #184518886`).
3. **If the game isn't saved yet,** drag the page's **Scry game** link to your bookmarks bar. In your dominion.games tab, on the lobby screen, click it and enter the number.

The tab then loads the game, plays it out against a bot and sends it to the app, and the analysis page opens it.

- The bookmark posts to `http://127.0.0.1:8765/api/games`. The first time, the browser may ask whether dominion.games may connect to apps on this device: allow it. Until you answer, the request waits. (In Brave 155 on 2026-10-11 the request from the dominion.games tab hung like this.)
- If the post doesn't get through in a minute, the bookmark copies the game to the clipboard instead. Paste it into the page's **Or paste a game** box.

## Capture

How loading a finished game works, as tested on 2026-10-11:

- **Seats.** The account that loads a game always takes the first player's seat, and the bot takes the second. The table has no way to swap. Sitting down from Spectate, or adding the bot first, still puts you first, and Randomize Player Order drops the loaded game. The score screen's "Original Player" lines say whose cards each seat had, and the replay is named after those players.
- **Where it stops.** "Load from End" gives the highest decision index the server takes (one more is reset to it). The game is replayed up to that decision, and the seat that owns it decides next.
- **Bot moves.** When the next decision is the bot's seat's (the game ended with a resignation), the bot answers and plays on. The client's log model marks every decision (`decisionIndex`, `playerIndex`), so the capture keeps the log up to the loaded decision and saves the bot's lines apart, as `<id>.bot.log`. The checks compare the score screen with both together, since the score screen counts the bot's moves.
- **The last move.** When the next decision is yours, nothing is played, and the game's last move is missing from the log.
- **Games that don't load.** For some finished games the server answers "Internal error." and Load from End sets the decision index to -1. The field keeps that -1 until the next answer, so the capture resets it before each load; without the reset, a game after a failed one looked failed too. The 18 games counted as not loading on 2026-10-11 may include such false failures; all are being tried again. Games saved the day before still loaded, so it depends on the game, not its age.
- **Kingdom.** Once a loaded game starts, the client's kingdom viewer (`kingdomViewer.cardGroups`) holds the kingdom, the landscapes and the cards outside the Supply. The capture saves them as `# kingdom`, `# landscapes` and `# other cards` lines in the score file. For games saved before that, `run(ids, { mode: "kingdom" })` starts each game, reads the kingdom and resigns: about 10 seconds a game.
- **Profiles.** A player's Profile tab lists their 25 latest games (`ProfileResult`): game number, time, whether it was rated, both players' rating levels, the scores and the kingdom. The search on that tab opens anyone's profile.

The capture clicks only the client's own buttons (My Table, Load Old Game, Load from End, the bot seat, Ready, Resign, Yes, Ok, Leave Table, the Profile tab and its player search) and reads the page. Use it on finished games only: the dominion.games Terms of Service ban tools that give a competitive advantage (§2A). Loading hundreds of games a night is heavy use of the client, and the account carries that risk.

### Many games at once

1. Paste `web/capture.js` into the console of your dominion.games tab while logged in.
2. Queue games:
   - `scryCapture.run(ids)` captures games by number, dropping AFK games (a player with fewer than 3 turns);
   - `scryCapture.run(ids, { mode: "kingdom" })` only reads the kingdoms of games saved earlier;
   - `scryCapture.harvest({ minLevel: 50, skip })` visits profiles, starting from your own and following opponents above `minLevel`. It queues their rated two-player games where a player is above `minLevel`, except the numbers in `skip`. Each game keeps the profile's date, scores and levels.

   `run` adds to a running queue. `scryCapture.status()` shows progress.
3. Run `scryCapture.exportNext(25)`, press Cmd+C in the page, then `pbpaste > batch.jsonl`.
4. `scry import batch.jsonl` saves each game to `<user config>/scrying-pool/games` and checks it against its score screen. On macOS that's `~/Library/Application Support/scrying-pool/games`. Kingdom-only lines update the saved games' score files.
5. `scry check` checks all saved games again. It compares the computed final VP with the scores in each game's row, or in a saved DomBot `!prior` list with `-prior FILE`.

For an unattended run (overnight):

- Results and the queue live in the page's IndexedDB. After a reload, paste the script again and call `scryCapture.resume()`.
- Each game has a 6-minute limit, after which the page is reset and the next game starts. A logout or a lost connection is waited out for 15 minutes.
- On 2026-10-11 the server closed the connection after about 230 kingdom-only games ("Server connection was closed or could not be established."). Retry led to the login page, so the run waits for you to log in again; then paste the script and call `resume()`.
- In a hidden tab each game took about a minute while the waits inside it took seconds, which matches Chrome's throttling of chained timers in hidden tabs (at most one wake-up a minute). The script now waits on a worker's timers, and a kingdom-only game takes about 10 seconds.
- The machine must not sleep. On m4 on 2026-10-11 Amphetamine kept it awake, and captures ran in a hidden tab.
- With `scryCapture.server = "http://127.0.0.1:8765"`, each game is posted to a `scry web` on the same machine as soon as it's captured. The browser may first ask whether dominion.games may connect to apps on the device.

For a single game without the app, the **Copy log** bookmark (`scry bookmarklet`) copies the log of a game you loaded yourself: `pbpaste > game.log`, then `scry review game.log`.

## Cartographer: how logs are read

- **Lines.** Each log line is matched against the client's own templates (244 of them, e.g. `BUY_AND_GAIN` "Argument0 buys and gains Argument1."), most specific first. Card lists are read with the client's singular, plural and "a/an" forms.
- **Ownership.** Every change of ownership is replayed: gains, trashes, returns to the Supply or the Box, Masquerade passes, Traveller exchanges, Inheritance. The log doesn't say where some cards come from, so the line that caused an event decides:
  - exiling or trashing a card from the Supply (Invest, Transport, Way of the Camel, Gladiator...);
  - cards that come back right after being trashed (Fortress, Lich, a Trail played from the trash);
  - Loot hidden from your seat but revealed just before (Pickaxe).
- **Per turn.** `Turns()` gives coins, buys, gains, trashes, Actions played, cards drawn and shuffles for each turn. It also gives every player's deck and counters after the turn: Coffers, Villagers, Favors, debt, VP tokens.
- **Zones.** Each turn also says where every card is after its Clean-up: hand (the next hand), draw pile, discard pile, in play, set aside, the Tavern, Island and Native Village mats, Exile.
  - Where the log doesn't say where a card comes from, the template and the card that caused the line decide. Mine gains to the hand, Supplies gains onto the deck, a Horse returns itself rather than the Horse it drew, Wandering Minstrel topdecks what it revealed.
  - Durations stay in play until their last effect. A Throne Room stays as long as the Duration it played twice. "The next time" cards (Secluded Shrine, Cutthroat, Search, Flagship) stay until they fire. Gear, Haven and the like leave at once if they set nothing aside.
  - Cards the log doesn't name, such as an opponent's "draws 5 cards", are counted per zone as unnamed, and the turn lists what they can be. Known cards on top of a deck (topdecks) and at its bottom (Shadow cards) are tracked, so a hidden draw of a known top card is named.
  - The log hides some of your own moves: Scheme's and Merchant Camp's "topdecks a card" in Clean-up. These are guessed from the cards in play.
- **Supply.** `Supply()` gives each pile's starting size and what's left, plus the trash. Sizes follow the setup rules: split and rotating piles, Knights, Castles, Ruins, and piles outside the Supply. Every gain, return, Supply trash and exile changes them.
- **VP.** `Client.Score()` scores each player from their deck, VP tokens and Misery. That includes the cards whose worth depends on the deck: Gardens, Duke, Fairgrounds, Feodum, Vineyard, the Castles, Pasture, Marchland, Territory, Demesne, Distant Lands on the Tavern mat. It also covers the landmarks that score at the end, and Plateau Shepherds.
- **Current cards only.** The rules cover the cards on dominion.games today. Cards the second editions removed (Silk Road, Great Hall, Sea Hag, the Prizes...) appear in none of the saved and test games.

Not covered yet: Possession, and cards played from other players' decks.

### What the saved games showed

- **The corpus.** DomBot's `!prior` lists 462 rated games of gamrevinu in 2026. On 2026-10-11, 242 are saved, the server can't load 18, 6 are AFK games, 3 failed during capture and 193 (2026-03-01 to 2026-09-30) were never tried. `manifest-2026.json` in the games folder lists every game with its status.
- **Seats.** In 119 of the 242 saved games the first seat was the opponent's, so the log's "gamrevinu" is the opponent there and your own draws are hidden. `SeatNames` maps the names back.
- **Checks.** `scry check` compares saved logs with their score screens. Every line must be read, and each player's last turn and final deck must match. On 2026-10-11 all 242 saved games and all test logs passed.
- **Final scores.** Computed VP equal DomBot's final scores in 198 of 242 games. (DomBot's minus sign arrives as "?": "R4D4: ?5" is -5.) The 8 mismatches among the 22 games added on 2026-10-11 aren't classified yet. The other 36, from the first 220:
  - **The game's last decision is missing (33 games).** The replay never contains it. Usually it was the buy that ended the game: 6 VP short for a final Province, 3 for a Duchy, 1 for an Estate. In 32 games it was your seat's decision. In 184163893 it was the bot's seat's, which bought a Gold instead.
  - **Both players are short (2 games, 184149206 and 184162961).** The game went on past the decisions the server keeps.
  - **8 VP more than DomBot (1 game, 184308261).** Not explained yet.
- **Zones.** On those games plus the test logs (137,923 log lines):
  - every player's zones always hold exactly the cards they own;
  - 270 moves didn't find the card where the replay expected it, 116 of them for seats whose cards the log names in full;
  - after a turn, the hand equals the Clean-up draw in 3,638 of 3,641 checked turns.
- **Supply.** At the end of game 184518886 every pile matches the client's piles.

### Tests

`go test ./...` checks that:

- every test log passes `scry check`'s rules;
- the turn-by-turn replay ends at the final decks, and no balance ever goes negative;
- every player's zones add up to their deck after every turn, the next hand is the Clean-up draw, and cards out of place stay rare;
- Supply piles and final VP match the client and DomBot for 184518886;
- every template is reachable, and every card reads in every form.

Set `SCRY_GAMES` to a folder of saved games to run the replay tests on those too.

## Advisor: what your games say

`advisor/` reads every saved game and describes the ones you played (the account that loaded most games, or `-me NAME`).

- **What cards do** comes from the logs, not card text: every "plays X" line is followed by the lines one level under it (+Actions, +Buys, cards drawn, +$, trashing, Curses given). `scry features -profiles` prints these averages. A card played twice without a second "plays" line (Royal Galley) counts as two plays. On the test logs Market Square measures +1 Card +1 Action +1 Buy and Horse +2 Cards +1 Action.
- **Kingdom roles** come from those profiles, with coarse rules per play: a village gives +2 Actions or more, draw draws 2 cards or more, +Buy gives a Buy on most plays, a trasher trashes a card or more, a curser gives Curses on a third of plays or more, plus Attacks by type. A card needs 5 plays to count.
- **Each game** gets: the result from the row's final scores (DomBot `!prior` or the profile), whether the replay's VP match them, who went first, the ending, the kingdom and its roles, which kingdom cards you took, your opening split, the turns of your first Gold and first Province, and your $ on each turn.
- **Endings** follow markus's rule for logs that miss the last decision: at most one Province or Colony left is a Province ending, at most one card from three empty Supply piles is a three-pile ending, anything else a resignation.
- **Win rates** count ties as half, with 95% Wilson ranges. Kingdom cards seen in at least 8 games are ranked by a win rate pulled toward your overall rate, as if 10 average games were added.

Every game is described from both players' sides, so any player with enough saved games has a profile, and two profiles can be compared.

- `scry db` builds `<user config>/scrying-pool/scrying-pool.db` (SQLite) from the saved files: `games`, `player_games` (one row per player per game), `cards` (the profiles) and a `player_summary` view. Lists are JSON text, so `json_each` unpacks them; `db.go` has an example query.
- `scry site -players "a,b"` writes `site/my-games/`: the page plus one JSON file per profile, for any static host. Profiles carry rates only, no game list or opponents' names. Card images link to KingdomCreator's copies on GitHub at a fixed commit (`kingdomCreatorImages` in `site.go`). `artifact.html` is the same page for a Claude Artifact, where outside images are blocked, so its cards show no art.

Not yet: opponents' ratings (an expected result per game), comparison with top players' games, luck.

On 2026-10-12, over 243 games of gamrevinu, the page showed 52% wins overall, 59% going first against 45% second, and 57% with a village in the kingdom against 44% without. With these sample sizes the ranges overlap a lot; treat single numbers as leads, not findings.

## Match notes

`notes.html` (linked from the start page) turns a match's spectator chat into notes split by game. Each game shows its result, kingdom, landscapes and empty piles from DomBot, then that game's chat, with the players' lines in bold. **Load demo match** shows a real six-game match.

1. **Match chat:** after the match, select the whole chat box on dominion.games, copy it, and paste it. Lines like `Match: Alice 1, Bob 0` mark the end of each game.
2. **Game results (optional):** pick one of these.
   - **Paste:** DM DomBot on Discord with `!match Alice, Bob`, copy its reply and paste it into the second box. DomBot's `!match` only covers the last day or two.
   - **Automatic:** tick *Ask DomBot automatically*, and the tool sends `!match` for you (setup below).
   - **Neither:** you get the chat split by game with winners and running score.

Each game's kingdom is shown as piles sorted by cost, then landscapes, framed in their card-type colours. Click a card to open its page on the [Dominion Strategy wiki](https://wiki.dominionstrategy.com).

`go run ./cmd/buildsite` builds a browser-only copy of the notes page into `site/`: the formatter compiled to WebAssembly plus one HTML page that any static host can serve. It can't ask DomBot itself, since that would DM from whoever's token is on the server.

### Automatic DomBot lookups (opt-in, read this first)

Automatic mode sends the DM from **your own Discord account** using your user token. Discord's rules forbid automating user accounts, and your account could be restricted. The tool only ever messages the DM channel whose recipient is the DomBot bot account. It sends one message per lookup and never prints or stores the token.

1. DM DomBot once from the Discord app so the DM channel exists.
2. Get your token: open Discord in a browser, open DevTools > Network, click any `/api/` request, and copy its `authorization` request header. Logging out of Discord invalidates it.
3. Make it available to scry, in either of these ways:
   - macOS Keychain (prompts for the value, keeps it out of shell history): `security add-generic-password -s scrying-pool -a discord -w`. The older item name `dombot` still works.
   - environment variable: `DISCORD_TOKEN=...`

## CLI

```bash
scry web                                              # start page, analysis page and match notes
scry client save [FILE]                               # store client tables from the Copy client tables bookmark
scry bookmarklet [-client]                            # print the Copy log (or Copy client tables) bookmark
scry import [-dir DIR] [-min-turns 3] batch.jsonl     # save captured games and check each one
scry check [-prior prior.json] [DIR or LOGS]          # check saved games against score screens (and final scores)
scry review [-json] game.log                          # final decks, VP and a turn table (JSON: every turn's state)
scry features [-me NAME] [-profiles] > games.csv      # one row per saved game (or what each card does per play)
scry db                                               # SQLite database of the saved games
scry site -players "gamrevinu"                        # the Profiles page as a static site in site/my-games
scry format -dombot reply.txt chat.txt > notes.md     # match notes with a pasted DomBot reply
scry format -ask chat.txt > notes.md                  # match notes, asking DomBot
scry match Alice Bob                                  # print DomBot's !match reply as text
scry ask '!game 184206813'                            # any DomBot command; -raw FILE keeps the JSON
scry cards sync                                       # download all card art thumbnails (about 11 MB)
```

## Data

- **Client tables** (log templates, card names and types) come from your own dominion.games client through the Copy client tables bookmark and stay in `<user config>/scrying-pool/client`. `SCRY_CLIENT` points to another file.
- **Saved games** live in `<user config>/scrying-pool/games`: `<id>.log`, `<id>.score.txt` (with the row, the original players and the loaded decision) and, when the bot played on, `<id>.bot.log`.
- **Card facts:** `cards/cards.json` lists 819 cards with names, types, costs and sets. It comes from [dominiontabs](https://github.com/sumpfork/dominiontabs) (MIT); regenerate it with `go run ./cmd/carddb`. Card text isn't included.
- **Card images** (full card faces) for the Analyze and Profiles pages live in `card-images/`, which git ignores, or in `$SCRY_CARD_IMAGES`. They are the English images from [KingdomCreator](https://github.com/blakevanlan/KingdomCreator) (`docs/img/cards`, 1,010 files, 89 MB), fetched with a sparse clone:

  ```bash
  git clone --depth 1 --filter=blob:none --sparse https://github.com/blakevanlan/KingdomCreator.git /tmp/kc
  git -C /tmp/kc sparse-checkout set docs/img/cards
  cp -R /tmp/kc/docs/img/cards/. card-images/
  ```

  Files are named `<set>_<name>.jpg` (`plunder_buriedtreasure.jpg`, landscapes `plunder_event_avoid.jpg`, new art `_2nd`). `scry web` matches them to cards by name with letters and digits only, preferring second-edition sets and art. A test checks that every real card in the client's tables (832 on 2026-10-11) has an image. Every name in the saved kingdoms has one too.
- **Card art** belongs to Rio Grande Games, so it isn't in this repo. A static site from `scry site` links to KingdomCreator's public copies instead of carrying them. The local app downloads thumbnails from the wiki the first time it shows each card. It keeps them in your user cache folder (`~/Library/Caches/scrying-pool/art` on macOS; `SCRY_ART_DIR` overrides it).

## Security notes

- `scry web` listens on `127.0.0.1` by default.
- `/api/format` refuses cross-origin requests, because one request can send a DM from your account.
- `/api/games` also accepts posts from `https://dominion.games`, so the Scry game bookmark can deliver a game. It only saves a game log under its numeric ID.
- Markdown is rendered in the browser with [marked](https://github.com/markedjs/marked) 18.0.14 from jsDelivr, pinned with a subresource integrity hash.
