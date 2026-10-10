// Scrying Pool capture script for dominion.games. Loads FINISHED games by ID through
// My Table > Load Old Game, copies each game's log and score screen, and exports them
// as JSON lines for `scry import`. It only clicks the client's own buttons (My Table,
// Load Old Game, Load from End, the bot seat, Ready, Resign, Yes, Ok, Leave Table) and
// reads the page. Games where a player took fewer than minTurns turns (AFK) are dropped.
//
// Use: paste into the console (or a javascript_tool call) on dominion.games while logged in, then
//   scryCapture.run(["184518886", ...])      // runs in the background; progress: scryCapture.status()
//   scryCapture.exportNext(25)               // then press Cmd+C in the page; `pbpaste | scry import`
(() => {
  const C = {
    minTurns: 3,
    pauseMs: 4000,
    results: {}, // id -> {status: "ok"|"short"|"error", record?, error?}
    exported: {},
    queue: [],
    running: false,
    stopRequested: false,

    sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
    visible: (e) => !!(e && (e.offsetParent !== null || getComputedStyle(e).position === "fixed")),
    byText(txt) {
      const all = [...document.querySelectorAll("*")].filter((e) => this.visible(e) && e.textContent.trim() === txt);
      return all.find((e) => ![...e.children].some((c) => c.textContent.trim() === txt)) || null;
    },
    byLabel(txt) {
      return [...document.querySelectorAll("[title],[aria-label]")].find((e) => (e.getAttribute("title") === txt || e.getAttribute("aria-label") === txt) && this.visible(e)) || null;
    },
    async waitFor(fn, ms = 20000, what = "the page") {
      const t0 = Date.now();
      while (Date.now() - t0 < ms) { const v = fn(); if (v) return v; await this.sleep(300); }
      throw new Error("timed out waiting for " + what);
    },
    // The lobby menu shows "My Table" at a table and "New Table" elsewhere; the login page shows neither.
    loggedIn() { return !!this.byText("Logout") || !!document.querySelector(".log-scroll-container"); },

    extractLog() {
      const rows = [...document.querySelectorAll(".log-scroll-container .log-line")];
      const lines = rows.map((r) => {
        const m = /padding-left:\s*([\d.]+)%/.exec(r.getAttribute("style") || "");
        const level = m ? Math.round(parseFloat(m[1]) / 4.54545) : 0;
        const kind = r.classList.contains("new-turn-line") ? "T" : r.classList.contains("shuffles-line") ? "S" : "";
        return level + kind + "\t" + [...r.children].map((c) => c.textContent.trim()).filter(Boolean).join("|");
      });
      return "# scrying-pool log v1\n" + lines.join("\n") + "\n";
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

    async captureId(id) {
      const resultRe = /You (Resigned|Lose|Win|Won|Tie)/;
      await this.toMyTable();
      let input = this.gameIdInput();
      if (!input) {
        (await this.waitFor(() => this.byText("Load Old Game"), 8000, "Load Old Game")).click();
        input = await this.waitFor(() => this.gameIdInput(), 8000, "the Game Id field");
      }
      this.setInput(input, id);
      await this.sleep(500);
      this.byText("Load from End").click();
      await this.waitFor(() => { const d = this.decisionInput(); return d && /^\d+$/.test(d.value) && d.value !== "0"; }, 15000, "the decision index");
      await this.sleep(800);
      const original = (document.body.innerText.match(/Original Player: [^\n]+/g) || []).map((s) => s.slice(17));
      await this.seatBot();
      (await this.waitFor(() => this.byText("Ready"), 8000, "Ready")).click();
      let n = 0, same = 0;
      await this.waitFor(() => {
        const m = document.querySelectorAll(".log-scroll-container .log-line").length;
        if (m > 3 && m === n) same++; else same = 0;
        n = m;
        return same >= 6 || (m > 3 && resultRe.test(document.body.innerText) && same >= 2);
      }, 45000, "the log to finish loading");
      const log = this.extractLog();
      if (!resultRe.test(document.body.innerText)) {
        (this.byLabel("Resign") || this.byText("Resign")).click();
        (await this.waitFor(() => this.byText("Yes"), 8000, "Yes")).click();
        (await this.waitFor(() => this.byText("Ok"), 8000, "Ok")).click();
      }
      await this.waitFor(() => resultRe.test(document.body.innerText), 10000, "the score screen");
      const t = document.body.innerText, k = t.search(resultRe);
      const score = t.slice(k, t.indexOf("Players (", k));
      (await this.waitFor(() => this.byText("Leave Table"), 8000, "Leave Table")).click();
      await this.sleep(1200);
      return { id, original, log, score };
    },
    // Puts the page back on the lobby after an error: finishes an open game and leaves the table.
    async recover() {
      try {
        if (document.querySelector(".log-scroll-container") && !/You (Resigned|Lose|Win|Won|Tie)/.test(document.body.innerText)) {
          (this.byLabel("Resign") || this.byText("Resign"))?.click();
          (await this.waitFor(() => this.byText("Yes"), 5000))?.click();
          (await this.waitFor(() => this.byText("Ok"), 5000))?.click();
        }
      } catch (e) { /* not in a game */ }
      try { (await this.waitFor(() => this.byText("Leave Table"), 4000))?.click(); } catch (e) { /* no table */ }
      await this.sleep(1500);
    },

    async run(ids, meta = {}) {
      if (this.running) return "already running";
      this.queue = ids.filter((id) => !this.results[id]);
      this.running = true; this.stopRequested = false;
      (async () => {
        while (this.queue.length && !this.stopRequested) {
          if (!this.loggedIn()) { this.lastError = "logged out"; break; }
          const id = this.queue.shift();
          try {
            const rec = await this.captureId(id);
            rec.row = meta[id] || "";
            const turns = this.minTurnsIn(rec.log);
            this.results[id] = turns < this.minTurns ? { status: "short", turns } : { status: "ok", record: rec };
          } catch (e) {
            this.results[id] = { status: "error", error: String(e.message || e) };
            await this.recover();
          }
          await this.sleep(this.pauseMs);
        }
        this.running = false;
      })();
      return `started: ${this.queue.length} games`;
    },
    stop() { this.stopRequested = true; return "will stop after the current game"; },
    status() {
      const by = { ok: 0, short: 0, error: 0 };
      for (const r of Object.values(this.results)) by[r.status]++;
      const notExported = Object.entries(this.results).filter(([id, r]) => r.status === "ok" && !this.exported[id]).length;
      const errors = Object.entries(this.results).filter(([, r]) => r.status === "error").slice(-3).map(([id, r]) => id + ": " + r.error);
      return { running: this.running, left: this.queue.length, ...by, notExported, lastError: this.lastError || null, recentErrors: errors };
    },
    // Arms the next Cmd+C to copy up to n captured games as JSON lines for `scry import`.
    exportNext(n = 25) {
      const ids = Object.entries(this.results).filter(([id, r]) => r.status === "ok" && !this.exported[id]).slice(0, n).map(([id]) => id);
      const text = ids.map((id) => JSON.stringify(this.results[id].record)).join("\n") + "\n";
      document.addEventListener("copy", (e) => { e.clipboardData.setData("text/plain", text); e.preventDefault(); for (const id of ids) this.exported[id] = true; }, { once: true, capture: true });
      return `${ids.length} games armed (${text.length} chars); press Cmd+C in the page`;
    },
  };
  window.scryCapture = C;
  return "scryCapture ready";
})();
