# Published profiles

The profiles on https://yourconscience.github.io/scrying-pool/. Each push to `main` rebuilds the page from `scry/web/` and these files (`.github/workflows/site.yml`).

They hold win rates only: no list of games and no opponents' names. To publish new numbers, rebuild them from your saved games and commit them:

```bash
scry site -players "gamrevinu" -o site/my-games
```

```bash
cp site/my-games/data/*.json site-data/
```

Add a player by listing them in `-players`; ask before you publish someone else's profile.
