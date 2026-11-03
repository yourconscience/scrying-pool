# Reading finished games: plans and position evaluation (design)

Back to the [README](../README.md). It builds on the [match report](match-report.md), and it changes one decision of the [simulator plan](simulator.md).

Status: design, not built. The numbers were measured on 2026-11-03 with one-off scripts that aren't in the repo. Feedback wanted on the kingdom rules, the combo table and the decisions at the end.

## The problem

The report judges a game by markers that fit only a game about Provinces: first Gold, first Province, $ per turn, the VP gap. On boards with other ways to score, or with an engine that greens late, they tell the wrong story.

Measured on the 481 decided games among 518 saved:

| What | Result |
|---|---|
| Winners with 0-1 Province | 36% (4 or more: 40%) |
| Winners with under half their points from Province and Colony | 35% |
| Winners who never bought a Gold | 44% (losers: 47%) |
| The player with the earlier Gold won | 55% of 167 |
| The player with the earlier Province won, little alt VP in the game | 61% of 146 |
| The same, 10 VP or more of alt VP in the game | 51% of 72 |
| The player with more Provinces won, little alt VP | 91% of 226 |
| The same, 10 VP or more of alt VP | 75% of 101 |

Three real games show what this costs:

| Game | The report's reading | What the logs show |
|---|---|---|
| 185440489, lost 47-59 to Aw-Neb | No Gold, no Province; turning point t3-5 | 32 of the 47 VP were tokens. You came back from behind and led by 6 at t13. The evaluation below gives you 64-82% on t13-14. Aw-Neb's last turn swung 16 VP and ended the game on piles. |
| 185400051, lost 5-9 to heth | Turning point t12-15 by the VP gap | You led on VP until heth's last turn, because heth trashed all three Estates by t7. The evaluation has you behind from t4 (36%), and at 20% by t8. |
| 185438923, lost 23-35 to Scheiszegal | "Deck", hands normal | 23 of their 35 VP were Castles, split 6:2. The question is the Castle split, not Gold. |

Other weak spots:

- **Roles are presence flags.** A role says a kingdom card can do something, not what the player built. The research of 2026-10-23 found that presence doesn't separate winners from losers: 84% of winners and 84% of losers took a trasher from the kingdom.
- **Landscapes have no roles.** Bonfire, Gamble, Looting and Collection's on-gain tokens are invisible to the per-play measure.
- **Draw luck counts only treasure,** so a deck that pays with Actions looks unlucky. It barely predicts the result: 58% against 60%.
- **The result comes from the row's scores.** 37 games read as ties, and some are really resignations (185440400 is 0-0 at t8). Who resigned isn't saved.

## What the sources say

Read for this design, in their own terms:

- [Generalized Kingdom Analysis](https://dominionstrategy.com/2021/03/27/generalized-kingdom-analysis-and-some-ramblings-about-teaching/): think backwards. How do I score, what must my deck do for that, how do I build it, what could disturb the plan. Alt VP is any scoring beyond Province, Duchy and Estate, and the first question is whether it's usable on this board.
- [Minimal Jargon](https://dominionstrategy.com/2017/11/23/how-to-win-at-dominion-with-minimal-jargon/): payload cards win, cycling cards let you play the payload more often. Most games need both.
- [What is an Engine](https://dominionstrategy.com/2017/11/06/dominion-101-what-is-an-engine/), [The Five Fundamental Deck Types](https://dominionstrategy.com/2013/01/23/the-five-fundamental-deck-types-the-engine/): an engine is strong draw plus payload, with villages when the draw or payload is terminal. Thinning, gains and +Buy feed it. Modular pieces make an engine; needing specific cards makes a combo.
- [When Should I Start Greening?](https://dominionstrategy.com/2019/03/31/when-should-i-start-greening/): it depends on how fast the deck improves. A one-buy deck greens when it usually hits $8, an engine around $13, $16 or $18. [Engine Theory](http://wanderingwindergames.blogspot.com/2015/05/engine-theory-consistency-and-what-you.html): don't green until an ending is in sight; consistency comes from thinning and sifting.
- [A Guide to Alternate Victory Points](https://dominionstrategy.com/2012/08/27/a-guide-to-alternate-victory-points/): alt VP plays as a rush, a slog or support. Support is the usual case: more points on the board make the Province split matter less, which gives engines time.
- Combos, each with its own plan:
  - [Hermit/Market Square](https://dominionstrategy.com/2013/02/11/combo-of-the-day-30-hermitmarket-square/): one prepared megaturn, with named targets for each card and a deck of 29 cards or fewer.
  - [Guildhall/Beggar](https://adamhorton.com/flog/guildhall-combos-beggar-delve-and-masterpiece/): fifth Province around t12, eighth around t16 without collisions.
  - [Native Village/Bridge](https://dominionstrategy.com/2012/01/02/combo-of-the-day-27-native-villagebridge/): a whole-game plan; watch the piles.
  - [Souk/Gamble](https://forum.dominionstrategy.com/index.php?topic=12757.1700): only a forum note so far, so it stays low confidence.

The [Dominion Strategy wiki](https://wiki.dominionstrategy.com/index.php/Combo) sits behind a proof-of-work check for bots. A search index's copies gave us its Engine, Combo and Terminal space pages. The others stayed closed and we don't get around the check, so its per-card "works with" lists aren't used.

## The design

Five layers, each usable before the next exists.

### 1. Score and clock

Facts from the replay, for both players after every turn:

- VP by source: Province, Duchy, Estate, each alt Victory card, Curses, and tokens by the card or landscape that gave them. `Client.Score` has all but the token source.
- VP per turn over the last 3 turns.
- Who controls the ending: Provinces left, low and empty piles, gains per turn, and whether the leader can end the game next turn.

Game cards show "how each player scored" where they now show first Gold and first Province.

### 2. Kingdom read

Which pieces the board offers. Each piece is measured from the logs, and a small hand-made table covers the 193 landscapes, which have no plays.

| Piece | Rule |
|---|---|
| Net draw | Draws 2 or more per play, so it nets a card after replacing itself. Strong: 3 or more. Terminal or non-terminal by its +Actions. |
| Village | +2 Actions or more per play; also Villagers and Throne Room effects. |
| Trasher | Trashes a card or more per play. Fast: 2 or more, or from the start of a shuffle. |
| Gain or +Buy | +Buy on most plays, or a gain per play. Cost reducers are marked apart. |
| Payload | What the deck is for: $ with +Buy, gains, attacks, VP tokens. |
| Alt VP | Kingdom Victory cards, measured token sources, Landmarks, Colony. |
| Pile pressure | Cheap gainers, junkers, split piles, Castles. |

From the pieces come plan hypotheses, each with its reason and source, and several can stand at once:

- **Engine:** net draw with villages, or non-terminal draw, plus gain or +Buy, plus payload. Trashing makes it faster, and its absence is named as the risk.
- **Money:** control is missing. The report gives the best money baseline for the board (below), so "one Province a turn" has a number to beat.
- **Alt VP:** rush, slog or support, by how many points it adds and how often it can be played.
- **Combo:** a match in the combo table.

**Combo table.** `scry/strategy/combos.json`, hand-written: the cards, the plan in our own words, targets to check in a game (how many of each, when), the par when a source gives one, counters, a confidence, and the source's link. No card text. It starts with about 20 entries: Hermit/Market Square, Guildhall/Beggar, Native Village/Bridge, Bridge or Bridge Troll with a strong village, Souk/Gamble and the wiki's combo list.

The corpus can't judge combos: Souk with Gamble appears in 0 of 518 kingdoms, Hermit with Market Square in 0, Beggar with Guildhall in 1, Bridge Troll with Native Village in 1, Bridge or Bridge Troll with one of 24 listed villages in 12. So the table rests on the sources, and the report words a match as a hypothesis.

**Money baseline.** `scry sim play -a bm -b bm+X -kingdom ...` already runs in about 0.3 s per 2,000 games. On a mixed test kingdom, BM+Smithy beats BM 1407-493. On the first heth kingdom, Fortune Hunter beats BM 1489-422, Cobbler 1286-625 and Hunting Grounds 1163-728. BM+Native Village loses all 2,000, which is the bot failing to play the card. So the baseline is a floor: take the best X, and never read a bad result as a bad card. It still needs a count of turns to 4 Provinces.

**Does "all the pieces, so build" hold in our games?** We can't tell yet. A builder (a turn with 6 or more Actions played by t10) against a non-builder (3 or fewer in every such turn) won 74% of 58 games:

| Kingdom | The builder won |
|---|---|
| Control, trasher and gain or +Buy | 73% of 22 |
| 2 of those | 75% of 28 |
| 0-1 of those | 75% of 8 |
| Village and net draw | 83% of 24 |
| No village, no non-terminal draw | 81% of 16 |

The cells are small. Builders who managed without a village also show the flags miss cantrips, Villagers and landscapes. Where levels are known, builders won 65% against 53% expected (26 games). So the rule stays a source-based hypothesis, tested by a strong player's ratings (below).

### 3. What each player built

Counts and timing, not presence:

- Cards by role over the turns, and how many turns the deck took to get them.
- Actions played per turn, the share of the deck seen, stop cards.
- Collisions, now that both seats name their hands:
  - terminals left in hand;
  - draw without Actions, villages without draw;
  - a trasher without a target, and targets without a trasher.

  "Apprentice and Estate didn't meet for 7 hands" becomes a line the report writes for any board.
- When greening started, against the $ and +Buy the deck was reaching.
- A label for the build (engine, money, alt VP, rush, combo, hybrid) set by thresholds measured on the corpus.

The report then compares: the board suggests this; you built that; they built this.

### 4. Position evaluation

A chance to win after every turn, from public information. It's a logistic model on turn-boundary features, with its weights embedded in Go, so inference is a dot product. Results are for held-out games: 5-fold cross-validation grouped by game, repeated 3 times, on 13,909 positions from 481 games.

| Features | Log loss (coin 0.693) | AUC | t1-4 | t5-8 | t9-12 | t13-16 | t17+ |
|---|---|---|---|---|---|---|---|
| Seat and turn | 0.687 | 0.54 | 0.687 | 0.686 | 0.687 | 0.687 | 0.692 |
| VP and piles | 0.640 | 0.65 | 0.687 | 0.687 | 0.649 | 0.528 | 0.418 |
| + deck size and money density | 0.625 | 0.69 | 0.692 | 0.675 | 0.607 | 0.513 | 0.407 |
| + recent $, Actions, gains, VP | 0.608 | 0.72 | 0.695 | 0.652 | 0.577 | 0.489 | 0.395 |
| + role counts in the deck | 0.600 | 0.72 | 0.695 | 0.642 | 0.571 | 0.483 | 0.336 |
| **+ both** | **0.596** | **0.73** | 0.697 | **0.638** | **0.563** | **0.477** | **0.332** |
| + both + simulated turns | 0.600 | 0.73 | 0.696 | 0.637 | 0.573 | 0.490 | 0.338 |

- **Gain.** The chosen model beats VP and piles by 0.044 (95% interval over games: 0.025 to 0.063). It predicts the winner in 62% of positions at t5-8, 71% at t9-12 and 76% at t13-16. The Brier score goes from 0.227 to 0.207.
- **Calibration is good as it stands.** Where it says 0.75, players won 0.73; at 0.85, 0.86; at 0.25, 0.27. The worst bin is off by 3 points.
- **t1-4 stay flat:** nothing beats a coin there, so the bar shows only the seat and level prior and says it's too early.
- **Noise.** Over one round at t5-12, the chance moves a median of 6 points, 17 at the 90th percentile and 26 at the 97th. A move of 17 or more in a round is marked as a swing. Swings replace the VP-gap turning point.
- **Levels.** On the 230 games with levels, adding them to the VP-and-deck model takes log loss from 0.632 to 0.607. They enter as a fixed offset where known.
- **More games of this kind won't help much.** Training on 60 games gives 0.66, 120 gives 0.61, and 180 to 385 stay at 0.60-0.61. The next gain has to come from card-aware features, not volume. The model is fitted mostly on one player's games, so its fit to other populations is unknown.
- **What failed.** Simulating turns from each deck, with each card's measured average effect (chance of $8, of two Provinces, of drawing the deck), adds nothing once role counts and recent turns are in. It's left out. Its residual, actual $ minus expected $, reaches AUC 0.64 on the result, but it averages +0.6 $ a turn, so it's measuring deck power the simulation misses, not luck. It can't replace draw luck.

**Luck.** The treasure-only number stays until a hand measure passes two tests: its mean is zero, and it doesn't correlate with deck strength. Both seats now give every hand, so the measure can compare each hand with other hands from the same deck.

### 5. Decisions, later

- **Rule flags first.** Penultimate Province, greening before the deck reaches its target, more terminals than Actions, leaving the ending to a leader who can take it. Each flag is counted on the corpus, with and without, before it's shown.
- **Buy review after that.** A buy only adds a card, so the evaluation can compare the actual buy with alternatives (the research of 2026-10-23). It's shown only when a bootstrap interval excludes zero, and only after a strong player has rated a sample. A model of this size may just prefer whatever correlates with winning, so this layer has the highest risk.

## The report, changed

The first heth game would read like this; the plan lines are worded as they'd come out of layers 2 and 3, and the pieces in them still need the rules above:

```text
## Game 1 (185400051): lost 5-9 to heth, second seat

Board: engine pieces all here (Bridge Troll with Native Village is a known pairing; Apprentice trashes, Develop gains)
Money baseline: BM+Fortune Hunter
Built: you 2 Apprentices by t7, 53 cards played t5-14; heth 2 by t4, 85 cards played
Collisions: Apprentice in hand t7, t9; Estate in hand t6, t8, t10, t11; first together t12
Scored: you 2 Estates, a Duchy; heth Duchies, all on the last turn, which ended the game
Chance: 45% t1, 36% t4, 20% t8, 26% t14, then heth's last turn
Swings: none above the noise until heth's last turn; the fall from t4 to t8 is slow
Hands: normal (+0.07, heth +0.09)
```

The verdict "draw / deck / unclear" gives way to three lines: plan, clock, hands.

## How we'll know it's right

- **The evaluation.** Held-out log loss of 0.60 or less, beating VP and piles with an interval above zero, each bin within 5 points. Measured again when the corpus grows.
- **The kingdom read.** A strong player writes the plan for 20 kingdoms blind, and the read must agree on at least 15. Disagreements become rule changes. The model that writes the read never grades it.
- **The game read.** For 10 recent games you mark each line right, wrong or useless. The target is 80% right and at most 10% wrong.
- **Each flag.** Its win-rate difference is measured on the corpus and printed with the flag.

## The simulator plan

- **Decision 3 changes.** The plan says straight to the network, with no stage of hand-made evaluation. This design puts a small model first: it works now, it explains itself, and the network's start (M5) is still open. The network stays the goal for strength and for buy review.
- **M6's bar moves.** The network must beat log loss 0.596 on these games, not 0.639.
- **The engine earns its keep sooner,** through money baselines and later scripted plans for the combo table's pars.

## Data fixes

- **Real results.** Check whether the server's profile result names the winner or who resigned, which needs a logged-in look, and save it. Until then a resignation is read from the scores.
- **Second seats.** 28 of 518 games have them. `collect -other 30` took 8 minutes, so the nightly run could take far more than 30 and finish the backfill in days instead of about 16 nights. Say how many is fine for the account: the capture notes already call hundreds of games a night heavy use.
- **Logs that end differently.** 2 of 30 merges failed because one seat's log runs longer at the end. Merge the common part.
- **Token sources,** by the card or landscape that gave each token.

## Build order

Effort is an estimate.

1. Score sources and the new game card wording: half a day.
2. The evaluation: export, fit, embedded weights, chart, swings: 2-3 days.
3. Kingdom read, landscape table, combo table with 20 entries, money baseline: 3-4 days.
4. What each player built, with collisions: 2 days.
5. Rule flags, each measured: 2 days.
6. Buy review, after the ratings: unknown.

## Not in this change

- Anything during a game. Finished games only, as the [SPEC](../SPEC.md) says.
- Free-text commentary from a model. It may come later on top of these facts, labelled as a hypothesis.

## Decisions needed

1. Accept the change to decision 3 of the simulator plan?
2. Who rates the 20 kingdoms: you, or a stronger player you trust?
3. How many second-seat games a night?
