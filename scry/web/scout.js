// Scout page (local app only): runs `scry scout` for a player through /api/scout,
// shows its progress, lists the players scouted before and shows a player's
// report, which the app makes from the saved games as Markdown.
"use strict";

const $ = (id) => document.getElementById(id);
const params = new URLSearchParams(location.search);
let shown = ""; // the player whose report is on the page
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

async function showReport(player) {
  shown = player;
  $("report-box").hidden = false;
  $("report-h").textContent = player;
  $("report-note").textContent = "Counting their games against yours…";
  $("report").replaceChildren();
  history.replaceState(null, "", "?player=" + encodeURIComponent(player));
  for (const b of $("list").querySelectorAll("button")) b.setAttribute("aria-pressed", String(b.dataset.player === player));
  try {
    const d = await api("/api/scout?player=" + encodeURIComponent(player) + "&n=" + encodeURIComponent($("n").value));
    if (shown !== player) return;
    $("report-note").textContent = "";
    render(d.markdown);
  } catch (e) {
    if (shown === player) $("report-note").textContent = e.message;
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
      if (d.run.running) return;
      clearInterval(polling);
      polling = null;
      if (!d.run.error) showReport(d.players.find((p) => p.player.toLowerCase() === d.run.player.toLowerCase())?.player || d.run.player);
    } catch (e) {
      showError(e.message);
    }
  }, 3000);
}

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
