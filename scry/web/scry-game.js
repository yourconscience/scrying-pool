// "Scry game" bookmark: the analysis page builds it from capture.js plus this file,
// with SCRY_ORIGIN set to the app's address. On dominion.games it loads one
// finished game by ID (from "#scry=ID" in the address, or it asks), plays it out
// against a bot like capture.js does, and sends it to the app: to the tab that
// opened this one, and straight to the app's /api/games. If neither works, the
// game is copied to the clipboard for the analysis page's paste box.
(async () => {
  const ORIGIN = "SCRY_ORIGIN";
  const C = window.scryCapture;
  const say = (msg, ms) => C.say(msg, ms);
  if (C.running) return say("Scrying Pool: a capture is already running in this tab.");
  const m = /scry=(\d+)/.exec(location.hash);
  const id = (m ? m[1] : prompt("Scrying Pool: ID of the finished game to load") || "").trim();
  if (!id) return;
  if (!/^\d+$/.test(id)) return say("Scrying Pool: a game ID is a number, e.g. 184518886.");
  if (!C.loggedIn()) return say("Scrying Pool: log in to dominion.games first.");
  say(`Scrying Pool: loading game ${id}. This tab plays it out against a bot; leave it be for a minute.`, 0);
  let rec;
  try {
    rec = await C.captureId(id);
  } catch (e) {
    await C.recover();
    return say(`Scrying Pool: couldn't load game ${id}: ${e.message || e}`);
  }
  rec.row = "";
  const body = JSON.stringify(rec);
  if (window.opener) window.opener.postMessage({ scryGame: rec }, ORIGIN);
  // The browser may first ask whether dominion.games may connect to apps on this
  // device; the request waits for that answer.
  say("Scrying Pool: sending the game to the app. If the browser asks whether dominion.games may connect to apps on this device, allow it.", 0);
  const ac = new AbortController();
  const timer = setTimeout(() => ac.abort(), 60000);
  try {
    const r = await fetch(ORIGIN + "/api/games", { method: "POST", headers: { "Content-Type": "text/plain" }, body, signal: ac.signal });
    if (!r.ok) throw new Error((await r.json()).error || "HTTP " + r.status);
    return say(`Scrying Pool: game ${id} is saved. See it at ${ORIGIN}/analyze.html#${id}`, 10000);
  } catch (e) {
    window.scryGame = body;
    const why = ac.signal.aborted ? "no answer in a minute" : e.message || e;
    try {
      await navigator.clipboard.writeText(body);
      say(`Scrying Pool: couldn't reach the app (${why}). The game is copied: paste it on the analysis page.`, 15000);
    } catch (e2) {
      say(`Scrying Pool: couldn't reach the app (${why}) or the clipboard. Is scry web running?`, 15000);
    }
  } finally {
    clearTimeout(timer);
  }
})();
