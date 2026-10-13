<div align="center">

<h1>Scrying Pool</h1>

<p><b>Replay your finished Dominion Online games and see what wins them.</b></p>

<p>
<a href="#get-started">Get started</a> |
<a href="docs/capture.md">Loading games</a> |
<a href="docs/match-notes.md">Match notes</a> |
<a href="docs/internals.md">How it works</a>
</p>

</div>

Scrying Pool is a small app that runs on your computer. Type the number of a game you played on [dominion.games](https://dominion.games), and it shows you that game again, turn by turn.

<table>
<tr>
<td width="50%" valign="top">
<img src="docs/icons/clock-counter-clockwise.svg" width="36" alt=""><br>
<b>Replay any game</b><br>
Step through a game one turn at a time. See the score, the money and where every card is.
</td>
<td width="50%" valign="top">
<img src="docs/icons/chart-line-up.svg" width="36" alt=""><br>
<b>See what wins your games</b><br>
Your win rate going first, with a village, with +Buy and more. Clear differences show in green or red.
</td>
</tr>
<tr>
<td width="50%" valign="top">
<img src="docs/icons/users-three.svg" width="36" alt=""><br>
<b>Compare two players</b><br>
Put another player's profile next to yours and see where you differ.
</td>
<td width="50%" valign="top">
<img src="docs/icons/chats-circle.svg" width="36" alt=""><br>
<b>Notes from a match chat</b><br>
Paste the chat of a match. Get one short section per game with its kingdom and result.
</td>
</tr>
</table>

## Get started

You need a dominion.games account and [Go](https://go.dev/dl/) 1.26 or newer.

1. Install the app:

   ```bash
   go install github.com/yourconscience/scrying-pool@latest
   ```

2. Start it:

   ```bash
   scry web
   ```

3. Open http://127.0.0.1:8765 in your browser.

The first time, the app needs one bookmark from your dominion.games tab. [Loading games](docs/capture.md) shows how.

## Good to know

- **Private.** Your games stay on your computer. Nothing is sent anywhere unless you publish a page or turn on DomBot lookups.
- **Finished games only.** The dominion.games Terms of Service ban tools that help during a game.
- **Card art** belongs to Rio Grande Games. The app shows images from [KingdomCreator](https://github.com/blakevanlan/KingdomCreator); none are stored here.
- **Work in progress.** The plan is in [SPEC.md](SPEC.md). Not affiliated with Shuffle iT, Rio Grande Games, DomBot or the Dominion League.

<sub>Icons: [Phosphor](https://phosphoricons.com) (MIT).</sub>
