// Scout page (local app only): runs `scry scout` for a player through /api/scout,
// shows its progress, lists the players scouted before and shows a player's
// report, which the app makes from the saved games as Markdown.
"use strict";

const $ = (id) => document.getElementById(id);
const params = new URLSearchParams(location.search);
let vs = params.get("vs") || ""; // the opponent filter a link names
let shown = ""; // the player whose report is on the page
let reportSeq = 0; // the latest report asked for; an older answer is dropped
let polling = null;

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}
async function api(path, body) {
  const res = await fetch(path, body === undefined ? {} : {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const d = await res.json();
  if (!res.ok) throw new Error(d.error || res.statusText);
  return d;
}
function showError(msg) {
  $("error").textContent = msg;
  $("error").hidden = !msg;
}
const day = (iso) => (iso || "").slice(0, 10);

// A report cell: "54% (40-67%), 7 of 13", with a share first in the styles
// tables ("33%, 58% (53-64%), 79.5 of 136").
const RATE = /^(?:(\d+%), )?(\d+%) \((\d+-\d+%)\), (.+)$/;
function cell(td, text) {
  const m = RATE.exec(text);
  if (!m) {
    td.textContent = text === "-" ? "–" : text;
    if (text === "-") td.className = "nil";
    return;
  }
  if (m[1]) td.append(el("span", "share", m[1] + " "));
  td.append(el("b", "", m[2]), " ", el("span", "ci", m[3] + " · " + m[4]));
}

// render draws scout's Markdown: headings, pipe tables, bullets and paragraphs.
function render(md) {
  const out = $("report");
  out.replaceChildren();
  const lines = md.split("\n");
  for (let i = 0; i < lines.length; ) {
    const l = lines[i];
    if (l.startsWith("# ")) {
      $("report-h").textContent = l.slice(2);
    } else if (l.startsWith("## ")) {
      out.append(el("h3", "", l.slice(3)));
    } else if (l.startsWith("|")) {
      const rows = [];
      for (; i < lines.length && lines[i].startsWith("|"); i++) rows.push(lines[i].trim().replace(/^\||\|$/g, "").split("|").map((c) => c.trim()));
      const wrap = el("div", "scroll"), t = el("table"), head = el("tr"), body = el("tbody");
      for (const h of rows[0]) head.append(el("th", "", h));
      for (const r of rows.slice(2)) {
        const tr = el("tr");
        const th = el("th", "", r[0]);
        th.scope = "row";
        tr.append(th);
        for (const c of r.slice(1)) {
          const td = el("td");
          cell(td, c);
          tr.append(td);
        }
        body.append(tr);
      }
      const thead = el("thead");
      thead.append(head);
      t.append(thead, body);
      wrap.append(t);
      out.append(wrap);
      continue;
    } else if (l.startsWith("- ")) {
      const ul = el("ul");
      for (; i < lines.length && lines[i].startsWith("- "); i++) ul.append(el("li", "", lines[i].slice(2)));
      out.append(ul);
      continue;
    } else if (l.trim()) {
      out.append(el("p", "", l));
    }
    i++;
  }
}

// ---- Battles: the player's listed games, with what the saved logs add

const RESULT = { 1: ["W", "win", "won"], 0.5: ["T", "tie", "tied"], 0: ["L", "loss", "lost"] };
const ENDINGS = { provinces: "Provinces", colonies: "Colonies", piles: "Three piles", resign: "Resignation, by whom isn't saved" };
const KINDS = { victory: "Victory", treasure: "Treasure", action: "Action", curse: "Curse" };
let battles = null; // the player's {me, games, images}

function deck(cards) {
  const box = el("div", "deck");
  const size = cards.reduce((n, c) => n + c.n, 0);
  box.append(el("span", "size", `${size} cards`));
  for (const kind of Object.keys(KINDS)) {
    const of = cards.filter((c) => c.kind === kind);
    if (!of.length) continue;
    const g = el("p", "grp " + kind);
    g.append(el("b", "", KINDS[kind] + " "), of.map((c) => `${c.n} ${c.card}`).join(", "));
    box.append(g);
  }
  return box;
}

function kingdom(g, images) {
  const box = el("div", "kc-row");
  for (const name of [...g.log.kingdom, ...g.log.landscapes]) {
    if (images[name]) {
      const im = el("img", "kc");
      im.src = images[name];
      im.alt = im.title = name;
      im.loading = "lazy";
      box.append(im);
    } else {
      box.append(el("span", "kc", name));
    }
  }
  return box;
}

function battle(g, player, images) {
  const [mark, word, said] = RESULT[g.result];
  const box = el("article", "battle");
  const top = el("div", "b-top");
  const title = el("h3");
  title.append(`${g.score}–${g.opp_score} ${said} against ${g.opponent}`);
  top.append(el("span", "res " + word, mark), title);
  const facts = [day(g.time)];
  if (g.level && g.opp_level) facts.push(`levels ${g.level.toFixed(1)} and ${g.opp_level.toFixed(1)}`);
  const l = g.log;
  if (l) facts.push(ENDINGS[l.ending] || l.ending, `${l.turns} turns`, `${l.first ? player : g.opponent} went first`);
  else facts.push("log not loaded");
  box.append(top, el("p", "facts", facts.join(" · ")));
  if (!l) return box;
  const a = el("a", "open", "Open the game");
  a.href = "analyze.html#" + g.id;
  box.append(kingdom(g, images));
  const decks = el("div", "decks");
  for (const [who, cards] of [[player, l.deck], [g.opponent, l.opp_deck]]) {
    const d = el("div", "who");
    d.append(el("h4", "", who), deck(cards));
    decks.append(d);
  }
  box.append(decks);
  if (!l.complete) box.append(el("p", "warn", "The log's VP differ from the score: a deck may be a card short."));
  box.append(a);
  return box;
}

function showBattles() {
  const box = $("battles");
  if (!battles) return;
  const q = $("opp").value.trim().toLowerCase();
  const rows = battles.games.filter((g) => g.opponent.toLowerCase().includes(q));
  const withLog = rows.filter((g) => g.log).length;
  $("battles-note").textContent = `${rows.length} of ${battles.games.length} listed games${q ? ` against "${$("opp").value.trim()}"` : ""}; ${withLog} with a saved log. Newest first.`;
  box.replaceChildren(...rows.map((g) => battle(g, battles.player, battles.images)));
  const url = new URL(location.href);
  if (q) url.searchParams.set("vs", $("opp").value.trim());
  else url.searchParams.delete("vs");
  history.replaceState(null, "", url);
}

function setBattles(b) {
  battles = b;
  $("battles-box").hidden = !b;
  if (!b) return;
  const n = {};
  for (const g of b.games) n[g.opponent] = (n[g.opponent] || 0) + 1;
  $("opps").replaceChildren(...Object.keys(n).sort().map((o) => {
    const x = el("option");
    x.value = o;
    x.label = `${o} (${n[o]})`;
    return x;
  }));
  $("vs-me").textContent = `Against ${b.me}`;
  $("vs-me").hidden = b.me.toLowerCase() === b.player.toLowerCase();
  showBattles();
}

async function showReport(player) {
  shown = player;
  const seq = ++reportSeq;
  $("report-box").hidden = false;
  $("report-h").textContent = player;
  $("report-note").textContent = "Counting their games against yours…";
  $("report").replaceChildren();
  $("battles-box").hidden = true;
  battles = null;
  $("opp").value = vs;
  vs = ""; // the link's filter is for the first report only
  history.replaceState(null, "", "?player=" + encodeURIComponent(player));
  for (const b of $("list").querySelectorAll("button")) b.setAttribute("aria-pressed", String(b.dataset.player === player));
  try {
    const d = await api("/api/scout?player=" + encodeURIComponent(player) + "&n=" + encodeURIComponent($("n").value));
    if (seq !== reportSeq) return;
    $("report-note").textContent = "";
    render(d.markdown);
    setBattles(d.battles);
  } catch (e) {
    if (seq === reportSeq) $("report-note").textContent = e.message;
  }
}

function showList(players) {
  const list = $("list");
  list.replaceChildren();
  $("list-note").textContent = players.length
    ? "Each fetch is kept in the games folder; a report is made again from the saved games."
    : "No one yet: type a name above.";
  for (const p of players) {
    const b = el("button", "pick");
    b.type = "button";
    b.dataset.player = p.player;
    b.setAttribute("aria-pressed", String(p.player === shown));
    b.append(el("b", "", p.player), el("span", "", `${p.saved} of ${p.listed} saved · ${day(p.fetched)}`));
    b.addEventListener("click", () => showReport(p.player));
    list.append(b);
  }
}

function showRun(run) {
  if (!run.player) return;
  $("run").hidden = false;
  const last = run.log.length ? run.log[run.log.length - 1] : "";
  $("dot").hidden = !run.running;
  $("run-text").textContent = run.running
    ? `Scouting ${run.player} since ${new Date(run.started).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false })}. ${last}`
    : run.error ? `Scouting ${run.player} stopped: ${run.error}` : `Scouted ${run.player}.`;
  $("log").textContent = run.log.join("\n");
  $("go").disabled = run.running;
}

async function refresh() {
  const d = await api("/api/scouts");
  showList(d.players);
  showRun(d.run);
  return d;
}

function poll() {
  if (polling) return;
  polling = setInterval(async () => {
    try {
      const d = await refresh();
      showError("");
      if (d.run.running) return;
      clearInterval(polling);
      polling = null;
      if (!d.run.error) showReport(d.players.find((p) => p.player.toLowerCase() === d.run.player.toLowerCase())?.player || d.run.player);
    } catch (e) {
      showError(e.message);
    }
  }, 3000);
}

$("opp").addEventListener("input", showBattles);
$("vs").addEventListener("submit", (ev) => ev.preventDefault());
$("vs-me").addEventListener("click", () => {
  $("opp").value = battles.me;
  showBattles();
});
$("vs-all").addEventListener("click", () => {
  $("opp").value = "";
  showBattles();
});

$("form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  showError("");
  try {
    showRun(await api("/api/scout", { player: $("player").value, n: $("n").value }));
    poll();
  } catch (e) {
    showError(e.message);
  }
});

(async () => {
  try {
    const d = await refresh();
    if (d.run.running) poll();
    const p = params.get("player") || d.players[0]?.player;
    if (p) showReport(p);
  } catch (e) {
    showError(e.message);
  }
})();
