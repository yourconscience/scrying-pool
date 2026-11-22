// Scrying Pool extension, page side. Runs in the dominion.games page itself (the MAIN
// world) because the kingdom lives in the client's game object. When a game ends, it
// reads the log panel, the score screen and the kingdom, in the shape the capture script
// exports for `scry import` (scry/web/capture.js), and hands it to relay.js. It only
// reads the page.
//
// Left alone: games while scry is replaying games in the tab (window.scryCapture is
// running), and games with the bot that scry and Load Old Game put in a seat.
(() => {
  if (window.scrySaveLogs) return;
  window.scrySaveLogs = true;

  const resultRe = /You (Resigned|Lose|Win|Won|Tie)/;
  const bot = "Lord Rattington";
  const seen = new Set();
  let game = null;

  // The client's log updates carry the game object; keep the latest one.
  const hook = () => {
    const P = window.LogEntriesChange && window.LogEntriesChange.prototype;
    if (!P || P.scrySaveLogsHooked) return;
    const execute = P.execute;
    P.execute = function (g) { game = g; return execute.call(this, g); };
    P.scrySaveLogsHooked = true;
  };

  // Same line format as capture.js and the Copy log bookmark.
  const lines = (rows) => rows.map((r) => {
    const m = /padding-left:\s*([\d.]+)%/.exec(r.getAttribute("style") || "");
    const level = m ? Math.round(parseFloat(m[1]) / 4.54545) : 0;
    const kind = r.classList.contains("new-turn-line") ? "T" : r.classList.contains("shuffles-line") ? "S" : "";
    return level + kind + "\t" + [...r.children].map((c) => c.textContent.trim()).filter(Boolean).join("|");
  });

  const kingdom = () => {
    const groups = (game && game.kingdomViewer && game.kingdomViewer.cardGroups) || [];
    const out = {};
    for (const g of groups) {
      const name = typeof g.name === "string" ? g.name : (g.name && (g.name.name || g.name.key)) || String(g.name);
      if (g.cards && g.cards.length) out[name] = g.cards.map((c) => c.name);
    }
    return out;
  };

  // The score screen's own "You Win" (or Lose, Tie, Resigned), not the same words in the log.
  const ended = () => {
    const found = document.evaluate("//*[not(*)][starts-with(normalize-space(.), 'You ')]", document.body, null, XPathResult.ORDERED_NODE_SNAPSHOT_TYPE, null);
    for (let i = 0; i < found.snapshotLength; i++) {
      const e = found.snapshotItem(i);
      if (resultRe.test(e.textContent) && !e.closest(".log-scroll-container")) return true;
    }
    return false;
  };

  const check = () => {
    hook();
    if (window.scryCapture && window.scryCapture.running) return;
    const rows = [...document.querySelectorAll(".log-scroll-container .log-line")];
    if (!rows.length) return;
    const text = document.body.innerText;
    const k = text.search(resultRe);
    if (k < 0 || !ended()) return;
    const log = lines(rows);
    const head = /Game #(\d+)/.exec(log[0] || "");
    if (!head || seen.has(head[1])) return;
    seen.add(head[1]);
    // players in turn order, from the first turn lines
    const players = [];
    for (const l of log) {
      const t = /^\d+T\tTurn \d+ - (.+?)(?: \[[^\]]+\])?$/.exec(l);
      if (t && !players.includes(t[1])) players.push(t[1]);
      if (players.length === 2) break;
    }
    if (players.length !== 2 || players.includes(bot)) return;
    const end = text.indexOf("Players (", k);
    const now = new Date();
    const day = `${now.getUTCFullYear()}-${now.toLocaleString("en", { month: "short", timeZone: "UTC" })}-${String(now.getUTCDate()).padStart(2, "0")}`;
    const rated = /\brated\b/.test(log[0]) && !/\bunrated\b/.test(log[0]);
    const record = {
      id: head[1],
      row: `${day} | ${players.join(", ")}`,
      original: players,
      log: "# scrying-pool log v1\n" + log.join("\n") + "\n",
      score: end > k ? text.slice(k, end) : text.slice(k),
      kingdom: kingdom(),
      profile: { time: now.toISOString(), rated, players },
    };
    window.postMessage({ scrySaveLogs: "game", record }, location.origin);
  };

  setInterval(check, 2000);
})();
