// "Recent games" bookmark: the Recent games page builds it from capture.js plus
// this file, with SCRY_ORIGIN set to the page's address (the local app, or the
// live site). On dominion.games it opens that Recent games tab (or finds it open;
// the live page's Update button opens this tab with #scry-recent, and the
// bookmark answers that page), reads your profile's latest rated two-player
// games and sends them there. The local app then asks for the games it hasn't
// saved, and the bookmark loads them one at a time, like capture.js does.
// Clicked again while games are loading, it only reconnects to the tab.
(async () => {
  const ORIGIN = "SCRY_ORIGIN";
  const C = window.scryCapture;
  if (!C.loggedIn()) return C.say("Scrying Pool: log in to dominion.games first.");
  const fromPage = location.hash.includes("scry-recent") && window.opener && !window.opener.closed;
  const win = fromPage ? window.opener : window.open(ORIGIN + "/recent.html", "scry-recent");
  if (!win) return C.say("Scrying Pool: the browser blocked the new tab. Allow pop-ups for dominion.games and click the bookmark again.", 15000);
  C.attach(win, ORIGIN);
  // the tab answers once its page has loaded
  let hello = null;
  for (let i = 0; i < 30 && !hello; i++) hello = await C.request("hello", {}, 1000);
  if (!hello) return C.say(`Scrying Pool: the Recent games tab didn't answer. Is scry web running at ${ORIGIN}?`, 15000);
  if (C.running) return C.say("Scrying Pool: games are loading in this tab; the Recent games tab gets the rest.");

  C.say("Scrying Pool: reading your profile…", 0);
  let me, games;
  try {
    if (hello.account) {
      me = hello.account;
      games = await C.profile(me);
    } else {
      ({ me, games } = await C.ownProfile());
    }
  } catch (e) {
    return C.say(`Scrying Pool: couldn't read your profile: ${e.message || e}`, 15000);
  }
  const list = games.filter((r) => r.rated && (r.players || []).length === 2).map((r) => {
    const id = C.noteGame(r, me);
    return { id, profile: C.meta[id] };
  });
  const answer = await C.request("games", { me, games: list });
  if (!answer) return C.say("Scrying Pool: the Recent games tab didn't answer.", 15000);
  if (answer.error) return C.say(`Scrying Pool: ${answer.error}`, 15000);
  const report = (e) => C.tell("progress", { id: e.id, status: e.status, error: e.error || "", left: C.queue.length });
  // games captured here before keep their result: send those again instead of loading them
  await C.load();
  const load = [];
  for (const id of answer.need) {
    const r = C.results[id];
    if (r && r.status === "ok") {
      r.delivered = false;
      await C.deliver(r);
      report(r);
    } else if (r && (r.status === "short" || r.status === "server")) report(r);
    else load.push(id);
  }
  if (!load.length) return C.say(`Scrying Pool: your ${list.length} latest rated games are in the Recent games tab.`);
  C.onResult = report;
  await C.run(load, { alone: true });
  C.say(`Scrying Pool: loading ${load.length} games against a bot, about 20 seconds each. Leave this tab be; the Recent games tab shows them as they arrive.`, 15000);
})();
