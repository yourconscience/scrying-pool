# dombot

Readable notes for [Dominion Online](https://dominion.games) matches. Paste the chat from a finished match and get Markdown split by game. Each game shows its result, kingdom, landscapes and empty piles from [DomBot](http://dominionleague.org/player-guide), followed by that game's chat, with the players' lines in bold.

Not affiliated with Shuffle iT, DomBot or the Dominion League.

## Run

Needs Go 1.22+.

```bash
go install github.com/yourconscience/dombot@latest
dombot web            # http://127.0.0.1:8765
```

Or from a clone: `go run . web`.

Click **Load demo match** to see a full example. It uses a real six-game match, with the real spectator chat.

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
3. Make it available to dombot, in either of these ways:
   - macOS Keychain (prompts for the value, keeps it out of shell history): `security add-generic-password -s dombot -a discord -w`
   - environment variable: `DISCORD_TOKEN=...`

## CLI

```bash
dombot format -dombot reply.txt chat.txt > notes.md   # pasted DomBot reply
dombot format -ask chat.txt > notes.md                # fetch it from DomBot
dombot format chat.txt > notes.md                     # chat only
dombot match Alice Bob                                # print DomBot's !match reply as text
dombot ask '!game 184206813'                          # any DomBot command; -raw FILE keeps the JSON
```

## Kingdom viewer and card data

Each game section shows its kingdom: piles sorted by cost, then landscapes (Events, Projects, Landmarks, Ways, Allies, Traits, Prophecies), framed in their card-type colours. Click a card to open its page on the [Dominion Strategy wiki](https://wiki.dominionstrategy.com).

- `cards/cards.json` lists 819 cards with names, types, costs and sets. It comes from [dominiontabs](https://github.com/sumpfork/dominiontabs) (MIT); regenerate it with `go run ./cmd/carddb`. Card text isn't included.
- Card art belongs to Rio Grande Games, so it isn't in this repo or the hosted page. The local app downloads art thumbnails from the wiki the first time it shows each card. It keeps them in your user cache folder (`~/Library/Caches/dombot/art` on macOS; `DOMBOT_ART_DIR` overrides it). To fetch them all at once (about 11 MB), run `dombot cards sync`.

## Notes

- The web UI listens on `127.0.0.1` only and refuses cross-origin requests, because one request can send a DM from your account.
- Markdown is rendered in the browser with [marked](https://github.com/markedjs/marked) 18.0.14 from jsDelivr, pinned with a subresource integrity hash.
- DomBot lists kingdom cards and landscapes (Events, Ways, Allies and so on) in one list, and dombot keeps it that way.
