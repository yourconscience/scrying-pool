# Simulator: a Dominion rules engine for self-play

Status: draft for review, 2026-10-23. Nothing below is built yet. Back to the [README](../README.md); how the log reader works is in [internals.md](internals.md).

## Goal

A rules engine in Go that plays complete games of Dominion with every card on dominion.games, and asks an agent for every choice. Bots can then play each other at scale, as the Temple Gates app's AI does ([their write-up](https://www.templegatesgames.com/dominion-ai/)).

What it is for, in order:

1. **Self-play training.** A value and policy network learns from bot-against-bot games. Its value output is the per-move evaluation bar for real games.
2. **Analysis of real games.** It starts from a position Cartographer replayed, then plays rollouts and asks "what if they had bought X" one turn ahead.
3. **Kingdom analysis.** It plays candidate strategies against each other on one kingdom: Big Money plus one card, engines, rushes.
4. **Benchmarks.** Bots of known strength give the network and later bots a yardstick.

How we know it works:

- **Coverage:** the share of the 819 cards in `scry/cards/cards.json` that have rules, and the share of saved games whose whole kingdom is covered.
- **Conformance:** replaying a real saved game through the engine gives the same log entries and the same zones as Cartographer's replay, turn by turn.
- **Determinism:** the same seed and the same decisions give the same game, byte for byte.
- **Speed:** random-bot games per second, and how long a state copy takes. Both are measured, never assumed.

## Not in scope

- **Anything near a live game.** No client for dominion.games, no connection to its servers, no advice during a game. The ToS rules in [SPEC.md](../SPEC.md) still apply: the engine only ever looks at finished games.
- **A playable Dominion for people.** There is a debug CLI to step through a bot game, but no game UI, accounts or online play. Dominion belongs to Rio Grande Games; a public playable clone is a different matter from an analysis engine. The repo stays without card text and art, as now.
- **First-edition cards come last.** 48 of the 819 cards exist only in the first editions (Silk Road, Great Hall, Sea Hag, the Prizes...). None appears in our saved games. Whether dominion.games still deals them needs checking; either way they come after everything else.
- **First version: two players.** The state allows up to 6, but rules that only matter with 3+ players (who is "left", attack order) are checked later.

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
- `scry/internal/bots/`: agents. Random, Big Money, buy-rule bots, search, network.
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
- **Big Money:** the canonical buy rules, as the floor.
- **Buy-rule bots:** priority lists with conditions. DominionSim's `DomBots.xml` (MIT, 198 bots) is read as data, with attribution. A "Big Money + one X" bot is generated for every kingdom card, as DominionSim does.
- **Default play rules per card.** Each card file also gives a simple default for its choices: what to trash, what to discard, which mode to pick. Buy-rule bots need it, and it is the starting policy for search. Provincial did the same.
- **Search:** information-set MCTS with determinization. Each simulation draws one shuffle of the hidden cards from the known composition, then uses `Clone()` and the turn re-play described above.
- **Network:** a value head (one win probability per player) and a policy head over the legal options, after Temple Gates.
  - Each card is a learned vector of about ten numbers, so one network covers every kingdom.
  - Inference runs in pure Go: small matrices and no cgo.
  - Training runs outside the binary.

### Self-play and training

- **Games.** `scry sim selfplay -games N -out DIR` plays bot games on random kingdoms. For every decision it writes the view's features, the legal options, the answer and the search's visit counts, and for every game the result. Files are compact and append-only.
- **Training.** A one-off `uv run` job with PyTorch reads them, trains, and exports the weights in a plain file format that Go reads. Package versions stay pinned and at least 3 days old.
- **Head start.** Before any self-play, a network trained to copy the named seat's choices in our saved logs gives a starting policy. Temple Gates started from no knowledge; we don't have to.
- **The loop:**
  1. self-play with search guided by the current network;
  2. train on those games, using the final result and TD targets;
  3. keep the new network only if it beats the old one in a fixed match;
  4. repeat.
- **Measuring strength:**
  - against Big Money, BM+X and the buy-rule bots, and against earlier networks, in matches of fixed size;
  - for the evaluation bar, calibration on real games: Brier score and log loss against what actually happened.

### Speed

Speed targets are set after M0, from real measurements. What we know:

- A toy Go engine played a whole Big Smithy against Big Money game in 3.1 µs on one M1 thread. That is a floor, not a forecast.
- The hot path avoids maps and allocations: cards are small integers, zones reuse their slices, and card hooks are looked up in a table by `CardID`.
- A state copy, a one-turn re-play and a random game are benchmarked on every milestone.

## Plan

The milestones follow mechanisms, since each expansion brings its own. Effort is an estimate, flagged as such, and is re-estimated after M0 from how many cards per day actually pass `scry sim check`.

| Milestone | What | Done when | Effort (estimate) |
|---|---|---|---|
| M0 | Core: state, zones, turn structure, decisions, triggers, `Move`, costs, log output, clone and re-play, `scry sim check`, fuzzing, random and Big Money bots. The base set and Intrigue. | Random games run without broken invariants. Engine logs read back through Cartographer. Clone and re-play costs measured. | 3-5 weeks |
| M1 | Seaside (Durations), Prosperity (Colony, Platinum, VP tokens), Hinterlands (on-gain effects), Cornucopia & Guilds (Rewards, Coffers, overpay), Alchemy (Potions) | Every saved game that only uses these sets passes `scry sim check`. | estimated after M0 |
| M2 | Dark Ages (Ruins, Shelters, Knights, Spoils), Adventures (Reserve, Travellers, tokens, Events), Empires (debt, Landmarks, split piles, Castles) | as above | estimated after M0 |
| M3 | Nocturne (Night, Boons, Hexes, States, Heirlooms, Spirits), Renaissance (Villagers, Projects, Artifacts), Menagerie (Ways, Horses, Exile) | as above | estimated after M0 |
| M4 | Allies (Favors, Liaisons, rotating piles), Plunder (Loot, Traits), Rising Sun (Prophecies, Omens, Shadow cards), Promos, then Possession | All 771 current cards, then the 48 first-edition ones. 90% or more of the saved games pass `scry sim check`; the rest are listed with reasons. | estimated after M0 |
| M5 | Buy-rule bots, information-set MCTS, the self-play file format, the imitation head start | MCTS beats Big Money and BM+X in fixed matches. | 2-3 weeks |
| M6 | Network training loop and Go inference | The network beats the M5 bots. Its value is calibrated on real games. | open |

**Writing 819 cards.** Card files can be written in parallel by coding-agent sessions on the user's subscriptions, one set per session. The steps for each card:

1. read the card's text locally (dominiontabs or the rulebook) and the rulings;
2. write the card file and its default play rule;
3. run `scry sim check` on the saved games that use the card.

A card counts as done when the games that use it pass the check, not when the session says it's done. My rough guess for all cards is two to three months part-time including debugging. That is an estimate to replace with the M0 measurement.

**Rules source.** The second-edition rulebooks and the published rulings decide; where they're silent, dominion.games' behaviour as seen in our logs does. The Dominion Strategy wiki's rulings sections are a good human reference, but they can't be read by our tools.

## Open questions

1. **Who plays.** This draft assumes "play each other" means bots playing bots, for training and analysis. Is a way for a person to play against the bots locally wanted too, and when?
2. **Player count.** Two players first, then 3-4 for self-play, or is two enough?
3. **Order of M5 and M6.** Ship search with a hand-made evaluation first and add the network after, or go straight for the network as Temple Gates did?
4. **Public repo.** pydominion and DominionSim implement every card in public. Keeping the engine in this public repo seems fine, but should it stay private until it works?
5. **pydominion's license.** If its author licensed it (MIT, say), its rules and tests could be ported directly instead of only read. Draft an issue asking?
