// Scrying Pool capture script for dominion.games. Loads FINISHED games by ID through
// My Table > Load Old Game, copies each game's log, score screen and kingdom, and
// exports them as JSON lines for `scry import`. It only clicks the client's own buttons
// (My Table, Load Old Game, Load from End, the bot seat, Ready, Resign, Yes, Ok, Leave
// Table, the Profile tab and its player search) and reads the page. Games where a player
// took fewer than minTurns turns (AFK) are dropped.
//
// A loaded game stops at the original game's last decision. When that decision is the
// bot's seat's (the game ended with a resignation), the bot answers it and plays on. The
// client's log model marks every decision, so the lines after the loaded decision are cut
// off and exported separately as "bot": they are not part of the game, but the score
// screen counts them.
//
// Built to run unattended overnight: every result is kept in the page's IndexedDB, so a
// reload loses nothing (inject the script again and call resume()); each game has a time
// limit and is tried once more after an error; a logout or a lost connection is waited
// out. Each captured game goes to a Scrying Pool tab opened from here (attach(), as the
// Recent games bookmark does), or with `server` set, is posted to scry web.
//
// Use: paste into the console (or a javascript_tool call) on dominion.games while logged in, then
//   scryCapture.run(["184518886", ...])             // full capture; progress: scryCapture.status()
//   scryCapture.run(ids, { mode: "kingdom" })       // only the kingdom, for games saved before it was read
//   scryCapture.harvest({ minLevel: 50, skip })     // rated games of strong players, from their profiles
//   scryCapture.ownProfile()                        // your name and your profile's latest games
//   scryCapture.exportNext(25)                      // then press Cmd+C in the page; `pbpaste | scry import`
(() => {
  if (window.scryCapture && window.scryCapture.running) return "a capture is running; call scryCapture.stop() first";
  const C = {
    version: 2, // the bookmarks load this script again over an older copy
    minTurns: 3,
    pauseMs: 4000,
    gameTimeoutMs: 10 * 60 * 1000, // a game that takes longer is abandoned and the page reset
    // A loaded game starts about a second after Ready. For some older games the server's
    // replay of the original decisions stalls, and it sends nothing for minutes or for good.
    startTimeoutMs: 2 * 60 * 1000,
    offlineWaitMs: 15 * 60 * 1000, // how long to wait for the client to come back after a logout or disconnect
    server: null, // e.g. "http://127.0.0.1:8765": post each captured game to scry web
    target: null, // a Scrying Pool tab, from attach(): it gets each captured game instead
    targetOrigin: null,
    replies: {}, // request number -> its resolver, for the target tab's answers
    seq: 0,
    onResult: null, // called with each game's result
    results: {}, // id -> {id, status: "ok"|"short"|"kingdom"|"server"|"error", record?, kingdom?, error?, at, exported?, delivered?}
    meta: {}, // id -> what a profile said about the game: {time, rated, players, levels, scores, row}
    queue: [], // [{id, mode: "full"|"kingdom"}]
    running: false,
    stopRequested: false,
    gen: 0, // bumped when a game is abandoned, so its leftover waits stop

    // Timers in a hidden tab wake at most about once a minute once they chain;
    // a worker's timers don't, so waits run on one.
    sleep(ms) {
      if (this.timer === undefined) {
        try {
          const src = "onmessage = (e) => setTimeout(() => postMessage(e.data), e.data.ms)";
          this.timer = new Worker(URL.createObjectURL(new Blob([src], { type: "text/javascript" })));
          this.waiting = new Map();
          this.timerSeq = 0;
          this.timer.onmessage = (e) => { const f = this.waiting.get(e.data.id); this.waiting.delete(e.data.id); if (f) f(); };
        } catch (e) {
          this.timer = null;
        }
      }
      if (!this.timer) return new Promise((r) => setTimeout(r, ms));
      return new Promise((r) => { const id = ++this.timerSeq; this.waiting.set(id, r); this.timer.postMessage({ id, ms }); });
    },
    visible: (e) => !!(e && (e.offsetParent !== null || getComputedStyle(e).position === "fixed")),
    byText(txt) {
      const all = [...document.querySelectorAll("*")].filter((e) => this.visible(e) && e.textContent.trim() === txt);
      return all.find((e) => ![...e.children].some((c) => c.textContent.trim() === txt)) || null;
    },
    byLabel(txt) {
      return [...document.querySelectorAll("[title],[aria-label]")].find((e) => (e.getAttribute("title") === txt || e.getAttribute("aria-label") === txt) && this.visible(e)) || null;
    },
    async waitFor(fn, ms = 20000, what = "the page") {
      const t0 = Date.now(), gen = this.gen;
      while (Date.now() - t0 < ms) {
        if (gen !== this.gen) throw new Error("abandoned");
        const v = fn();
        if (v) return v;
        await this.sleep(300);
      }
      throw new Error("timed out waiting for " + what);
    },
    // The lobby menu shows "My Table" at a table and "New Table" elsewhere; the login page shows neither.
    loggedIn() { return !!this.byText("Logout") || !!document.querySelector(".log-scroll-container"); },
    // A note at the top of the page; ms 0 keeps it until the next one.
    say(msg, ms = 6000) {
      if (this.note) this.note.remove();
      const note = (this.note = document.createElement("div"));
      note.textContent = msg;
      note.style.cssText = "position:fixed;z-index:99999;top:16px;left:50%;transform:translateX(-50%);max-width:90vw;" +
        "background:#1c201a;color:#e5e8de;font:14px system-ui;padding:10px 16px;border-radius:6px;" +
        "box-shadow:0 2px 12px rgba(0,0,0,.4)";
      document.body.append(note);
      if (ms) setTimeout(() => note.remove(), ms);
    },

    // ---- storage: IndexedDB "scrying-pool", so results outlive a reload

    db: null,
    async store() {
      if (!this.db) {
        this.db = await new Promise((ok, fail) => {
          const r = indexedDB.open("scrying-pool", 1);
          r.onupgradeneeded = () => { r.result.createObjectStore("games", { keyPath: "id" }); r.result.createObjectStore("state"); };
          r.onsuccess = () => ok(r.result);
          r.onerror = () => fail(r.error);
        });
      }
      return this.db;
    },
    async tx(name, mode, fn) {
      const db = await this.store();
      return new Promise((ok, fail) => {
        const t = db.transaction(name, mode), req = fn(t.objectStore(name));
        t.oncomplete = () => ok(req && req.result);
        t.onerror = () => fail(t.error);
      });
    },
    async save(entry) { this.results[entry.id] = entry; await this.tx("games", "readwrite", (s) => s.put(entry)); },
    async saveState() { await this.tx("state", "readwrite", (s) => s.put({ queue: this.queue, later: this.later || null, meta: this.meta }, "run")); },
    // Loads what earlier runs in this browser stored.
    async load() {
      for (const e of (await this.tx("games", "readonly", (s) => s.getAll())) || []) this.results[e.id] = e;
      const st = await this.tx("state", "readonly", (s) => s.get("run"));
      if (st) {
        if (!this.queue.length) this.queue = st.queue || [];
        if (!this.later && st.later) this.later = st.later; // a queue set aside by an alone run
        this.meta = Object.assign(st.meta || {}, this.meta);
      }
      return Object.keys(this.results).length;
    },

    // ---- reading the page

    lines(rows) {
      return rows.map((r) => {
        const m = /padding-left:\s*([\d.]+)%/.exec(r.getAttribute("style") || "");
        const level = m ? Math.round(parseFloat(m[1]) / 4.54545) : 0;
        const kind = r.classList.contains("new-turn-line") ? "T" : r.classList.contains("shuffles-line") ? "S" : "";
        return level + kind + "\t" + [...r.children].map((c) => c.textContent.trim()).filter(Boolean).join("|");
      });
    },
    // The log up to and including last (a log line element), and the lines after it.
    extractLog(last) {
      const rows = [...document.querySelectorAll(".log-scroll-container .log-line")];
      const k = last ? rows.indexOf(last) : -1;
      const keep = k >= 0 ? rows.slice(0, k + 1) : rows, bot = k >= 0 ? rows.slice(k + 1) : [];
      return { log: "# scrying-pool log v1\n" + this.lines(keep).join("\n") + "\n", bot: bot.length ? this.lines(bot).join("\n") + "\n" : "" };
    },
    // Keeps a reference to the game the client is showing: its log model holds the
    // decisions (index, decisionIndex, playerIndex) between the log entries.
    watchLog() {
      const proto = window.LogEntriesChange && window.LogEntriesChange.prototype;
      if (!proto || proto.scryWatchedGame) return;
      const execute = proto.execute;
      proto.execute = function (game) { if (window.scryCapture) window.scryCapture.game = game; return execute.call(this, game); };
      proto.scryWatchedGame = true;
    },
    entries() { return (this.game && this.game.logModel && this.game.logModel.entries) || []; },
    // The last log line of the original game: the one before decision n's marker. Null
    // when decision n was never made (the game ended on your seat's decision).
    lastOriginalLine(n) {
      const es = this.entries();
      const at = es.findIndex((e) => e.decisionIndex === n);
      for (let i = at - 1; i >= 0; i--) if (es[i].renderedLines && es[i].renderedLines.length) return es[i].renderedLines[es[i].renderedLines.length - 1];
      return null;
    },
    // The game's kingdom as the client's kingdom viewer groups it, e.g.
    // {"Cards in Supply": ["Craftsman", ...], "Events, Landmarks, ...": ["Peril"], "Loot": [...]}.
    kingdom() {
      const groups = (this.game && this.game.kingdomViewer && this.game.kingdomViewer.cardGroups) || [];
      const out = {};
      for (const g of groups) {
        const name = typeof g.name === "string" ? g.name : (g.name && (g.name.name || g.name.key)) || String(g.name);
        if (g.cards && g.cards.length) out[name] = g.cards.map((c) => c.name);
      }
      return out;
    },
    minTurnsIn(log) {
      const last = {};
      for (const l of log.split("\n")) {
        const m = l.match(/^\d+T\tTurn (\d+) - (.+?)(?: \[[^\]]+\])?$/);
        if (m) last[m[2]] = +m[1];
      }
      // players by initial in "starts with" lines (Heirlooms and Shelters add more lines per player)
      const players = new Set((log.match(/^\d+\t\S{1,2}\|starts with\|/gm) || []).map((x) => x.split("\t")[1].split("|")[0])).size;
      const turns = Object.values(last);
      return turns.length < Math.max(2, players) ? 0 : Math.min(...turns);
    },
    gameIdInput() {
      const el = document.querySelector('input[ng-model="$ctrl.replayId"]');
      return this.visible(el) ? el : null;
    },
    decisionInput() {
      const el = document.querySelector('input[ng-model="$ctrl.replayDecision"]');
      return this.visible(el) ? el : null;
    },
    setInput(el, value) {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set;
      setter.call(el, value);
      for (const t of ["input", "change", "blur"]) el.dispatchEvent(new Event(t, { bubbles: true }));
    },
    tab(name) { return [...document.querySelectorAll("li.tab")].find((e) => e.textContent.trim() === name && this.visible(e)) || null; },
    async toMyTable() {
      (await this.waitFor(() => this.byText("My Table") || this.byText("New Table"), 10000, "My Table")).click();
      await this.sleep(1000);
    },
    // Seats a bot in the empty seat left by the opponent ("Empty Slot / Original Player: X").
    async seatBot() {
      if (!this.byText("Empty Slot")) return false;
      const bot = [...document.querySelectorAll(".table-add-bot-icon")].find((e) => this.visible(e));
      if (!bot) throw new Error("can't find the add-bot button next to the empty seat");
      bot.click();
      await this.waitFor(() => !this.byText("Empty Slot"), 8000, "the bot to take the seat");
      return true;
    },
    // Loads a finished game at its last decision and returns that decision's index. The
    // field is reset first: it keeps the last answer, and a stale -1 would fail the next game.
    async loadOld(id) {
      await this.toMyTable();
      let input = this.gameIdInput();
      if (!input) {
        (await this.waitFor(() => this.byText("Load Old Game"), 8000, "Load Old Game")).click();
        input = await this.waitFor(() => this.gameIdInput(), 8000, "the Game Id field");
      }
      const d0 = this.decisionInput();
      if (d0) this.setInput(d0, "0");
      this.setInput(input, id);
      await this.sleep(500);
      this.byText("Load from End").click();
      let minus = 0;
      const v = await this.waitFor(() => {
        const d = this.decisionInput(), v = d && d.value;
        // -1 that stays: the server answered "Internal error." Some finished games never load.
        if (v === "-1" && ++minus >= 8) { const e = new Error("the server can't load this game (decision index -1)"); e.server = true; throw e; }
        return v && /^\d+$/.test(v) && v !== "0" ? v : null;
      }, 15000, "the decision index");
      await this.sleep(800);
      return +v;
    },
    async startGame() {
      await this.seatBot();
      this.watchLog();
      this.game = null;
      (await this.waitFor(() => this.byText("Ready"), 8000, "Ready")).click();
    },
    // Resigns if the game is still on, reads the score screen and leaves the table.
    // A replayed game can be showing its own Yes/No question, so only a Yes that
    // appears after clicking Resign confirms it; Resign is clicked again until it takes.
    async finish() {
      const resultRe = /You (Resigned|Lose|Win|Won|Tie)/;
      const yeses = () => [...document.querySelectorAll("*")].filter((e) => this.visible(e) && e.textContent.trim() === "Yes" && ![...e.children].some((c) => c.textContent.trim() === "Yes"));
      let before = null, clickedAt = 0;
      await this.waitFor(() => {
        if (resultRe.test(document.body.innerText)) return true;
        const ok = this.byText("Ok"); // the resignation and the game's end each show a dialog with Ok
        if (ok) { ok.click(); return false; }
        if (before) {
          const yes = yeses().find((e) => !before.includes(e));
          if (yes) { yes.click(); before = null; return false; }
          if (Date.now() - clickedAt < 3000) return false;
          before = null;
        }
        const resign = this.byLabel("Resign") || this.byText("Resign");
        if (resign && Date.now() - clickedAt > 3000) { before = yeses(); resign.click(); clickedAt = Date.now(); }
        return false;
      }, 40000, "the score screen");
      const t = document.body.innerText, k = t.search(resultRe);
      const score = t.slice(k, t.indexOf("Players (", k));
      const kingdom = this.kingdom();
      await this.leave();
      await this.sleep(1200);
      return { score, kingdom };
    },

    async captureId(id) {
      const resultRe = /You (Resigned|Lose|Win|Won|Tie)/;
      const decision = await this.loadOld(id);
      const original = (document.body.innerText.match(/Original Player: [^\n]+/g) || []).map((s) => s.slice(17));
      await this.startGame();
      const lines = () => document.querySelectorAll(".log-scroll-container .log-line").length;
      try {
        await this.waitFor(() => lines() > 0, this.startTimeoutMs, "the game to start");
      } catch (e) {
        if (e.message === "abandoned") throw e;
        const err = new Error(`the server didn't start the game in ${Math.round(this.startTimeoutMs / 60000)} min (its replay stalls)`);
        err.server = true;
        throw err;
      }
      // done when the log has reached the loaded decision and stops growing: the bot has
      // played up to your seat's next decision, or the loaded decision is yours, or the bot
      // ended the game. A slow replay streams in for minutes; 90 s without a new line ends it.
      let n = 0, same = 0, grew = Date.now();
      try {
        await this.waitFor(() => {
          const m = lines();
          if (!m) throw new Error("the game closed while its log loaded (the connection dropped)");
          if (m === n) same++; else { same = 0; n = m; grew = Date.now(); }
          if (Date.now() - grew > 90000) throw new Error("timed out waiting for the log to finish loading");
          const reached = !this.game || this.entries().some((x) => x.decisionIndex >= decision - 1);
          return (reached && same >= 8) || (resultRe.test(document.body.innerText) && same >= 2);
        }, this.gameTimeoutMs, "the log to finish loading");
      } catch (e) {
        // the bot can get stuck ("Waiting for Lord Rattington"); past its first move the
        // game's own log is complete anyway
        if (e.message === "abandoned" || !lines() || !this.entries().some((x) => x.decisionIndex === decision)) throw e;
      }
      const { log, bot } = this.extractLog(this.lastOriginalLine(decision));
      const markers = !!(this.game && this.game.logModel);
      const { score, kingdom } = await this.finish();
      return { id, original, decision, markers, log, bot, score, kingdom, profile: this.meta[id] };
    },
    // Only the kingdom: starts the game, reads the kingdom viewer, resigns.
    async kingdomOnly(id) {
      await this.loadOld(id);
      await this.startGame();
      await this.waitFor(() => { const k = this.kingdom(); return k["Cards in Supply"] && k["Cards in Supply"].length; }, 60000, "the kingdom");
      await this.sleep(500);
      return (await this.finish()).kingdom;
    },
    // Leaves the table; its button may show an unrendered label after a game the bot ended.
    async leave() {
      (await this.waitFor(() => this.byText("Leave Table") || document.querySelector('[ng-click="$ctrl.leave()"]'), 8000, "Leave Table")).click();
    },
    // The client stops rendering its templates after some game endings; only a reload fixes it.
    broken() { return document.body.innerText.includes("{{$ctrl."); },
    // Puts the page back on the lobby after an error: finishes an open game and leaves the table.
    async recover() {
      this.gen++;
      try {
        if (document.querySelector(".log-scroll-container") && !/You (Resigned|Lose|Win|Won|Tie)/.test(document.body.innerText)) {
          (this.byLabel("Resign") || this.byText("Resign"))?.click();
          (await this.waitFor(() => this.byText("Yes"), 5000))?.click();
          (await this.waitFor(() => this.byText("Ok"), 5000))?.click();
        }
      } catch (e) { /* not in a game */ }
      try { await this.leave(); } catch (e) { /* no table */ }
      await this.sleep(1500);
    },
    // Rejects when p takes longer than ms; the abandoned work stops at its next wait.
    limit(p, ms) {
      let timer;
      return Promise.race([p, new Promise((_, fail) => { timer = setTimeout(() => fail(new Error(`gave up after ${Math.round(ms / 60000)} min`)), ms); })]).finally(() => clearTimeout(timer));
    },
    // Uses a Scrying Pool tab this one opened: games go to it by postMessage, which
    // the browser's rules for pages reaching apps on your device don't hold up.
    attach(win, origin) {
      this.target = win;
      this.targetOrigin = new URL(origin).origin; // the page's address may have a path (GitHub Pages)
      if (this.listening) return;
      this.listening = true;
      window.addEventListener("message", (e) => {
        const d = e.data, f = d && this.replies[d.scryReply];
        if (e.origin !== this.targetOrigin || !f) return;
        delete this.replies[d.scryReply];
        f(d);
      });
    },
    // Sends the target tab a message and returns its answer, or null after ms.
    request(type, data, ms = 60000) {
      if (!this.target || this.target.closed) return Promise.resolve(null);
      const n = ++this.seq;
      return new Promise((resolve) => {
        const timer = setTimeout(() => { delete this.replies[n]; resolve(null); }, ms);
        this.replies[n] = (d) => { clearTimeout(timer); resolve(d); };
        this.target.postMessage({ scry: type, seq: n, data }, this.targetOrigin);
      });
    },
    tell(type, data) {
      if (this.target && !this.target.closed) this.target.postMessage({ scry: type, data }, this.targetOrigin);
    },
    async deliver(entry) {
      if (entry.status !== "ok" || entry.delivered) return;
      if (this.target && !this.target.closed) {
        const r = await this.request("game", entry.record);
        if (r && r.ok) { entry.delivered = true; await this.save(entry); } else this.lastDeliveryError = r ? r.error : "the Scrying Pool tab didn't answer";
        return;
      }
      if (!this.server) return;
      const ac = new AbortController(), timer = setTimeout(() => ac.abort(), 60000);
      try {
        const r = await fetch(this.server + "/api/games", { method: "POST", headers: { "Content-Type": "text/plain" }, body: JSON.stringify(entry.record), signal: ac.signal });
        if (r.ok) { entry.delivered = true; await this.save(entry); } else this.lastDeliveryError = "HTTP " + r.status;
      } catch (e) {
        this.lastDeliveryError = String(e.message || e);
      } finally {
        clearTimeout(timer);
      }
    },

    // ---- runs

    // Queues games and starts the loop, or adds them to a running one. Games that
    // already have a final result are skipped unless retry is set; "error" results
    // are always tried again. With alone, a queue left from an earlier run waits
    // until these games are done, and is then kept for resume().
    async run(ids, { mode = "full", retry = false, alone = false } = {}) {
      if (!this.running) await this.load();
      if (alone && !this.running) { this.later = [...(this.later || []), ...this.queue]; this.queue = []; }
      const queued = new Set(this.queue.map((q) => q.id));
      const done = (id) => { const r = this.results[id]; return r && r.status !== "error" && (mode === "full" ? r.status !== "kingdom" : true); };
      let added = 0;
      for (const id of ids.map(String)) if (!queued.has(id) && (retry || !done(id))) { this.queue.push({ id, mode }); added++; }
      await this.saveState();
      return this.running ? `queued ${added} more; ${this.queue.length} in the queue` : this.resume();
    },
    // Continues the stored queue, e.g. after a reload and a fresh injection.
    async resume() {
      if (this.running) return "already running";
      if (!this.queue.length) await this.load();
      this.running = true; this.stopRequested = false; this.lastError = null; this.startedAt = Date.now();
      (async () => {
        while (this.queue.length && !this.stopRequested) {
          if (this.broken()) { this.lastError = "the page stopped rendering; reload it, inject the script and call resume()"; break; }
          if (!this.loggedIn()) {
            const t0 = Date.now();
            while (!this.loggedIn() && Date.now() - t0 < this.offlineWaitMs) await this.sleep(10000);
            if (!this.loggedIn()) { this.lastError = "logged out or disconnected for " + Math.round(this.offlineWaitMs / 60000) + " min"; break; }
          }
          const { id, mode } = this.queue[0];
          this.current = id;
          const prev = this.results[id] || {};
          const entry = { id, at: Date.now(), tries: (prev.tries || 0) + 1 };
          try {
            if (mode === "kingdom") {
              entry.kingdom = await this.limit(this.kingdomOnly(id), this.gameTimeoutMs);
              entry.status = "kingdom";
            } else {
              const rec = await this.limit(this.captureId(id), this.gameTimeoutMs);
              rec.row = rec.row || (this.meta[id] && this.meta[id].row) || "";
              const turns = this.minTurnsIn(rec.log);
              Object.assign(entry, turns < this.minTurns ? { status: "short", turns } : { status: "ok", record: rec });
            }
          } catch (e) {
            Object.assign(entry, { status: e.server ? "server" : "error", error: String(e.message || e) });
            await this.recover();
            if (!e.server && entry.tries < 2) this.queue.push({ id, mode }); // once more, after the rest
          }
          await this.save(entry);
          this.queue.shift();
          await this.saveState();
          await this.deliver(entry);
          if (this.onResult) this.onResult(entry);
          await this.sleep(this.pauseMs);
        }
        if (this.later) { this.queue = [...this.queue, ...this.later.filter((q) => !this.queue.some((x) => x.id === q.id))]; this.later = null; await this.saveState(); }
        this.current = null;
        this.running = false;
      })();
      return `started: ${this.queue.length} games`;
    },
    stop() { this.stopRequested = true; return "will stop after the current game"; },
    status() {
      const by = {};
      for (const r of Object.values(this.results)) by[r.status] = (by[r.status] || 0) + 1;
      const pending = Object.values(this.results).filter((r) => (r.status === "ok" || r.status === "kingdom") && !r.exported && !r.delivered).length;
      const recent = Object.values(this.results).sort((a, b) => b.at - a.at).slice(0, 4).map((r) => `${r.id} ${r.status}${r.error ? ": " + r.error : ""}`);
      return { running: this.running, current: this.current || null, left: this.queue.length, ...by, notExported: pending, lastError: this.lastError || null, lastDeliveryError: this.lastDeliveryError || null, recent };
    },
    // Arms the next Cmd+C to copy up to n captured games (and kingdoms) as JSON lines for `scry import`.
    exportNext(n = 25) {
      const rs = Object.values(this.results).filter((r) => (r.status === "ok" || r.status === "kingdom") && !r.exported && !r.delivered).slice(0, n);
      const text = rs.map((r) => JSON.stringify(r.status === "ok" ? r.record : { id: r.id, kingdom: r.kingdom })).join("\n") + "\n";
      document.addEventListener("copy", (e) => {
        e.clipboardData.setData("text/plain", text);
        e.preventDefault();
        for (const r of rs) { r.exported = true; this.save(r); }
      }, { once: true, capture: true });
      return `${rs.length} games armed (${text.length} chars); press Cmd+C in the page`;
    },

    // ---- finding games: the Profile tab lists a player's 25 latest games with both
    // players' levels, whether they were rated, and the kingdom

    watchProfiles() {
      const P = window.ProfileResult && window.ProfileResult.prototype;
      if (!P || P.scryWatched) return;
      // myResigned is the last field the constructor sets
      Object.defineProperty(P, "myResigned", {
        configurable: true,
        set(v) { Object.defineProperty(this, "myResigned", { value: v, writable: true, enumerable: true, configurable: true }); (window.scryCapture.seen = window.scryCapture.seen || []).push(this); },
      });
      P.scryWatched = true;
    },
    // Opens a player's profile and returns the games it lists.
    async profile(name) {
      this.watchProfiles();
      const page = () => document.querySelector('[ng-click="$ctrl.toggleSearch()"]');
      const input = () => document.querySelector('input[ng-model="$ctrl.searchInput"]');
      if (!page()) {
        const t = this.tab("Profile");
        if (!t) throw new Error("no Profile tab (leave the table first)");
        t.click();
        await this.waitFor(page, 8000, "the profile page");
        await this.sleep(2000); // your own profile loads first
      }
      if (!input()) page().click(); // the player search opens from the magnifier
      const search = await this.waitFor(input, 8000, "the player search");
      const mark = (this.seen || []).length, lower = name.toLowerCase();
      const theirs = () => this.seen.slice(mark).filter((r) => (r.players || []).some((p) => p.name.toLowerCase() === lower));
      this.setInput(search, name);
      await this.sleep(300);
      const button = [...search.parentElement.querySelectorAll("*")].find((e) => e.textContent.trim() === "Search");
      if (!button) throw new Error("no Search button next to the player search");
      button.click();
      await this.waitFor(() => theirs().length, 15000, name + "'s results");
      await this.sleep(1500);
      return theirs();
    },
    // Opens the Profile tab, which shows your own profile first, and returns your
    // name (the one player in all its games) and the games.
    async ownProfile() {
      this.watchProfiles();
      const mark = (this.seen || []).length;
      const t = this.tab("Profile");
      if (!t) throw new Error("no Profile tab (leave the table first)");
      t.click();
      await this.waitFor(() => (this.seen || []).length > mark, 15000, "your profile's games");
      await this.sleep(1500);
      const games = this.seen.slice(mark);
      const names = games.map((r) => new Set((r.players || []).map((p) => p.name)));
      const me = [...names[0]].find((n) => names.every((s) => s.has(n)));
      if (!me) throw new Error("can't tell whose profile this is");
      return { me, games };
    },
    // Keeps what a profile lists about a game as its meta: when it was played,
    // whether it was rated, both players' levels and scores, and a row like DomBot's.
    noteGame(r, owner) {
      const names = (r.players || []).map((p) => p.name);
      const other = names.find((n) => n !== owner) || names[1];
      const scores = { [owner]: r.myScore };
      for (const o of r.opponents || []) scores[o.player.name] = o.score;
      const date = new Date(r.timeMillis);
      const day = `${date.getUTCFullYear()}-${date.toLocaleString("en", { month: "short", timeZone: "UTC" })}-${String(date.getUTCDate()).padStart(2, "0")}`;
      const id = String(r.gameId);
      this.meta[id] = {
        time: date.toISOString(), rated: !!r.rated, players: names,
        levels: { [owner]: Math.round(r.myLevel * 100) / 100, [other]: Math.round(r.opponentLevel * 100) / 100 },
        scores, row: `${day} | ${names.map((n) => `${n}: ${scores[n]}`).join(", ")}`,
      };
      return id;
    },
    // Visits the profiles of strong players and queues their rated two-player games
    // where a player's level is above minLevel. It starts from your own profile's
    // opponents (or seeds) and follows strong opponents from there.
    async harvest({ seeds = [], minLevel = 50, maxProfiles = 30, me, skip = [], start = true } = {}) {
      if (this.running) return "a run is going; the profile pages would interrupt it. Stop it first";
      await this.load();
      if (!me) me = (await this.ownProfile()).me;
      const saved = new Set(skip.map(String)); // games already saved elsewhere
      try { await this.leave(); } catch (e) { /* not at a table */ }
      const level = {}, visited = new Set(), found = [];
      const todo = [me, ...seeds];
      while (todo.length && visited.size < maxProfiles) {
        todo.sort((a, b) => (level[b] || 0) - (level[a] || 0));
        const name = todo.shift();
        if (visited.has(name)) continue;
        visited.add(name);
        let rs;
        try { rs = await this.profile(name); } catch (e) { this.lastError = String(e.message || e); continue; }
        for (const r of rs) {
          const names = (r.players || []).map((p) => p.name);
          if (names.length !== 2) continue;
          const owner = names.find((n) => n.toLowerCase() === name.toLowerCase()) || name;
          const other = names.find((n) => n !== owner) || names[1];
          level[owner] = r.myLevel; level[other] = r.opponentLevel;
          if (r.opponentLevel > minLevel && !visited.has(other) && !todo.includes(other)) todo.push(other);
          const id = String(r.gameId);
          if (!r.rated || Math.max(r.myLevel, r.opponentLevel) <= minLevel || this.meta[id] || saved.has(id)) continue;
          found.push(this.noteGame(r, owner));
        }
      }
      await this.saveState();
      this.lastHarvest = { profiles: visited.size, found: found.length, strongSeen: Object.values(level).filter((l) => l > minLevel).length };
      return start ? this.run(found) : this.lastHarvest;
    },
  };
  window.scryCapture = C;
  return "scryCapture ready";
})();
