// Last games page (local app only): a report on your latest games or the games you
// name, from /api/report, and a run of `scry games` that loads the missing ones.
"use strict";

const $ = (id) => document.getElementById(id);
const RESULT = { 1: ["W", "win", "won"], 0.5: ["T", "tie", "tied"], 0: ["L", "loss", "lost"] };
const ENDINGS = { provinces: "Provinces", piles: "3 piles", resign: "resign" };
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
const one = (x) => x.toFixed(1);
const pct = (x) => Math.round(100 * x) + "%";
const points = (x) => String(Math.round(x * 10) / 10);
const signed = (x) => (x >= 0 ? "+" : "−") + Math.abs(x).toFixed(2);
const turn = (t) => (t ? "turn " + t : "none");
const games_ = (n) => n + (n === 1 ? " game" : " games");

function ask() {
  return { last: $("last").value, opponent: $("opponent").value.trim(), ids: $("ids").value.trim() };
}

function kpi(label, value, sub) {
  const k = el("div", "kpi");
  k.append(el("div", "l", label), el("div", "v", value), el("div", "s", sub));
  return k;
}

function table(box, head, rows) {
  const tr = el("tr");
  for (const h of head) tr.append(el("th", "", h));
  const thead = el("thead");
  thead.append(tr);
  const tbody = el("tbody");
  for (const row of rows) {
    const r = el("tr");
    row.forEach((c, i) => r.append(el(i ? "td" : "th", "", c)));
    tbody.append(r);
  }
  box.replaceChildren(thead, tbody);
}

function statCells(name, s) {
  if (!s.games) return [name, "0", "–", "–", "–", "–", "–"];
  return [name, String(s.games), pct(s.score), one(s.turns), "$" + one(s.coins),
    s.first_province ? "turn " + one(s.first_province) : "–", String(s.no_province)];
}

function luckText(l) {
  if (!l) return "hidden";
  return `${signed(l.luck)} $/turn, ${l.bad_hands} bad of ${l.hands} hands`;
}

const VERDICT = {
  draw: "Hands far below what the deck's treasure could give",
  deck: "Hands normal for the deck's treasure: look at the build and the play first",
  unclear: "The hands don't settle it",
};

function gameCard(g, i) {
  const [mark, word, said] = RESULT[g.result] || ["?", "", ""];
  const box = el("section", "panel game");
  const top = el("div", "game-top");
  const a = el("a", "", `Game ${i + 1}, ${g.opponent}`);
  a.href = "analyze.html#" + g.id;
  a.title = `Open game ${g.id} on the analysis page`;
  const h = el("h2");
  h.append(a);
  const vp = g.final || [g.my_vp, g.opp_vp];
  const facts = [`${said} ${vp[0]}–${vp[1]}`, ENDINGS[g.ending] || g.ending, `${g.turns} turns`, g.first ? "you went first" : "you went second"];
  if (g.chance) facts.push(`win chance ${pct(g.chance)} at levels ${one(g.my_level)} and ${one(g.opp_level)}`);
  facts.push(g.date);
  top.append(el("span", "res " + word, mark), h, el("span", "facts", facts.join(" · ")));

  const body = el("div", "game-body");
  const left = el("div", "scroll");
  const t = el("table", "sides");
  const buys = (o) => [o.opening, (o.opening_buys || []).join(" / ")].filter(Boolean).join(": ") || "–";
  table(t, ["", "You", g.opponent], [
    ["Opening", buys(g), buys(g.them)],
    ["First Gold", turn(g.first_gold), turn(g.them.first_gold)],
    ["First Province", turn(g.first_province), turn(g.them.first_province)],
    ["$ per turn, t3-12", "$" + one(g.coins_mid), "$" + one(g.them.coins_mid)],
    ["Draw luck", luckText(g.luck), luckText(g.them.luck)],
  ]);
  left.append(t);
  const lines = el("dl", "lines");
  const line = (k, v) => {
    const d = el("div");
    d.append(el("dt", "", k), v instanceof Node ? v : el("dd", "", v));
    lines.append(d);
  };
  if (g.verdict) line("Hands", el("dd", "verdict " + g.verdict, VERDICT[g.verdict]));
  if (g.turning) {
    const tp = g.turning, list = (x) => (x && x.length ? x.join(", ") : "nothing");
    line(`Turning point, turns ${tp.from}-${tp.to}`, `Gap ${tp.gap_before} to ${tp.gap_after}. You gained ${list(tp.my_gains)}. ${g.opponent} gained ${list(tp.their_gains)}.`);
  }
  line("Kingdom", g.kingdom.join(", ") + (g.landscapes && g.landscapes.length ? " · " + g.landscapes.join(", ") : ""));
  if (g.records && g.records.length) line("Your record with the cards you took", g.records.map((c) => `${c.card} ${pct(c.score)} of ${c.games}`).join("; "));
  body.append(left, lines);
  box.append(top, body);
  return box;
}

function render(r) {
  document.title = "Dominion Last Games";
  const who = r.me + (r.opponent ? " vs " + r.opponent : "");
  $("title").textContent = r.named ? `${who}: ${games_(r.games.length)} you named` : `${who}: last ${games_(r.games.length)}`;
  let meta = `Your newest saved game is from ${r.newest}. Games played since then need Load.`;
  if (r.missing && r.missing.length) meta += ` Not saved: ${r.missing.join(", ")}.`;
  else if (!r.named && r.games.length < r.asked) meta += ` Only ${r.games.length} of the ${r.asked} you asked for are saved.`;
  $("meta").textContent = meta;

  const u = r.usual, s = r.these, box = $("kpis");
  const against = (text, ok) => (ok ? text : "no other games to compare");
  box.replaceChildren(
    kpi("Score", `${points(r.score)}/${s.games}`, r.leveled ? (r.leveled < s.games ? `${points(r.leveled_score)}/${r.leveled} with levels, expected ${one(r.expected)}` : `expected ${one(r.expected)}`) : "no levels to predict from"),
    kpi("Won, lost, tied", `${r.wins}-${r.losses}-${r.ties}`, against(`your usual: ${pct(u.score)} of the points`, u.games)),
    kpi("$ per turn, t1-10", "$" + one(s.coins), against(`your usual: $${one(u.coins)}`, u.games)),
    kpi("First Province", s.first_province ? "turn " + one(s.first_province) : "none", against(`your usual: turn ${one(u.first_province)}`, u.first_province)),
  );

  $("vs-box").hidden = !r.compare;
  if (r.compare) {
    const c = r.compare;
    table($("vs"), ["", r.me, r.opponent], [
      ["Level at the newest game", one(c.my_level), one(c.opp_level)],
      ["Head to head, saved", `${c.wins}-${c.losses}-${c.ties}`, ""],
      [`Last ${c.recent_days} days, saved`, `${pct(c.my_recent.score)} of ${c.my_recent.games}`, `${pct(c.opp_recent.score)} of ${c.opp_recent.games}`],
    ]);
  }

  $("sides-box").hidden = false;
  table($("sides"), ["", "Games", "Score", "Turns", "$", "First Province", "No Province"], [
    statCells("These", s), statCells("Wins", r.win_stats), statCells("Losses", r.loss_stats), statCells("Usual", u),
  ]);

  $("notes-box").hidden = !r.takeaways.length;
  $("notes").replaceChildren(...r.takeaways.map((t) => el("li", "", t)));
  $("games").replaceChildren(...r.games.map(gameCard));
  $("luck-note").hidden = false;
}

async function show() {
  showError("");
  const a = ask(), q = new URLSearchParams();
  if (a.ids) q.set("ids", a.ids);
  else q.set("last", a.last);
  if (a.opponent) q.set("opponent", a.opponent);
  history.replaceState(null, "", "?" + q);
  try {
    render(await api("/api/report?" + q));
  } catch (e) {
    showError(e.message);
    $("meta").textContent = "";
  }
}

function showRun(run) {
  $("run").hidden = !run.started;
  $("load").disabled = run.running;
  $("dot").classList.toggle("off", !run.running);
  const last = run.log[run.log.length - 1] || "";
  $("run-text").textContent = run.running ? "Loading in the dominion.games tab… " + last
    : run.error ? "It stopped: " + run.error
    : "Done. " + last;
  $("log").textContent = run.log.join("\n");
}

async function follow() {
  try {
    const run = await api("/api/report/load");
    showRun(run);
    if (run.running) {
      polling = setTimeout(follow, 2000);
    } else if (polling) {
      polling = null;
      show();
    }
  } catch (e) {
    showError(e.message);
  }
}

$("form").addEventListener("submit", (e) => {
  e.preventDefault();
  show();
});
$("load").addEventListener("click", async () => {
  showError("");
  try {
    showRun(await api("/api/report/load", ask()));
    polling = setTimeout(follow, 2000);
  } catch (e) {
    showError(e.message);
  }
});

const params = new URLSearchParams(location.search);
for (const k of ["last", "opponent", "ids"]) if (params.get(k)) $(k).value = params.get(k);
follow();
show();
