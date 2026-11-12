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

const VP_COLORS = {
  Province: "var(--up)", Colony: "color-mix(in oklab, var(--up) 55%, var(--ink))", Duchy: "var(--coin)", Estate: "var(--axis)",
  "Other Victory cards": "var(--ink-2)", "VP tokens": "color-mix(in oklab, var(--coin) 50%, var(--ink))", "Landscapes and states": "var(--muted)",
};
const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const charts = [];

// One player's points: a bar scaled to the larger total, then each source as text with its number.
function vpRow(name, side, max) {
  const row = el("div", "vp-row");
  const bar = el("div", "vp-bar");
  bar.setAttribute("role", "img");
  bar.setAttribute("aria-label", `${name}: ${side.total} VP`);
  for (const x of side.vp) {
    if (x.vp <= 0 || !VP_COLORS[x.label]) continue;
    const seg = el("i");
    seg.style.width = (100 * x.vp) / max + "%";
    seg.style.background = VP_COLORS[x.label];
    seg.title = `${x.label} ${x.vp}`;
    bar.append(seg);
  }
  const head = el("div", "vp-head");
  head.append(el("b", "", name), el("span", "vp-total", side.total + " VP"));
  const list = el("div", "vp-list");
  for (const x of side.vp) {
    const item = el("span", "vp-src");
    const dot = el("i");
    dot.style.background = VP_COLORS[x.label] || "var(--down)";
    item.append(dot, `${x.label} ${x.vp}`);
    if (x.detail) item.title = x.detail;
    list.append(item);
  }
  if (!side.vp.length) list.append(el("span", "vp-src", "no points"));
  row.append(head, bar, list);
  const detail = side.vp.filter((x) => x.detail).map((x) => `${x.label}: ${x.detail}`);
  if (detail.length) row.append(el("div", "vp-detail", detail.join(". ")));
  return row;
}

function deckRow(name, side) {
  const row = el("div", "deck-row");
  row.append(el("div", "deck-head", `${name}: ${side.cards} cards`));
  const chips = el("div", "chips-line");
  for (const c of side.deck) chips.append(el("span", "dc " + c.kind, `${c.n} ${c.card}`));
  row.append(chips);
  return row;
}

function pointsCol(box, list, good) {
  const items = list.filter((p) => p.good === good);
  const col = el("div", "pts " + (good ? "well" : "badly"));
  col.append(el("h3", "", good ? "Went well" : "Went badly"));
  if (!items.length) col.append(el("p", "none", "Nothing the rules pick out."));
  else {
    const ul = el("ul");
    for (const p of items) ul.append(el("li", "", p.text));
    col.append(ul);
  }
  box.append(col);
}

function gapChart(host, g) {
  if (!window.echarts) return;
  const seats = {};
  const data = g.view.gap.map((p) => {
    seats[p.turn] = (seats[p.turn] || 0) + 1;
    return [p.turn + (seats[p.turn] > 1 ? 0.5 : 0), p.gap];
  });
  const ch = echarts.init(host, null, { renderer: "svg" });
  charts.push(ch);
  if (window.ResizeObserver) new ResizeObserver(() => ch.resize()).observe(host);
  const up = css("--up"), down = css("--down"), muted = css("--muted"), ink = css("--ink-2"), rule = css("--rule");
  ch.setOption({
    animation: false,
    grid: { left: 40, right: 12, top: 12, bottom: 28 },
    textStyle: { fontFamily: css("--num"), color: muted, fontSize: 12.5 },
    xAxis: { type: "value", min: 1, minInterval: 1, name: "turn", nameLocation: "end", nameGap: 6, nameTextStyle: { color: muted }, axisLine: { lineStyle: { color: rule } }, splitLine: { show: false } },
    yAxis: { type: "value", axisLine: { show: false }, splitLine: { lineStyle: { color: rule } }, axisLabel: { formatter: (v) => (v > 0 ? "+" + v : v) } },
    tooltip: { trigger: "axis", confine: true, valueFormatter: (v) => (v > 0 ? "+" + v : v) },
    series: [{
      type: "line", data, step: "end", symbolSize: 5, lineStyle: { color: ink, width: 2 },
      itemStyle: { color: (p) => (p.value[1] >= 0 ? up : down) },
      markLine: { silent: true, symbol: "none", label: { show: false }, lineStyle: { color: ink, type: "dashed" }, data: [{ yAxis: 0 }] },
    }],
  });
}

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
  box.append(top);

  const v = g.view;
  const body = el("div", "game-body");
  if (v) {
    const left = el("div", "col");
    const drawn = (side) => side.vp.reduce((n, x) => n + Math.max(x.vp, 0), 0);
    const max = Math.max(drawn(v.you), drawn(v.them), 1);
    const vps = el("div", "vps");
    vps.append(vpRow("You", v.you, max), vpRow(g.opponent, v.them, max));
    left.append(el("h3", "", "Points by source"), vps, el("h3", "", "Final decks"), deckRow("You", v.you), deckRow(g.opponent, v.them));
    if (v.note) left.append(el("p", "hint", v.note));

    const right = el("div", "col");
    const pts = el("div", "pts-grid");
    pointsCol(pts, v.points, true);
    pointsCol(pts, v.points, false);
    right.append(pts, el("h3", "", "Ending and swing"), el("p", "", v.end.text));
    if (v.swing) right.append(el("p", "", v.swing.text));
    const host = el("div", "ec ec-gap");
    host.setAttribute("aria-label", "Your VP minus theirs after each turn");
    right.append(host);
    if (v.gap.length) pendingCharts.push([host, g]);
    right.append(el("p", "hint", "Gap after each turn: above zero you lead."));
    body.append(left, right);
  }
  box.append(body);

  const more = el("details", "more");
  more.append(el("summary", "", "Other numbers"));
  const t = el("table", "sides");
  const buys = (o) => [o.opening, (o.opening_buys || []).join(" / ")].filter(Boolean).join(": ") || "–";
  table(t, ["", "You", g.opponent], [
    ["Opening", buys(g), buys(g.them)],
    ["First Gold", turn(g.first_gold), turn(g.them.first_gold)],
    ["First Province", turn(g.first_province), turn(g.them.first_province)],
    ["$ per turn, t3-12", "$" + one(g.coins_mid), "$" + one(g.them.coins_mid)],
    ["Draw luck", luckText(g.luck), luckText(g.them.luck)],
  ]);
  const sc = el("div", "scroll");
  sc.append(t);
  const lines = el("dl", "lines");
  const line = (k, val) => {
    const d = el("div");
    d.append(el("dt", "", k), val instanceof Node ? val : el("dd", "", val));
    lines.append(d);
  };
  if (g.verdict) line("Hands", el("dd", "verdict " + g.verdict, VERDICT[g.verdict]));
  if (g.turning) {
    const tp = g.turning, list = (x) => (x && x.length ? x.join(", ") : "nothing");
    line(`Turning point, turns ${tp.from}-${tp.to}`, `Gap ${tp.gap_before} to ${tp.gap_after}. You gained ${list(tp.my_gains)}. ${g.opponent} gained ${list(tp.their_gains)}.`);
  }
  line("Kingdom", g.kingdom.join(", ") + (g.landscapes && g.landscapes.length ? " · " + g.landscapes.join(", ") : ""));
  if (g.records && g.records.length) line("Your record with the cards you took", g.records.map((c) => `${c.card} ${pct(c.score)} of ${c.games}`).join("; "));
  const grid = el("div", "game-body");
  grid.append(sc, lines);
  more.append(grid);
  box.append(more);
  return box;
}

let pendingCharts = [];
let last = null;
function drawGames(r) {
  for (const c of charts.splice(0)) c.dispose();
  pendingCharts = [];
  $("games").replaceChildren(...r.games.map(gameCard));
  for (const [host, g] of pendingCharts) gapChart(host, g);
}

function render(r) {
  last = r;
  document.title = "Dominion Last Games";
  const who = r.me + (r.opponent ? " vs " + r.opponent : "");
  $("title").textContent = r.named ? `${who}: ${games_(r.games.length)} you named` : `${who}: last ${games_(r.games.length)}`;
  let meta = `Your newest saved game is from ${r.newest}. Games played since then need Load.`;
  if (r.missing && r.missing.length) meta += ` Not saved: ${r.missing.join(", ")}.`;
  else if (!r.named && r.games.length < r.asked) meta += ` Only ${r.games.length} of the ${r.asked} you asked for are saved.`;
  $("meta").textContent = meta;

  const u = r.usual, s = r.these, box = $("kpis");
  const seat = (x, name) => kpi(name, x.games ? `${points(x.score)}/${x.games}` : "–", x.games ? pct(x.score / x.games) + " of the points" : "no games");
  box.replaceChildren(
    kpi("Score", `${points(r.score)}/${s.games}`, r.leveled ? (r.leveled < s.games ? `${points(r.leveled_score)}/${r.leveled} with levels, expected ${one(r.expected)}` : `expected ${one(r.expected)} from the levels`) : "no levels to predict from"),
    kpi("Won, lost, tied", `${r.wins}-${r.losses}-${r.ties}`, u.games ? `your usual: ${pct(u.score)} of the points` : "no other games to compare"),
    seat(r.first_seat, "Going first"),
    seat(r.second_seat, "Going second"),
  );

  const bot = r.dombot || [];
  $("bot").hidden = !bot.length;
  $("bot").textContent = bot.map((p) => `${p.name}: ${p.time}, ${p.decisions} decisions at ${p.sec_per_decision} s each`).join(". ") + ".";
  $("usual-box").hidden = false;
  $("vs-box").hidden = !r.compare;
  if (r.compare) {
    const c = r.compare;
    table($("vs"), ["", r.me, r.opponent], [
      ["Level at the newest game", one(c.my_level), one(c.opp_level)],
      ["Head to head, saved", `${c.wins}-${c.losses}-${c.ties}`, ""],
      [`Last ${c.recent_days} days, saved`, `${pct(c.my_recent.score)} of ${c.my_recent.games}`, `${pct(c.opp_recent.score)} of ${c.opp_recent.games}`],
    ]);
  }
  table($("sides"), ["", "Games", "Score", "Turns", "$", "First Province", "No Province"], [
    statCells("These", s), statCells("Wins", r.win_stats), statCells("Losses", r.loss_stats), statCells("Usual", u),
  ]);

  $("notes-box").hidden = false;
  const found = r.findings || [];
  $("findings").replaceChildren(...(found.length ? found.map((f) => el("li", "", f.text)) : [el("li", "", "Nothing the rules pick out came up in 2 or more games.")]));
  $("notes-h2").hidden = !r.takeaways.length;
  $("notes").replaceChildren(...r.takeaways.map((t) => el("li", "", t)));
  drawGames(r);
  $("rules-note").hidden = false;
  $("luck-note").hidden = false;
}

// the charts take their colours from the stylesheet: draw them again when the theme changes
window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => last && drawGames(last));

let requests = 0;
async function show() {
  const mine = ++requests;
  showError("");
  const a = ask(), q = new URLSearchParams();
  if (a.ids) q.set("ids", a.ids);
  else q.set("last", a.last);
  if (a.opponent) q.set("opponent", a.opponent);
  history.replaceState(null, "", "?" + q);
  try {
    const r = await api("/api/report?" + q);
    if (mine === requests) render(r);
  } catch (e) {
    if (mine !== requests) return;
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
