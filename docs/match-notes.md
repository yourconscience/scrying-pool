# Match notes

Back to the [README](../README.md).

`notes.html` ([on the live site](https://yourconscience.github.io/scrying-pool/notes.html), and in `scry web`) turns a match's spectator chat into notes split by game. Each game shows its result, kingdom, landscapes and empty piles from DomBot, then that game's chat, with the players' lines in bold. **Load demo match** shows a real six-game match.

1. **Match chat:** after the match, select the whole chat box on dominion.games, copy it, and paste it. Lines like `Match: Alice 1, Bob 0` mark the end of each game.
2. **Game results (optional):** pick one of these.
   - **Paste:** DM DomBot on Discord with `!match Alice, Bob`, copy its reply and paste it into the second box. DomBot's `!match` only covers the last day or two.
   - **Automatic:** tick *Ask DomBot automatically*, and the tool sends `!match` for you (setup below).
   - **Neither:** you get the chat split by game with winners and running score.

Each game's kingdom is shown as piles sorted by cost, then landscapes, framed in their card-type colours. Click a card to open its page on the [Dominion Strategy wiki](https://wiki.dominionstrategy.com).

The live page is a browser-only copy: the formatter compiled to WebAssembly plus one HTML page, built with the rest of the hosted site by `go run ./tools/buildsite`. It can't ask DomBot itself, since that would DM from whoever's token is on the server.

## Automatic DomBot lookups (opt-in, read this first)

Automatic mode sends the DM from **your own Discord account** using your user token. Discord's rules forbid automating user accounts, and your account could be restricted. The tool only ever messages the DM channel whose recipient is the DomBot bot account. It sends one message per lookup and never prints or stores the token.

1. DM DomBot once from the Discord app so the DM channel exists.
2. Get your token: open Discord in a browser, open DevTools > Network, click any `/api/` request, and copy its `authorization` request header. Logging out of Discord invalidates it.
3. Make it available to scry, in either of these ways:
   - macOS Keychain (prompts for the value, keeps it out of shell history): `security add-generic-password -s scrying-pool -a discord -w`. The older item name `dombot` still works.
   - environment variable: `DISCORD_TOKEN=...`
