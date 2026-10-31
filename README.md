<div align="center">

<h1>Scrying Pool</h1>

<p><b>Replay your finished Dominion Online games and see what wins them.</b></p>

<p><a href="https://yourconscience.github.io/scrying-pool/"><b>Open the live profile page</b></a></p>

</div>

<table>
<tr>
<td width="50%" valign="top">
<img src="docs/icons/chart-line-up.svg" width="36" alt=""><br>
<b>See what wins your games</b><br>
Your win rate with a village, +Buy, trashing or an attack in the kingdom, by opening and by card. Clear differences show in green or red.
</td>
<td width="50%" valign="top">
<img src="docs/icons/users-three.svg" width="36" alt=""><br>
<b>Compare two players</b><br>
Put another player's profile next to yours and see where you differ.
</td>
</tr>
<tr>
<td width="50%" valign="top">
<img src="docs/icons/clock-counter-clockwise.svg" width="36" alt=""><br>
<b>Replay any game</b><br>
Step through a game one turn at a time. See the score, the money and where every card is.
</td>
<td width="50%" valign="top">
<img src="docs/icons/chats-circle.svg" width="36" alt=""><br>
<b>Notes from a match chat</b><br>
Paste the chat of a match. Get one short section per game with its kingdom and result. [Try it](https://yourconscience.github.io/scrying-pool/notes.html).
</td>
</tr>
</table>

## Get your own profile

Profiles are built from games saved on your computer, so this part runs locally. You need a dominion.games account and [Go](https://go.dev/dl/) 1.26 or newer.

1. Install and start the app, then open http://127.0.0.1:8765:

   ```bash
   go install github.com/yourconscience/scrying-pool/scry@latest
   ```

   ```bash
   scry web
   ```

2. Save your finished games: one with the **Scry game** bookmark on the Analyze page, your 25 latest with the **Recent games** bookmark, many with the capture script. [Loading games](docs/capture.md) walks you through them.
3. Your profile appears on the app's Profiles page. To put it on the live page, follow [site-data/README.md](site-data/README.md).
4. Before a match, `scry scout PLAYER` loads your opponent's latest games and sets their openings, cards and styles beside yours ([Scouting an opponent](docs/capture.md#scouting-an-opponent)).

## Good to know

- **Private.** Your games stay on your computer. Nothing is sent anywhere unless you publish your profile or turn on DomBot lookups.
- **Finished games only.** The dominion.games Terms of Service ban tools that help during a game.
- **Card art** belongs to Rio Grande Games. The app shows images from [KingdomCreator](https://github.com/blakevanlan/KingdomCreator); none are stored here.
- **Not affiliated** with Shuffle iT, Rio Grande Games, DomBot or the Dominion League. How it works is in [docs/internals.md](docs/internals.md).

<sub>Icons: [Phosphor](https://phosphoricons.com) (MIT).</sub>
