// Profiles page: one player's saved games, optionally next to another player's.
// The local app serves the data from /api; `scry site` writes the same data as
// static JSON files next to the page.
"use strict";

const $ = (id) => document.getElementById(id);
const STATIC = document.querySelector('meta[name="scry-data"]').content === "static";
if (STATIC) {
  // a static copy is just this page: the brand leads back to it, the app's other pages aren't there
  document.body.classList.add("static");
  document.querySelector(".top .brand").href = "./";
  document.querySelector('.top a[aria-current="page"]').classList.add("local-only");
}

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}
const pct = (x) => Math.round(x * 100) + "%";
const pp = (x) => (x > 0 ? "+" : x < 0 ? "−" : "") + Math.abs(Math.round(x * 100));
const ROLE_NAMES = { village: "Village", draw: "Draw", plus_buy: "+Buy", trasher: "Trashing", curser: "Cursing", attack: "Attack" };
const ENDINGS = [["provinces", "Provinces"], ["piles", "Three piles"], ["resign", "Resigned"]];
const SVG = "http://www.w3.org/2000/svg";

function svg(w, h, label) {
  const s = document.createElementNS(SVG, "svg");
  s.setAttribute("viewBox", `0 0 ${w} ${h}`);
  s.setAttribute("role", "img");
  s.setAttribute("aria-label", label);
  s.add = (tag, attrs, text) => {
    const e = document.createElementNS(SVG, tag);
    for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
    if (text !== undefined) e.textContent = text;
    s.append(e);
    return e;
  };
  return s;
}

function legend(items) {
  const box = el("div", "legend");
  for (const [color, label] of items) {
    const s = el("span");
    const i = el("i");
    i.style.background = color;
    s.append(i, document.createTextNode(label));
    box.append(s);
  }
  return box;
}

// ---- data

let players = [];
const cache = {};

async function getJSON(url) {
  const r = await fetch(url);
  const b = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(b.error || `HTTP ${r.status}`);
  return b;
}

async function loadPlayers() {
  const d = await getJSON(STATIC ? "data/players.json" : "/api/players");
  players = d.players;
  for (const id of ["player", "compare"]) {
    const sel = $(id);
    for (const p of players) {
      const o = el("option", "", `${p.name} (${p.games})`);
      o.value = p.name;
      sel.append(o);
    }
  }
  $("player").value = d.default || (players[0] && players[0].name) || "";
  $("compare").disabled = players.length < 2;
  if (players.length < 2) $("compare").title = "Other profiles appear once their games are saved";
}

function profile(name) {
  if (!cache[name]) {
    const p = players.find((x) => x.name === name);
    cache[name] = getJSON(STATIC ? p.file : "/api/insights?player=" + encodeURIComponent(name));
  }
  return cache[name];
}

// ---- charts

// Diverging bars: each row's difference (with − without) and its 95% range.
function effects(a, b) {
  const rows = [{ key: "first", label: "Going first", get: (d) => d.seat[0] }];
  for (const r of a.roles) rows.push({ key: r.name, label: (ROLE_NAMES[r.name] || r.name) + " in the kingdom", desc: r.desc, get: (d) => d.roles.find((x) => x.name === r.name) });
  const sides = b ? [[a, "var(--a)"], [b, "var(--b)"]] : [[a, "var(--a)"]];
  const W = 760, L = 200, R = 150, rowH = b ? 34 : 26, T = 22, H = T + rows.length * rowH + 8;
  const lim = 0.4, x = (v) => L + ((Math.max(-lim, Math.min(lim, v)) + lim) / (2 * lim)) * (W - L - R);
  const s = svg(W, H, "Win rate difference by feature");
  for (const v of [-0.4, -0.2, 0, 0.2, 0.4]) {
    s.add("line", { x1: x(v), x2: x(v), y1: T - 6, y2: H - 4, stroke: v === 0 ? "var(--muted)" : "var(--rule)", "stroke-width": v === 0 ? 1.2 : 1 });
    s.add("text", { x: x(v), y: 12, "text-anchor": "middle" }, (v > 0 ? "+" : "") + Math.round(v * 100));
  }
  rows.forEach((row, i) => {
    const y0 = T + i * rowH;
    s.add("text", { x: 0, y: y0 + rowH / 2 + 4, class: "label" }, row.label);
    sides.forEach(([d, color], k) => {
      const sp = row.get(d);
      if (!sp || !sp.with.games || !sp.without.games) return;
      const y = y0 + (b ? 6 + k * 13 : rowH / 2 - 4);
      s.add("line", { x1: x(sp.diff_low), x2: x(sp.diff_high), y1: y + 4, y2: y + 4, stroke: color, "stroke-width": 1.5, opacity: 0.55 });
      s.add("rect", { x: Math.min(x(0), x(sp.diff)), y, width: Math.max(1.5, Math.abs(x(sp.diff) - x(0))), height: 8, rx: 2, fill: color });
      s.add("text", { x: W - R + 8, y: y + 8, fill: color }, `${pp(sp.diff)}  (${sp.with.games}/${sp.without.games})`);
    });
  });
  $("effects").replaceChildren(s, legend(b ? [["var(--a)", a.me], ["var(--b)", b.me]] : []));
}

function lineChart(target, series, { max, min = 0, xLabel, yFmt, refLine, label }) {
  const W = 380, H = 210, L = 38, R = 10, T = 10, B = 26;
  const n = Math.max(...series.map((s) => s.values.length));
  const x = (i) => L + (i * (W - L - R)) / Math.max(1, n - 1);
  const y = (v) => T + (H - T - B) * (1 - (v - min) / (max - min));
  const s = svg(W, H, label);
  for (let k = 0; k <= 4; k++) {
    const v = min + ((max - min) * k) / 4;
    s.add("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), stroke: "var(--rule)" });
    s.add("text", { x: L - 6, y: y(v) + 4, "text-anchor": "end" }, yFmt(v));
  }
  if (refLine !== undefined) s.add("line", { x1: L, x2: W - R, y1: y(refLine), y2: y(refLine), stroke: "var(--muted)", "stroke-dasharray": "4 4" });
  for (const ser of series) {
    const pts = ser.values.map((v, i) => (v === null ? null : `${x(i)},${y(v)}`)).filter(Boolean);
    if (pts.length) s.add("polyline", { points: pts.join(" "), fill: "none", stroke: ser.color, "stroke-width": 2.5, "stroke-linejoin": "round", "stroke-linecap": "round" });
  }
  xLabel(s, x, H);
  target.replaceChildren(s, legend(series.map((ser) => [ser.color, ser.name])));
}

function form(a, b) {
  const series = [{ name: a.me, color: "var(--a)", values: (a.form || []).map((p) => p.rate) }];
  if (b && (b.form || []).length) series.push({ name: b.me, color: "var(--b)", values: b.form.map((p) => p.rate) });
  if (!(a.form || []).length) {
    $("form").replaceChildren(el("p", "hint", "Needs at least 30 games."));
    return;
  }
  lineChart($("form"), series, {
    max: 1, yFmt: pct, refLine: 0.5, label: "Rolling win rate",
    xLabel: (s, x, H) => {
      const n = a.form.length;
      [[0, "start"], [Math.floor(n / 2), "middle"], [n - 1, "end"]].forEach(([i, anchor]) =>
        s.add("text", { x: x(i), y: H - 8, "text-anchor": anchor }, a.form[i].date || "game " + a.form[i].index));
    },
  });
}

function tempo(a, b) {
  const t = a.tempo, cut = (vals, counts) => vals.map((v, i) => (counts === null || counts[i] >= 5 ? v : null));
  const series = b
    ? [{ name: a.me + ", all games", color: "var(--a)", values: t.all }, { name: b.me + ", all games", color: "var(--b)", values: b.tempo.all }]
    : [{ name: "Wins", color: "var(--win)", values: cut(t.wins, t.n_wins) }, { name: "Losses", color: "var(--loss)", values: cut(t.losses, t.n_losses) }];
  $("tempo-note").textContent = b ? "Average $ on each of the player's turns." : "Average $ on each of your turns, in wins and in losses (turns with fewer than 5 games left out).";
  const max = Math.ceil(Math.max(8, ...series.flatMap((s) => s.values.filter((v) => v !== null))) / 2) * 2;
  lineChart($("tempo"), series, {
    max, yFmt: (v) => "$" + Math.round(v), label: "$ per turn",
    xLabel: (s, x, H) => t.turn.forEach((n, i) => { if (n === 1 || n % 5 === 0) s.add("text", { x: x(i), y: H - 8, "text-anchor": "middle" }, "T" + n); }),
  });
}

function province(d) {
  const h = d.first_province, W = 380, H = 200, L = 30, R = 8, T = 10, B = 26;
  const n = h.turn.length, max = Math.max(1, ...h.wins, ...h.losses);
  const bw = (W - L - R) / n, y = (v) => T + (H - T - B) * (1 - v / max);
  const s = svg(W, H, "Turn of the first Province");
  for (const v of [0, Math.ceil(max / 2), max]) {
    s.add("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), stroke: "var(--rule)" });
    s.add("text", { x: L - 6, y: y(v) + 4, "text-anchor": "end" }, v);
  }
  h.turn.forEach((turn, i) => {
    const x0 = L + i * bw;
    s.add("rect", { x: x0 + bw * 0.1, y: y(h.wins[i]), width: bw * 0.38, height: y(0) - y(h.wins[i]), fill: "var(--win)", rx: 1 });
    s.add("rect", { x: x0 + bw * 0.52, y: y(h.losses[i]), width: bw * 0.38, height: y(0) - y(h.losses[i]), fill: "var(--loss)", rx: 1 });
    if (turn === 1 || turn % 5 === 0) s.add("text", { x: x0 + bw / 2, y: H - 8, "text-anchor": "middle" }, turn === n ? turn + "+" : "T" + turn);
  });
  const median = (counts) => {
    const total = counts.reduce((a, b) => a + b, 0);
    let acc = 0;
    for (let i = 0; i < counts.length; i++) if ((acc += counts[i]) >= total / 2) return h.turn[i];
    return null;
  };
  $("province-note").textContent = `${d.me}: median turn ${median(h.wins)} in wins and ${median(h.losses)} in losses. No Province at all in ${h.never[0]} wins and ${h.never[1]} losses.`;
  $("province").replaceChildren(s, legend([["var(--win)", "Wins"], ["var(--loss)", "Losses"]]));
}

// Rate rows: a bar for the rate and a line for its 95% range.
function rateRows(target, title, rows, sides) {
  const W = 380, L = 110, R = 64, rowH = sides.length > 1 ? 30 : 22, T = title ? 20 : 4;
  const H = T + rows.length * rowH + 4, x = (v) => L + v * (W - L - R);
  const s = svg(W, H, title || "Rates");
  if (title) s.add("text", { x: 0, y: 12, class: "label" }, title);
  s.add("line", { x1: x(0.5), x2: x(0.5), y1: T - 4, y2: H - 2, stroke: "var(--muted)", "stroke-dasharray": "3 3" });
  rows.forEach(([key, label], i) => {
    const y0 = T + i * rowH;
    s.add("text", { x: 0, y: y0 + rowH / 2 + 4, class: "label" }, label);
    sides.forEach(([get, color], k) => {
      const r = get(key);
      if (!r || !r.games) return;
      const y = y0 + (sides.length > 1 ? 4 + k * 12 : rowH / 2 - 4);
      s.add("rect", { x: x(0), y, width: Math.max(1, x(r.rate) - x(0)), height: 8, rx: 2, fill: color, opacity: 0.85 });
      s.add("line", { x1: x(r.low), x2: x(r.high), y1: y + 4, y2: y + 4, stroke: "var(--ink)", "stroke-width": 1, opacity: 0.5 });
      s.add("text", { x: W - R + 6, y: y + 8 }, `${pct(r.rate)} (${r.games})`);
    });
  });
  target.replaceChildren(s);
}

function endingsAndOpenings(a, b) {
  const sides = (pick) => (b ? [[(k) => pick(a, k), "var(--a)"], [(k) => pick(b, k), "var(--b)"]] : [[(k) => pick(a, k), "var(--a)"]]);
  rateRows($("endings"), "How the game ended", ENDINGS, sides((d, k) => d.endings[k]));
  const opens = Object.entries(a.openings).sort((x, y) => y[1].games - x[1].games).slice(0, 6).map(([k]) => [k, k]);
  rateRows($("openings"), "Opening split", opens, sides((d, k) => d.openings[k]));
}

function cardTable(table, rows) {
  table.replaceChildren();
  const head = el("tr");
  for (const h of ["Card", "Games", "Won", "Took it", "Skipped it"]) head.append(el("th", "", h));
  table.append(head);
  for (const c of rows) {
    const tr = el("tr");
    const name = el("td");
    const link = el("a", "", c.card);
    link.href = "https://wiki.dominionstrategy.com/index.php/" + encodeURIComponent(c.card.replace(/ /g, "_"));
    link.target = "_blank";
    link.rel = "noopener";
    name.append(link);
    const cell = (r) => el("td", "", r.games ? `${pct(r.rate)} (${r.games})` : "–");
    tr.append(name, el("td", "", String(c.present.games)), el("td", "", pct(c.present.rate)), cell(c.took), cell(c.skipped));
    table.append(tr);
  }
}

function kpis(a, b) {
  const tile = (label, va, vb, sub) => {
    const t = el("div", "kpi");
    t.append(el("div", "label", label), el("div", "value", va));
    if (vb !== undefined) t.append(el("div", "value b", vb));
    if (sub) t.append(el("div", "sub", sub));
    return t;
  };
  const s = (d) => d.seat[0];
  $("kpis").replaceChildren(
    tile("Games", String(a.overall.games), b ? String(b.overall.games) : undefined, `${a.first_date || ""} – ${a.last_date || ""}`),
    tile("Won", pct(a.overall.rate), b ? pct(b.overall.rate) : undefined, `95% range ${pct(a.overall.low)}–${pct(a.overall.high)}`),
    tile("First / second", `${pct(s(a).with.rate)} / ${pct(s(a).without.rate)}`, b ? `${pct(s(b).with.rate)} / ${pct(s(b).without.rate)}` : undefined, "win rate going first and second"),
    tile("Turns per game", a.avg_turns.toFixed(1), b ? b.avg_turns.toFixed(1) : undefined, "the player's own turns"),
  );
}

async function render() {
  const name = $("player").value, vs = $("compare").value;
  if (!name) return;
  try {
    const a = await profile(name);
    const b = vs && vs !== name ? await profile(vs) : null;
    $("title").textContent = b ? `${a.me} vs ${b.me}` : `${a.me}'s games`;
    $("meta").textContent = `${a.overall.games} games from ${a.first_date || "?"} to ${a.last_date || "?"}. In ${a.complete} of them the replay ends exactly at the final scores; in the others the log misses the last decision.`;
    $("csv").href = "/api/features.csv?player=" + encodeURIComponent(name);
    kpis(a, b);
    effects(a, b);
    form(a, b);
    tempo(a, b);
    province(a);
    endingsAndOpenings(a, b);
    const cards = a.cards || [];
    $("cards-note").textContent = `${a.me}'s kingdom cards seen in at least ${a.min_games} games (${cards.length}), ranked by win rate pulled toward the overall ${pct(a.overall.rate)} as if 10 average games were added. "Took it": the card was in the deck at some point.`;
    cardTable($("worst"), cards.slice(0, 10));
    cardTable($("best"), cards.slice(-10).reverse());
    $("error").hidden = true;
  } catch (e) {
    $("error").textContent = e.message;
    $("error").hidden = false;
  }
}

$("player").addEventListener("change", render);
$("compare").addEventListener("change", render);
loadPlayers().then(render, (e) => {
  $("error").textContent = e.message;
  $("error").hidden = false;
});
