# Scrying Pool

**Look back into your finished [Dominion Online](https://dominion.games) games.**

<p>
<img src="https://raw.githubusercontent.com/blakevanlan/KingdomCreator/d9a802db2e41d52786eaa02097279cbf1b64c778/docs/img/cards/baseset2/baseset2_village.jpg" width="110" alt="Village">
<img src="https://raw.githubusercontent.com/blakevanlan/KingdomCreator/d9a802db2e41d52786eaa02097279cbf1b64c778/docs/img/cards/baseset2/baseset2_smithy.jpg" width="110" alt="Smithy">
<img src="https://raw.githubusercontent.com/blakevanlan/KingdomCreator/d9a802db2e41d52786eaa02097279cbf1b64c778/docs/img/cards/baseset2/baseset2_market.jpg" width="110" alt="Market">
<img src="https://raw.githubusercontent.com/blakevanlan/KingdomCreator/d9a802db2e41d52786eaa02097279cbf1b64c778/docs/img/cards/baseset2/baseset2_witch.jpg" width="110" alt="Witch">
<img src="https://raw.githubusercontent.com/blakevanlan/KingdomCreator/d9a802db2e41d52786eaa02097279cbf1b64c778/docs/img/cards/baseset2/baseset2_laboratory.jpg" width="110" alt="Laboratory">
</p>

Scrying Pool is a small app that runs on your own computer. Give it the number of a game you played, and it replays the game turn by turn. Save a few dozen games, and it shows what tends to win and lose games for you.

## Replay any game

Score, money and deck size for both players, turn by turn:

<img src="docs/img/analyze-charts.jpg" width="720" alt="A replayed game: VP, $ and cards owned after each turn">

Drag the slider to any turn to see what happened and where every card is: hand, draw pile, discard pile, mats. Cards the log doesn't name are counted, with a list of what they can be.

<img src="docs/img/analyze-turn.jpg" width="720" alt="One turn of the replay: the log and each player's cards">

## See what wins your games

Your win rate, the clearest patterns in plain words, and what changes your results: going first, a village in the kingdom, +Buy, trashing and so on. Green or red means the difference is clear, grey means it may be chance.

<img src="docs/img/profile-summary.jpg" width="720" alt="Profile: games, win rate, seat and the main takeaways">

<img src="docs/img/profile-effects.jpg" width="720" alt="Win rate with and without each kingdom feature, with 95% ranges">

The page also shows your form over the last 30 games, $ per turn in wins and losses, when you buy your first Province, and your best and hardest kingdom cards. You can put a second player next to you to compare.

## Notes from a match chat

Paste the chat of a finished match and get readable notes: one section per game with its kingdom, result and that game's chat.

<img src="docs/img/notes.jpg" width="720" alt="Match notes: one game with its result, kingdom and chat">

## Try it

You need a dominion.games account and [Go](https://go.dev/dl/) 1.26 or newer. Then, in a terminal:

```bash
go install github.com/yourconscience/scrying-pool@latest
```

```bash
scry web
```

Open http://127.0.0.1:8765. The first time, the app needs one bookmark from your dominion.games tab; [docs/capture.md](docs/capture.md) walks you through it.

## Good to know

- Your games stay on your computer. Nothing is sent anywhere unless you publish a page or turn on DomBot lookups.
- Finished games only: the dominion.games Terms of Service ban tools that help during a game.
- Card art belongs to Rio Grande Games. The images come from [KingdomCreator](https://github.com/blakevanlan/KingdomCreator) and aren't stored in this repo.
- Work in progress. Not affiliated with Shuffle iT, Rio Grande Games, DomBot or the Dominion League.

## More

- [Loading games](docs/capture.md): one game, hundreds at once, overnight runs.
- [Match notes](docs/match-notes.md): chat notes and DomBot results.
- [How it works](docs/internals.md): how logs are read, how the numbers are made, the CLI, where data lives.
- [SPEC.md](SPEC.md): the plan and the log of decisions.
