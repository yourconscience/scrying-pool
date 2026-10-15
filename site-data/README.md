# Published profiles

The profiles and the Recent games page on https://yourconscience.github.io/scrying-pool/. Each push to `main` rebuilds the pages from `scry/web/` and these files (`.github/workflows/site.yml`).

- `players.json` and `<player>.json`: the profiles. They hold win rates only: no list of games and no opponents' names.
- `recent.json`: the first player's 25 latest games for the Recent games page, with each opponent's name and level, the scores and a few tempo numbers. dominion.games shows the same on the player's profile.

To update the Recent games list from any machine, use its **Update** button on the live page: it opens dominion.games, where the Recent games bookmark sends your profile's 25 latest games back. **Publish** on the same page writes them to `recent.json` here (it needs a GitHub fine-grained token with Contents read and write for this repository), and the site rebuilds. Games without a saved log then show the profile's result, scores and levels only.

To publish everything from your saved games, profiles and tempo numbers included, rebuild these files and commit them:

```bash
scry site -players "gamrevinu" -o site/my-games
```

```bash
cp site/my-games/data/*.json site-data/
```

Add a player by listing them in `-players`; ask before you publish someone else's profile.
