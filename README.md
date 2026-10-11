# Scrying Pool

Reviews of your finished [Dominion Online](https://dominion.games) games, built from the game log: turn by turn, how each deck grew, and where the game was won or lost. The command is `scry`. Work in progress; see [SPEC.md](SPEC.md).

It also turns a match's spectator chat into readable notes split by game. Each game shows its result, kingdom, landscapes and empty piles from [DomBot](http://dominionleague.org/player-guide), followed by that game's chat, with the players' lines in bold.

Not affiliated with Shuffle iT, DomBot or the Dominion League.

## Run

Needs Go 1.22+.

```bash
go install github.com/yourconscience/scrying-pool@latest
scry web              # http://127.0.0.1:8765
```

Or from a clone: `go run . web`.

Click **Load demo match** to see a full example. It uses a real six-game match, with the real spectator chat.

## Getting a game log

1. One time: export the client's log grammar and card list. Run `scry bookmarklet -client`, save the printed `javascript:` link as a bookmark, click it on dominion.games, then run `pbpaste | scry client save`. These tables belong to 2nd Shuffle, so they stay on your machine instead of in this repo.
2. On dominion.games, open **Profile** and click the Load Game icon next to a game in Recent Results (or use **My Table** > **Load Old Game** with a game ID). Press **Load from End**, then **Ready**.
3. Click the **Copy log** bookmark (`scry bookmarklet` prints it), then save the clipboard to a file: `pbpaste > game.log`.
4. `scry review game.log` prints the final decks, VP and a turn table (`-json` for the full per-turn state). When the capture's `game.score.txt` sits next to the log, the seats get their original players' names and the score screen's landmarks are scored.

For many games at once, `web/capture.js` loads finished games by ID in your logged-in tab, drops AFK games (a player with fewer than 3 turns), and exports them as JSON lines for `scry import`, which saves and checks each one.

The bookmarks only read the page. Copy log refuses rated games, because a game you load to review is always unrated. Don't use it on games in progress: the dominion.games Terms of Service ban tools that give a competitive advantage (§2A).

## How logs are read (cartographer)

`cartographer/` is the parser and game-state module.

- **Lines.** Each log line is matched against the client's own templates (244 of them, e.g. `BUY_AND_GAIN` "Argument0 buys and gains Argument1."), most specific first. Card lists are read with the client's singular, plural and "a/an" forms.
- **Ownership.** Every change of ownership is replayed: gains, trashes, returns to the Supply or the Box, Masquerade passes, Traveller exchanges, Inheritance. The log doesn't say where some cards come from, so the line that caused an event decides:
  - exiling or trashing a card from the Supply (Invest, Transport, Way of the Camel, Gladiator...);
  - cards that come back right after being trashed (Fortress, Lich, a Trail played from the trash);
  - Loot hidden from your seat but revealed just before (Pickaxe).
- **Per turn.** `Turns()` gives coins, buys, gains, trashes, Actions played, cards drawn and shuffles for each turn. It also gives every player's deck and counters after the turn: Coffers, Villagers, Favors, debt, VP tokens.
- **Zones.** Each turn also says where every card is after its Clean-up: hand (the next hand), draw pile, discard pile, in play, set aside, the Tavern, Island and Native Village mats, Exile.
  - Where the log doesn't say where a card comes from, the template and the card that caused the line decide. Mine gains to the hand, Supplies gains onto the deck, a Horse returns itself rather than the Horse it drew, Wandering Minstrel topdecks what it revealed.
  - Durations stay in play until their last effect. A Throne Room stays as long as the Duration it played twice; "the next time" cards (Secluded Shrine, Cutthroat, Search, Flagship) stay until they fire; Gear, Haven and the like leave at once if they set nothing aside.
  - Cards the log doesn't name, such as an opponent's "draws 5 cards", are counted per zone as unnamed, and the turn lists what they can be. Known cards on top of a deck (topdecks) and at its bottom (Shadow cards) are tracked, so a hidden draw of a known top card is named.
  - The log hides some of your own moves: Scheme's and Merchant Camp's "topdecks a card" in Clean-up. These are guessed from the cards in play.
- **Supply.** `Supply()` gives each pile's starting size and what's left, plus the trash. Sizes follow the setup rules: split and rotating piles, Knights, Castles, Ruins, and piles outside the Supply. Every gain, return, Supply trash and exile changes them.
- **VP.** `Client.Score()` scores each player from their deck, VP tokens and Misery, including:
  - the cards whose worth depends on the deck: Gardens, Duke, Fairgrounds, Feodum, Silk Road, Vineyard, the Castles, Pasture, Marchland, Territory, Demesne, Distant Lands on the Tavern mat;
  - the landmarks that score at the end, and Plateau Shepherds.
- **Checks.** `scry check` compares saved logs with their score screens: every line must be read, and each player's last turn and final deck must match. On 2026-10-11 all 220 imported games and all test logs pass. `scry check -prior FILE` also compares computed VP with the final scores in a saved DomBot `!prior` list.

### What the saved games showed

- **Seats.** The account that loads a game takes the first player's seat, and the bot takes the second. In 108 of the 220 games the first seat was the opponent's, so the log's "gamrevinu" is the opponent there. `SeatNames` maps the names back using the table's "Original Player" lines.
- **Final scores.** Computed VP equal DomBot's final scores in 153 of 207 games. In the other 54, the captured log doesn't end where the game did:
  - the game's last decision is missing, often the buy that ended it (6 VP short: the last Province);
  - or the bot kept playing its seat after the load. In 20 of these games, a shorter part of the log scores exactly what DomBot reports.

  Fixing this needs a change to the capture on dominion.games.
- **Zones.** On the 220 games plus the test logs (137,923 log lines):
  - every player's zones always hold exactly the cards they own;
  - 270 moves didn't find the card where the replay expected it, 116 of them for seats whose cards the log names in full;
  - after a turn, the hand equals the Clean-up draw in 3,638 of 3,641 checked turns.
- **Supply.** At the end of game 184518886 every pile matches the client's piles.

The tests check that:

- every test log passes `scry check`'s rules;
- the turn-by-turn replay ends at the final decks, and no balance ever goes negative;
- every player's zones add up to their deck after every turn, the next hand is the Clean-up draw, and cards out of place stay rare;
- Supply piles and final VP match the client and DomBot for 184518886;
- every template is reachable;
- every card reads in every form.

Set `SCRY_GAMES` to a folder of saved games to run the replay tests on those too.

Not covered yet:

- getting the log to end exactly where the game ended;
- Possession, and cards played from other players' decks.

## Hosted version

`go run ./cmd/buildsite` builds a browser-only copy into `site/`: the formatter compiled to WebAssembly plus one HTML page. Any static host can serve it. It formats pasted chat and pasted DomBot replies, and can't ask DomBot itself, since that would DM from whoever's token is on the server.

## Getting the inputs

1. **Match chat:** after the match, select the whole chat box on dominion.games, copy it, and paste it. Lines like `Match: Alice 1, Bob 0` mark the end of each game.
2. **Game results (optional):** pick one of these.
   - **Paste:** DM DomBot on Discord with `!match Alice, Bob`, copy its reply and paste it into the second box. DomBot's `!match` only covers the last day or two.
   - **Automatic:** tick *Ask DomBot automatically* and the tool sends `!match` for you (setup below).
   - **Neither:** you get the chat split by game with winners and running score.

## Automatic DomBot lookups (opt-in, read this first)

Automatic mode sends the DM from **your own Discord account** using your user token. Discord's rules forbid automating user accounts, and your account could be restricted. The tool only ever messages the DM channel whose recipient is the DomBot bot account. It sends one message per lookup and never prints or stores the token.

To set it up:

1. DM DomBot once from the Discord app so the DM channel exists.
2. Get your token: open Discord in a browser, open DevTools > Network, click any `/api/` request, and copy its `authorization` request header. Logging out of Discord invalidates it.
3. Make it available to scry, in either of these ways:
   - macOS Keychain (prompts for the value, keeps it out of shell history): `security add-generic-password -s scrying-pool -a discord -w`. The older item name `dombot` still works.
   - environment variable: `DISCORD_TOKEN=...`

## CLI

```bash
scry format -dombot reply.txt chat.txt > notes.md     # pasted DomBot reply
scry format -ask chat.txt > notes.md                  # fetch it from DomBot
scry format chat.txt > notes.md                       # chat only
scry match Alice Bob                                  # print DomBot's !match reply as text
scry ask '!game 184206813'                            # any DomBot command; -raw FILE keeps the JSON
```

## Kingdom viewer and card data

Each game section shows its kingdom: piles sorted by cost, then landscapes (Events, Projects, Landmarks, Ways, Allies, Traits, Prophecies), framed in their card-type colours. Click a card to open its page on the [Dominion Strategy wiki](https://wiki.dominionstrategy.com).

- `cards/cards.json` lists 819 cards with names, types, costs and sets. It comes from [dominiontabs](https://github.com/sumpfork/dominiontabs) (MIT); regenerate it with `go run ./cmd/carddb`. Card text isn't included.
- Card art belongs to Rio Grande Games, so it isn't in this repo or the hosted page. The local app downloads art thumbnails from the wiki the first time it shows each card. It keeps them in your user cache folder (`~/Library/Caches/scrying-pool/art` on macOS; `SCRY_ART_DIR` overrides it). To fetch them all at once (about 11 MB), run `scry cards sync`.

## Notes

- The web UI listens on `127.0.0.1` only and refuses cross-origin requests, because one request can send a DM from your account.
- Markdown is rendered in the browser with [marked](https://github.com/markedjs/marked) 18.0.14 from jsDelivr, pinned with a subresource integrity hash.
