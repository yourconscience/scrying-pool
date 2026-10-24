# Simulator: a Dominion rules engine for position evaluation

Status: draft for review, 2026-10-23; the decisions at the end are agreed. M0 is built: the engine with the base set, random and Big Money bots, `scry sim play` and `scry sim fuzz`; the rest is the plan. Back to the [README](../README.md); how the log reader works is in [internals.md](internals.md).

## Goal

One thing: for any position in a finished two-player game, each player's chance to win from the board as it stands. That is the evaluation bar on the analysis page, after every turn of a saved game.

To learn it, a network needs millions of positions with known outcomes, more than any set of logs gives. So the plan is the one the Temple Gates app's AI follows ([their write-up](https://www.templegatesgames.com/dominion-ai/)):

- a rules engine in Go plays complete games with every card on dominion.games;
- the engine asks an agent for every choice, so bots play each other at scale;
- a value network learns from those games;
- the network then evaluates the positions Cartographer replays from real logs.

Rollouts from real positions and kingdom analysis come almost free once the engine exists, but they're not part of this plan.

How we know it works:

- **Coverage:** the share of the 819 cards in `scry/cards/cards.json` that have rules, and the share of saved games whose whole kingdom is covered.
- **Conformance:** replaying a real saved game through the engine gives the same log entries and the same zones as Cartographer's replay, turn by turn.
- **Determinism:** the same seed and the same decisions give the same game, byte for byte.
- **Evaluation:** on held-out real games, the calibrated network predicts the winner better than the logistic baseline from the research (log loss 0.639, where a coin is 0.693). It is measured by Brier score and log loss.
- **Speed:** random-bot games per second, and how long a state copy takes. Both are measured, never assumed.

## Not in scope

- **Anything near a live game.** No client for dominion.games, no connection to its servers, no advice during a game. The ToS rules in [SPEC.md](../SPEC.md) still apply: the engine only ever looks at finished games.
- **Playing against the bots.** Nobody plays the engine: there is a debug CLI to step through a bot game, but no game UI, accounts or online play. Dominion belongs to Rio Grande Games; a public playable clone is a different matter from an analysis engine. The repo stays without card text and art, as now.
- **First-edition cards come last.** 48 of the 819 cards exist only in the first editions (Silk Road, Great Hall, Sea Hag, the Prizes...). None appears in our saved games. Whether dominion.games still deals them needs checking; either way they come after everything else.
- **More than two players.** The state holds exactly two players, and rules that only matter with three or more (who is to the left, attack order) aren't written.

## The scope, measured

These numbers are measured, not guessed. The scripts are one-offs in the session scratchpad and aren't part of the repo.

- **Cards.** `scry/cards/cards.json` has 819 cards:
  - 503 kingdom cards and 113 others outside the Supply (Spirits, Loot, Horses, Traveller upgrades, Rewards, Heirlooms, Shelters, Ruins, Knights...);
  - 79 Events, 23 Allies, 21 Landmarks, 20 Projects, 20 Ways, 15 Prophecies, 15 Traits;
  - 10 basic cards.

  48 of the 819 exist only in the first editions.
- **How hard they are.** Measured on the card text in dominiontabs, which matches all but 2 of our cards:
  - only 17 cards are plain "+N Cards / Actions / Buys / $";
  - 501 have at most 25 words, 281 have 26-50, and 18 have more; the median is 22 words.
- **Mechanics that recur.** Counted by phrases in the text:
  - "gain" in 353 cards, "you may" (an optional choice) in 222, "trash" in 175, "reveal" in 106;
  - "each other player" (attacks) in 87, "when you gain" (triggers) in 86, "next turn" (Durations) in 73.
- **Partial coverage is useless.** Our 251 saved games with a kingdom line use 642 distinct cards and landscapes. Adding cards so as to complete the most games at each step:

  | Games fully playable | Cards and landscapes needed |
  |---|---|
  | 10% (26) | 178 |
  | 25% (63) | 337 |
  | 50% (126) | 489 |
  | 90% (226) | 613 |
  | 100% (251) | 642 |

  Self-play on random kingdoms needs all of them.
- **A reference for size.** pydominion (Python, no license, read for rules only) has 812 card, event and landscape files. They have a median of 64.5 lines each including their inline tests, 55k lines in all, and its Game, Player and Card core is about 3k lines.

## Design

### Where it lives

- `scry/internal/engine/`: the state, the rules core, decisions, and log output.
- `scry/internal/engine/cards/`: one Go file per card, Event or landscape, registered by name. One file per card keeps reviews and agent work small and parallel.
- `scry/internal/bots/`: agents. Random, Big Money, Big Money + X, network-guided search.
- `scry/internal/evaluator/`: the network's inference in Go, and the conversion from Cartographer's replay to the network's input.
- `scry/sim.go`: the `scry sim` command.

The engine reads card names, types, costs and sets from `scry/cards/cards.json`, so there is one card table. The rules for each card are Go code; no card text goes into the repo.

### State

- **Cards as numbers.** A card kind is a `CardID` (an index into the card table). Every physical card in a game is an `Inst` with its own number. Instances matter:
  - for Durations and set-aside cards;
  - for tokens on cards and Traveller exchanges;
  - for the "lose track" rule.
- **Zones.** Each zone is an ordered list of instances: draw pile, hand, discard, in play, set aside (with the card that set it aside), the Tavern, Island and Native Village mats, Exile, the trash, and the piles in and out of the Supply. Piles are ordered, which covers split and rotating piles, Knights, Castles and Ruins.
- **Per player:**
  - Coffers, Villagers, Favors, debt and VP tokens;
  - States, Artifacts, owned Projects, Boons kept until Clean-up;
  - the journey token, and the Adventures tokens on piles;
  - the Estate's ability from Inheritance.
- **Per game:**
  - the turn and its phase, the extra-turn queue, triggers waiting to resolve;
  - Traits on piles, the Prophecy and its Sun tokens, the Boon and Hex decks;
  - the random generator.
- **Randomness.** All of it goes through one seeded generator: shuffles, Boons and Hexes, Ruins and Knights at setup, Loot. A game is fully reproducible from its seed and its decisions.
- **Copying.** `Clone()` copies the whole state as flat slices, with no pointers into the old copy. Its cost is measured in M0.

### Decisions are the only way the engine asks anything

The engine never chooses for a player. Each choice is a `Decision`, and an agent answers it:

```go
type Decision struct {
	Player  int      // who answers; with Possession, the possessing player answers for the turn
	Kind    Kind     // PlayAction, PlayTreasures, Buy, ChooseCards, ChooseOption, Order, NameCard, YesNo, ChoosePile, React...
	Source  CardID   // the card asking
	Options []Option // the legal answers, never more
	Min     int      // how many to choose
	Max     int
}

type Agent interface {
	Decide(v *View, d *Decision) []int // indexes into d.Options
}
```

- **`View` is the player's information set.** It holds their own hand and known draw-pile tops, their own deck composition (not its order), what every player has in play, all piles, the trash, tokens and everything public. Agents never see the full state, so hidden information is enforced by the type, not by convention.
- **Card code reads straight down.** A card's effect is ordinary Go that calls `t.Ask(decision)` and goes on with the answer, as pydominion's cards do. Writing 819 cards as hand-made state machines would cost several times more.
- **Search clones at turn boundaries and re-plays the rest.** With straight-line card code, a game stopped in the middle of an effect can't be copied, since the effect lives on the Go stack. So:
  - a search node is a state copied at the start of a turn, plus the answers given so far in that turn;
  - to reach a node, the engine copies the snapshot and replays the turn with those answers;
  - with a fixed seed that is exact, and replaying at most one turn is cheap. The cost is measured in M0.
- **What this requires of cards.** Anything that outlives a turn is data, never a closure: a Duration is an entry (instance, what's left to do) that the card's `StartOfTurn` hook reads; "the next time you gain" effects are entries too. This rule is checked in review.

### Rules core

- **Turn structure:** start of turn (Durations, Boons, Prophecy effects), Action phase, Buy phase (Treasures, Coffers, buys of cards, Events and Projects, overpay), Night phase, Clean-up, then end-of-turn effects and extra turns. Outpost, Mission, Voyage, Seize the Day, Fleet and Possession share one extra-turn queue, and each card keeps its own condition (Outpost, Mission and Voyage give a turn only if the previous turn wasn't yours).
- **Triggers:** one ordered list of hooks.
  - On buy, on gain (where a card can be redirected, as with Watchtower, Tracker or Sleigh), on trash, on discard outside Clean-up, on play, on shuffle, when revealed.
  - When attacked: Reactions such as Moat and Guard Dog, and Lighthouse's protection.
  - Calling Reserve cards from the Tavern.
  - When several triggers fire at once, their owner orders them through an `Order` decision, as the rules say.
- **Moving cards.** All movement goes through `Move(inst, from, to)`, which does nothing if the card is no longer where the effect expects it. That is the "lose track" rule, and it is what makes Throne Room on cards that move themselves come out right.
- **Cards played as other cards.** A physical card and the ability it uses are kept apart. That covers:
  - Band of Misfits, Overlord and Captain;
  - the Ways and Inheritance;
  - cards whose types or text change, such as Capitalism, Enchantress and Highwayman.
- **Costs.** A cost is `{Coins, Potion, Debt}`, computed through modifiers: Bridge, Highway, Quarry, Canal, Ferry's -$2 token and cards that change their own cost.
- **Scoring.** The engine has its own scoring of decks. Cartographer's rules in `vp.go` already agree with pydominion on every end-game card it scores, so they are the starting point. Later, Cartographer can score through the engine so there is one rule set.

### Log output, the key to checking it

The engine writes its log as dominion.games template entries: the template name (`BUY_AND_GAIN`, `PLAY`, `DRAW`...) and its arguments. These are the same 244 templates Cartographer reads. That gives three things:

- Cartographer reads simulated games with no new code, so the analysis page shows bot games too.
- Engine output can be compared with real logs entry by entry.
- When we can't tell what the engine did, the log says it in the same words as dominion.games.

Rendering templates into text needs the client tables. They stay out of the repo and are read from the user's config folder, as Cartographer does now (`scry client save`).

### Checking against real games

The outside judge is the 265 saved games, not the engine's own tests. `scry sim check` replays each saved game through the engine:

1. **Setup** comes from the score file: kingdom, landscapes, seat order and starting decks (Shelters, Heirlooms).
2. **A log agent** answers each decision so that the engine's next entries match the log: what was played, bought, trashed and chosen.
3. **Draws** come from a draw source instead of the random generator. For the seat whose draws the log names, they are exact. For the other seat:
   - the source takes the cards that seat plays, reveals or discards by name later in the same turn;
   - the rest of the hand is filled from the known deck composition.
4. **After every turn**, the engine's log entries are compared with the real ones, and its zones and piles with Cartographer's.
5. **The report** lists, per card and per template, where the two disagree. `scry check` reports the parser's results the same way.

Cartographer's checks already show the data is good enough for this: every player's zones hold exactly the cards they own, and the Clean-up hand matches in 3,638 of 3,641 turns ([internals.md](internals.md)).

Two cheaper checks run all the time:

- **Invariants**, asserted in a debug build: every instance is in exactly one zone, piles never go below zero, and costs and counters are never negative.
- **Random-bot fuzzing:** millions of random-bot games on random kingdoms. Any panic or broken invariant is saved as its seed and kingdom, so it can be replayed.

The repo's rule is not to add unit tests, so the checks are `scry sim check` and the fuzz command, not per-card tests. If a Go test for `scry sim check` on `scry/testdata/logs` is wanted, that's a separate decision.

### Agents

- **Random:** a uniform choice among legal options. Used for fuzzing.
- **Big Money and Big Money + X:** the canonical buy rules, and the same with one kingdom card X added, as DominionSim generates them for every card. One shared rule set answers choices inside cards: trash Curses, then Estates, then Coppers; discard Victory cards first. These bots are yardsticks for the network's strength, not training partners.
- **The network's search:** information-set MCTS guided by the network, as AlphaZero and Temple Gates do. Each simulation draws one shuffle of the hidden cards from the known composition, then uses `Clone()` and the turn re-play described above. There is no hand-made evaluation stage: the network is the evaluation from the start.

### The network

- **Two heads.**
  - **Value:** the first player's chance to win (two players, so one number; a tie counts as half). This is the bar.
  - **Policy:** a score for each legal option of a decision, used by the search.
- **Cards as learned vectors.** Each card is a learned vector of about ten numbers, after Temple Gates, so one network covers every kingdom and kingdoms it has never seen.
- **Value input: public information only.** It holds:
  - each player's deck composition, which is public, since every gain and trash is in the log;
  - the cards known to be in each zone from public events: cards played since the last shuffle are in the discard pile, and known top cards are on the deck;
  - the number of unknown cards in each zone.

  Hands are not in it. It's taken at turn boundaries, after Clean-up.
- **Why public information.** Cartographer already tracks exactly this for the seat whose draws the log hides. For the seat whose draws it names, the evaluator replays the log with the drawn cards' names masked, so both seats are treated alike. The bar is then the same whichever seat the log came from, and the network sees real and simulated positions the same way.
- **Policy input:** the deciding player's own `View`, including their hand.
- **Inference** runs in pure Go: small matrices and no cgo. Training runs outside the binary.

### Self-play and training

- **Games.**
  - `scry sim selfplay -games N -out DIR` plays network-against-network games on random kingdoms.
  - Files are compact and append-only. For every decision they hold the view's features, the legal options, the answer and the search's visit counts; for every turn boundary, the public view; for every game, the result.
  - A turn cap ends runaway games early in training, scored as they stand.
- **Training.** A one-off `uv run` job with PyTorch reads them, trains, and exports the weights in a plain file format that Go reads. Package versions stay pinned and at least 3 days old.
- **Head start.** Before any self-play, the policy head learns to copy the named seat's choices in our saved logs, through the same log agent `scry sim check` uses. Temple Gates started from no knowledge; we don't have to, and it saves the first rounds of near-random games.
- **The loop:**
  1. self-play with search guided by the current network;
  2. train on those games, using the final result and TD targets;
  3. keep the new network only if it beats the old one in a fixed match;
  4. repeat.
- **Strength:** fixed-size matches against Big Money, BM+X and earlier networks.

### From self-play to real games

- **Same input from both sources.** `scry/internal/evaluator` turns each `cartographer.Turn` of a saved game into the public view.
  - The check: an engine game is written as a log from one seat, as dominion.games writes it, Cartographer reads it back, and the view built from that replay must equal the engine's own public view at every turn boundary.
  - Any difference is a bug in the engine, the log output or Cartographer.
- **Calibration.** The raw value is the chance to win when the network plays itself, not when two level-55 players do. A small calibration layer fixes that: Platt scaling, with the rating-level difference as an offset where the score file has levels, fitted on our saved games' results. The fit is checked on held-out games.
- **The bar.** `analyze.html` shows the calibrated chance after each turn, with its range from the calibration's uncertainty. Turns where the chance moves more than the measured noise are marked as swings.

### Speed

Speed targets are set after M0, from real measurements. What we know:

- A toy Go engine played a whole Big Smithy against Big Money game in 3.1 µs on one M1 thread. That is a floor, not a forecast.
- The hot path avoids maps and allocations: cards are small integers, zones reuse their slices, and card hooks are looked up in a table by `CardID`.
- A state copy, a one-turn re-play and a random game are benchmarked on every milestone.
- Measured 2026-10-23 with the base set only, log off, one M1 thread: about 12,000 Big Money or BM+Smithy games per second, and about 3,000 random-player games per second (those run to about 170 turns).

## Plan

The milestones follow mechanisms, since each expansion brings its own. Effort is an estimate, flagged as such, and is re-estimated after M0 from how many cards per day actually pass `scry sim check`.

| Milestone | What | Done when | Effort (estimate) |
|---|---|---|---|
| M0 | Core: state, zones, turn structure, decisions, triggers, `Move`, costs, log output, clone and re-play, `scry sim check`, fuzzing, random and Big Money bots. The base set and Intrigue. | Random games run without broken invariants. Engine logs read back through Cartographer. Clone and re-play costs measured. | 3-5 weeks |
| M1 | Seaside (Durations), Prosperity (Colony, Platinum, VP tokens), Hinterlands (on-gain effects), Cornucopia & Guilds (Rewards, Coffers, overpay), Alchemy (Potions) | Every saved game that only uses these sets passes `scry sim check`. | estimated after M0 |
| M2 | Dark Ages (Ruins, Shelters, Knights, Spoils), Adventures (Reserve, Travellers, tokens, Events), Empires (debt, Landmarks, split piles, Castles) | as above | estimated after M0 |
| M3 | Nocturne (Night, Boons, Hexes, States, Heirlooms, Spirits), Renaissance (Villagers, Projects, Artifacts), Menagerie (Ways, Horses, Exile) | as above | estimated after M0 |
| M4 | Allies (Favors, Liaisons, rotating piles), Plunder (Loot, Traits), Rising Sun (Prophecies, Omens, Shadow cards), Promos, then Possession | All 771 current cards, then the 48 first-edition ones. 90% or more of the saved games pass `scry sim check`; the rest are listed with reasons. | estimated after M0 |
| M5 | The network and its Go inference, network-guided MCTS, the self-play file format, the imitation head start, the training loop, BM and BM+X yardsticks | The network beats Big Money and BM+X in fixed matches. | open; compute needs measuring |
| M6 | The public view from Cartographer and its round-trip check, calibration on saved games, the bar on `analyze.html` | On held-out real games the bar beats the logistic baseline (log loss 0.639). | 1-2 weeks |

**Writing 819 cards.** Card files can be written in parallel by coding-agent sessions on the user's subscriptions, one set per session. The steps for each card:

1. read the card's text locally (dominiontabs or the rulebook) and the rulings;
2. write the card file;
3. run `scry sim check` on the saved games that use the card.

A card counts as done when the games that use it pass the check, not when the session says it's done. My rough guess for all cards is two to three months part-time including debugging. That is an estimate to replace with the M0 measurement.

**Rules source.** The second-edition rulebooks and the published rulings decide; where they're silent, dominion.games' behaviour as seen in our logs does. The Dominion Strategy wiki's rulings sections are a good human reference, but they can't be read by our tools.

## Decisions, 2026-10-23

1. **The only product is the evaluation of the board.** Bots play each other to train it; nobody plays against them.
2. **Two players only.**
3. **Straight to the network.** Network-guided search from the start, with no stage of hand-made evaluation.
4. **The engine lives in this public repo,** as pydominion and DominionSim do.
5. **Ask pydominion's author for a license.** With one (MIT, say), its rules and tests could be ported directly instead of only read.
