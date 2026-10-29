# Published profiles

The profiles and the Recent games page on https://yourconscience.github.io/scrying-pool/. Each push to `main` rebuilds the pages from `scry/web/` and these files (`.github/workflows/site.yml`).

- `players.json` and `<player>.json`: the profiles. They hold win rates and, for the page's filters, each game's own record without the opponent's name or the game id: date, seat, result and both players' VP, ending, kingdom and its roles, kingdom cards taken, opening split and the cards gained on turns 1 and 2, first Gold and Province, $ on turns 1-15 and both rating levels; and each kingdom card's community rating (Dominion Card Glicko).
- `engine.json`: for the Engine page, the first player's 20 latest kingdoms played out by the rules engine, with each opponent's name, the scores and what the player bought.
- `recent.json`: the first player's 25 latest games for the Recent games page, with each opponent's name and level, the scores and a few tempo numbers. dominion.games shows the same on the player's profile.

To update the Recent games list from any machine, use its **Update** button on the live page: it opens dominion.games, where the Recent games bookmark sends your profile's 25 latest games back. **Publish** on the same page writes them to `recent.json` here (it needs a GitHub fine-grained token with Contents read and write for this repository), and the site rebuilds. Games without a saved log then show the profile's result, scores and levels only.

To publish everything from your saved games, profiles and tempo numbers included, rebuild these files and commit them. Card images are linked only for cards found in `card-images/` (or `$SCRY_CARD_IMAGES`) where you run it:

```bash
scry site -players "gamrevinu" -o site/my-games
```

```bash
cp site/my-games/data/*.json site-data/
```

The Engine page reads `engine.json`: Big Money with one card against plain Big Money in each kingdom of the latest 20 saved games, and how far the engine replays the saved games. It takes about a minute:

```bash
scry sim probe -player "gamrevinu" -o site-data/engine.json
```

Add a player by listing them in `-players`; ask before you publish someone else's profile.
