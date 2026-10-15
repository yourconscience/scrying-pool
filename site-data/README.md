# Published profiles

The profiles and the Recent games page on https://yourconscience.github.io/scrying-pool/. Each push to `main` rebuilds the pages from `scry/web/` and these files (`.github/workflows/site.yml`).

- `players.json` and `<player>.json`: the profiles. They hold win rates only: no list of games and no opponents' names.
- `recent.json`: the first player's 25 latest games for the Recent games page, with each opponent's name and level, the scores and a few tempo numbers. dominion.games shows the same on the player's profile.

To publish new numbers, load your latest games (the Recent games bookmark), rebuild these files from your saved games and commit them:

```bash
scry site -players "gamrevinu" -o site/my-games
```

```bash
cp site/my-games/data/*.json site-data/
```

Add a player by listing them in `-players`; ask before you publish someone else's profile.
