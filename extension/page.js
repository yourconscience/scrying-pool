// Scrying Pool extension, page side. Runs in the dominion.games page itself (the MAIN
// world) because what it reads lives in the client's own objects. When the server says a
// game finished (tableService.gameResult, set on GAME_FINISHED), it reads the game's log,
// the score screen, the kingdom and each player's final score, in the shape the capture
// script exports for `scry import` (scry/web/capture.js), and hands it to relay.js. It
// only reads the page.
//
// The log comes from the client's log model (the rendered line of every entry), which
// outlives the log panel; a copy of the panel taken every tick while the game runs is the
// fallback.
//
// Left alone: games while scry is replaying games in the tab (window.scryCapture is
// running), and games with the bot that scry and Load Old Game put in a seat.
(() => {
  if (window.scrySaveLogs) return;
  window.scrySaveLogs = true;

  const resultRe = /You (Resigned|Lose|Win|Won|Tie)/;
  const bot = "Lord Rattington";
  const done = new Set();
  let game = null; // the game the client is showing, from its log updates
  let panelLog = []; // the log panel's lines, copied every tick while it shows
  let pending = null; // a finished game waiting for its score screen: {result, since}
  let started = false;

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

  const modelLog = () => {
    const els = [];
    for (const e of (game && game.logModel && game.logModel.entries) || []) for (const el of e.renderedLines || []) if (el && el.getAttribute) els.push(el);
    return lines(els);
  };

  const kingdom = () => {
    const groups = (game && game.kingdomViewer && game.kingdomViewer.cardGroups) || [];
    const out = {};
    for (const g of groups) {
      const name = typeof g.name === "string" ? g.name : (g.name && (g.name.name || g.name.key)) || String(g.name);
      if (g.cards && g.cards.length) out[name] = g.cards.map((c) => c.name);
    }
    return out;
  };

  // The players in turn order, from the first turn lines.
  const seats = (log) => {
    const players = [];
    for (const l of log) {
      const t = /^\d+T\tTurn \d+ - (.+?)(?: \[[^\]]+\])?$/.exec(l);
      if (t && !players.includes(t[1])) players.push(t[1]);
      if (players.length === 2) break;
    }
    return players;
  };

  const send = (r, me) => {
    const id = String(r.gameId);
    const forThis = (log) => log.length && (/Game #(\d+)/.exec(log[0]) || [])[1] === id;
    const fromModel = modelLog();
    const log = forThis(fromModel) && fromModel.length >= panelLog.length ? fromModel : forThis(panelLog) ? panelLog : fromModel;
    if (!forThis(log)) return;
    const players = seats(log);
    if (players.length !== 2 || players.includes(bot)) return;
    // final scores by name: mine by my player id, the other seat's by elimination
    const scores = {};
    for (const p of r.playerResults || []) {
      const name = p.playerId === me.id ? me.name : players.find((n) => n !== me.name);
      if (name && p.score) scores[name] = p.score.totalPoints;
    }
    // a live log's header names each player's rating level: "0\tjohnHatrick|: 54.13"
    const levels = {};
    for (const l of log.slice(0, 6)) {
      const m = /^0\t(.+)\|: (\d+(?:\.\d+)?)$/.exec(l);
      if (m && players.includes(m[1])) levels[m[1]] = parseFloat(m[2]);
    }
    const text = document.body.innerText, k = text.search(resultRe), end = k >= 0 ? text.indexOf("Players (", k) : -1;
    const now = new Date();
    const day = `${now.getUTCFullYear()}-${now.toLocaleString("en", { month: "short", timeZone: "UTC" })}-${String(now.getUTCDate()).padStart(2, "0")}`;
    const record = {
      id,
      row: `${day} | ${players.map((n) => n in scores ? `${n}: ${scores[n]}` : n).join(", ")}`,
      original: players,
      log: "# scrying-pool log v1\n" + log.join("\n") + "\n",
      score: k < 0 ? "" : end > k ? text.slice(k, end) : text.slice(k),
      kingdom: kingdom(),
      profile: { time: now.toISOString(), rated: /\brated\b/.test(log[0]) && !/\bunrated\b/.test(log[0]), players, scores,
        ...(Object.keys(levels).length === 2 ? { levels } : {}) },
    };
    window.postMessage({ scrySaveLogs: "game", record }, location.origin);
  };

  const check = () => {
    hook();
    let ts, me;
    try {
      const inj = window.angular && angular.element(document.body).injector();
      ts = inj && inj.get("tableService");
      me = inj && inj.get("meta").model.me;
    } catch (e) { return; }
    if (!ts || !me) return;
    const r = ts.gameResult;
    if (!started) { // a result already shown when the page loaded is not this session's game
      started = true;
      if (r) done.add(String(r.gameId));
    }
    const panel = lines([...document.querySelectorAll(".log-scroll-container .log-line")]);
    if (panel.length) panelLog = panel;
    if (window.scryCapture && window.scryCapture.running) return;
    if (r && !done.has(String(r.gameId))) {
      done.add(String(r.gameId));
      pending = { result: r, since: Date.now() };
    }
    // wait up to 10 s for the score screen, whose text goes with the game
    if (pending && (resultRe.test(document.body.innerText) || Date.now() - pending.since > 10000)) {
      const p = pending;
      pending = null;
      send(p.result, me);
    }
  };

  setInterval(check, 2000);
})();
