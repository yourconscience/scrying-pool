// Profiles page: one player's saved games, optionally next to another player's.
// The local app serves the data from /api; `scry site` writes the same data as
// static JSON files next to the page. Charts are drawn at their container's
// width, so their text stays at its real size, and redrawn when it changes.
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
// a list item whose text has **...** parts in bold; names stay plain text
function item(side, text) {
  const span = el("span");
  text.split("**").forEach((part, i) => span.append(i % 2 ? el("b", "", part) : document.createTextNode(part)));
  const li = el("li", side);
  li.append(span);
  return li;
}
const pct = (x) => Math.round(x * 100) + "%";
const pp = (x) => (Math.round(x * 100) > 0 ? "+" : Math.round(x * 100) < 0 ? "−" : "") + Math.abs(Math.round(x * 100));
const ROLES = {
  village: ["Village in the kingdom", "the kingdom has a village"],
  draw: ["Draw in the kingdom", "the kingdom has card draw"],
  plus_buy: ["+Buy in the kingdom", "the kingdom has +Buy"],
  trasher: ["Trashing in the kingdom", "the kingdom has trashing"],
  curser: ["Cursing in the kingdom", "the kingdom has a curser"],
  attack: ["Attack in the kingdom", "the kingdom has an Attack"],
};
const ENDINGS = [["provinces", "Provinces"], ["piles", "Three piles"], ["resign", "Resigned"]];
const NS = "http://www.w3.org/2000/svg";

function svg(w, h, label) {
  const s = document.createElementNS(NS, "svg");
  s.setAttribute("viewBox", `0 0 ${w} ${h}`);
  s.setAttribute("width", w);
  s.setAttribute("height", h);
  s.setAttribute("role", "img");
  s.setAttribute("aria-label", label);
  s.add = (tag, attrs, text) => {
    const e = document.createElementNS(NS, tag);
    for (const [k, v] of Object.entries(attrs)) {
      // the stylesheet colours chart text, which beats a fill attribute
      if (k === "fill" && tag === "text") e.style.fill = v;
      else e.setAttribute(k, v);
    }
    if (text !== undefined) e.textContent = text;
    s.append(e);
    return e;
  };
  return s;
}
const widthOf = (box) => Math.max(280, Math.floor(box.clientWidth));

function key(items) {
  const box = el("div", "key");
  for (const [color, label, hollow] of items) {
    const s = el("span");
    const i = el("i");
    if (hollow) i.style.border = `2px solid ${color}`;
    else i.style.background = color;
    s.append(i, document.createTextNode(label));
    box.append(s);
  }
  return box;
}

// whether a 95% range lies clearly above or below a reference
const sideOf = (lo, hi, ref) => (lo > ref ? "up" : hi < ref ? "down" : "none");

// ---- data

let players = [];
const cache = {};
let current = null; // the [a, b] profiles on screen

async function getJSON(url) {
  const r = await fetch(url);
  const b = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(b.error || `HTTP ${r.status}`);
  return b;
}

async function loadPlayers() {
  const d = await getJSON(STATIC ? "data/players.json" : "/api/players");
  players = d.players || [];
  for (const id of ["player", "compare"]) {
    for (const p of players) {
      const o = el("option", "", `${p.name} (${p.games} games)`);
      o.value = p.name;
      $(id).append(o);
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

// ---- summary and takeaways

function kpis(a, b) {
  const tile = (label, va, vb, sub) => {
    const t = el("div", "kpi");
    const v = el("div", "value", va);
    if (vb !== undefined) v.append(el("span", "vs", vb));
    t.append(el("div", "label", label), v);
    if (sub) t.append(el("div", "sub", sub));
    return t;
  };
  const s = (d) => d.seat[0];
  $("kpis").replaceChildren(
    tile("Games", String(a.overall.games), b ? String(b.overall.games) : undefined, `${a.first_date || "?"} to ${a.last_date || "?"}`),
    tile("Won", pct(a.overall.rate), b ? pct(b.overall.rate) : undefined, `95% range ${pct(a.overall.low)} to ${pct(a.overall.high)}`),
    tile("First · second", `${pct(s(a).with.rate)} · ${pct(s(a).without.rate)}`, b ? `${pct(s(b).with.rate)} · ${pct(s(b).without.rate)}` : undefined, "win rate in each seat"),
    tile("Turns per game", a.avg_turns.toFixed(1), b ? b.avg_turns.toFixed(1) : undefined, "the player's own turns"),
  );
}

// Plain sentences for the differences whose 95% range leaves no doubt about the direction.
function takeaways(a, b) {
  const items = []; // [side, text, size]
  const splits = [[a.seat[0], "they go first"], ...a.roles.map((r) => [r, ROLES[r.name] ? ROLES[r.name][1] : r.name])];
  for (const [s, phrase] of splits) {
    if (!s.with.games || !s.without.games) continue;
    const side = sideOf(s.diff_low, s.diff_high, 0);
    if (side === "none") continue;
    items.push([side, `${a.me} wins **${Math.abs(Math.round(s.diff * 100))} points ${side === "up" ? "more" : "less"}** when ${phrase}: ${pct(s.with.rate)} of ${s.with.games} games against ${pct(s.without.rate)} of ${s.without.games}.`, Math.abs(s.diff)]);
  }
  for (const [k, r] of Object.entries(a.openings)) {
    const side = r.games >= 12 ? sideOf(r.low, r.high, a.overall.rate) : "none";
    if (side !== "none") items.push([side, `**${k}** openings win ${pct(r.rate)} of ${r.games} games, ${side === "up" ? "above" : "below"} the overall ${pct(a.overall.rate)}.`, Math.abs(r.rate - a.overall.rate)]);
  }
  for (const [k, label] of ENDINGS) {
    const r = a.endings[k];
    const side = r && r.games >= 12 ? sideOf(r.low, r.high, a.overall.rate) : "none";
    if (side !== "none") items.push([side, `Games that end on **${label.toLowerCase()}** are won ${pct(r.rate)} of the time, in ${r.games} games.`, Math.abs(r.rate - a.overall.rate)]);
  }
  items.sort((x, y) => y[2] - x[2]);
  const list = $("takeaways");
  list.replaceChildren();
  if (!items.length) list.append(item("none", `No difference is clear yet in ${a.overall.games} games: every range in the charts below still crosses its reference line.`));
  for (const [side, text] of items.slice(0, 6)) list.append(item(side, text));
  const t = a.tempo, i10 = t.turn.indexOf(10);
  if (i10 >= 0 && t.n_wins[i10] >= 10 && t.n_losses[i10] >= 10) {
    list.append(item("none", `On turn 10, ${a.me} averages **$${t.wins[i10].toFixed(1)}** in wins and **$${t.losses[i10].toFixed(1)}** in losses.`));
  }
  if (b) list.append(item("none", `${b.me} has won **${pct(b.overall.rate)}** of ${b.overall.games} saved games, ${a.me} **${pct(a.overall.rate)}** of ${a.overall.games}.`));
}

// ---- charts

// Each feature's difference (games with it − games without), as a point on its 95% range.
function effects(a, b) {
  const box = $("effects"), W = widthOf(box);
  const rows = [{ label: "Going first", get: (d) => d.seat[0] }];
  for (const r of a.roles) rows.push({ label: ROLES[r.name] ? ROLES[r.name][0] : r.name, get: (d) => d.roles.find((x) => x.name === r.name) });
  const sides = b ? [a, b] : [a];
  const usable = (sp) => sp && sp.with.games && sp.without.games;
  const narrow = W < 600, lines = sides.length;
  // wide: label | chart | value and games; narrow: label and games above the chart, value to its right
  const L = narrow ? 0 : Math.min(220, Math.round(W * 0.28)), R = narrow ? 62 : 190;
  const head = narrow ? 22 : 0, rowH = head + (lines > 1 ? 50 : 40), T = 24, H = T + rows.length * rowH + 4;
  let lim = 0.2;
  for (const row of rows) for (const d of sides) { const sp = row.get(d); if (usable(sp)) lim = Math.max(lim, -sp.diff_low, sp.diff_high); }
  lim = Math.min(0.6, Math.ceil(lim * 10 - 1e-9) / 10);
  const x0 = L + 24, x1 = W - R - 8, x = (v) => x0 + ((Math.max(-lim, Math.min(lim, v)) + lim) / (2 * lim)) * (x1 - x0);
  const s = svg(W, H, "Win rate difference with and without each feature");
  const ticks = [-lim, -lim / 2, 0, lim / 2, lim];
  // vertical grid lines; on a narrow screen they skip each row's label line
  const grid = (y1, y2) => ticks.forEach((v) => s.add("line", { x1: x(v), x2: x(v), y1, y2, stroke: v === 0 ? "var(--ink)" : "var(--rule)", "stroke-width": v === 0 ? 1.5 : 1, opacity: v === 0 ? 0.6 : 1 }));
  for (const v of ticks) s.add("text", { x: x(v), y: 13, "text-anchor": "middle", class: "small" }, v === 0 ? "0" : pp(v) + " pp");
  if (!narrow) grid(T - 6, H);
  rows.forEach((row, i) => {
    const y0 = T + i * rowH, band = y0 + head, bandH = rowH - head;
    if (i > 0) s.add("line", { x1: 0, x2: W, y1: y0, y2: y0, stroke: "var(--rule)" });
    if (narrow) grid(band, y0 + rowH);
    s.add("text", { x: 0, y: narrow ? y0 + 17 : y0 + rowH / 2 + 5, class: "label" }, row.label);
    sides.forEach((d, k) => {
      const sp = row.get(d);
      const cy = band + (lines > 1 ? bandH / 2 + (k ? 11 : -11) : bandH / 2);
      if (!usable(sp)) {
        s.add("text", { x: x(0) + 8, y: cy + 4, class: "small" }, "not enough games");
        return;
      }
      const side = sideOf(sp.diff_low, sp.diff_high, 0);
      const color = b ? (k ? "var(--b)" : "var(--a)") : `var(--${side})`;
      const soft = b ? (k ? "var(--b-soft)" : "var(--a-soft)") : `var(--${side}-soft)`;
      s.add("rect", { x: x(sp.diff_low), y: cy - 7, width: Math.max(4, x(sp.diff_high) - x(sp.diff_low)), height: 14, rx: 7, fill: soft, stroke: color, "stroke-width": 1.5 });
      s.add("circle", { cx: x(sp.diff), cy, r: 6.5, fill: side === "none" ? "var(--panel)" : color, stroke: color, "stroke-width": 2.5 });
      const games = `${sp.with.games} vs ${sp.without.games} games`;
      s.add("text", { x: W - R + 10, y: cy + 5, class: "value", fill: color }, pp(sp.diff) + " pp");
      if (!narrow) s.add("text", { x: W - R + 72, y: cy + 5, class: "small" }, games);
      else if (lines === 1) s.add("text", { x: W, y: y0 + 17, "text-anchor": "end", class: "small" }, games);
    });
  });
  box.replaceChildren(s);
  $("effects-key").replaceChildren(
    b
      ? key([["var(--a)", a.me], ["var(--b)", b.me], ["var(--muted)", "hollow: can't tell yet", true]])
      : key([["var(--up)", "clearly more wins"], ["var(--down)", "clearly fewer"], ["var(--none)", "can't tell yet", true]]),
  );
}

// Lines labelled at their right end. Each series spans the full width, so the
// last 30 games of players with different game counts line up at "now".
function lineChart(box, series, { min = 0, max, step = (max - min) / 4, yFmt, ref, xTicks, label }) {
  const short = (name) => (name.length > 18 ? name.slice(0, 17) + "…" : name);
  const longest = Math.max(...series.map((s) => short(s.name).length));
  const W = widthOf(box), H = 240, L = 44, R = 18 + 7.5 * longest, T = 12, B = 30;
  const x = (i, n) => L + (i * (W - L - R)) / Math.max(1, n - 1);
  const y = (v) => T + (H - T - B) * (1 - (v - min) / (max - min));
  const s = svg(W, H, label);
  for (let v = min; v <= max + 1e-9; v += step) {
    s.add("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), stroke: "var(--rule)" });
    s.add("text", { x: L - 8, y: y(v) + 4, "text-anchor": "end" }, yFmt(v));
  }
  if (ref !== undefined) s.add("line", { x1: L, x2: W - R, y1: y(ref), y2: y(ref), stroke: "var(--ink)", "stroke-dasharray": "5 5", opacity: 0.45 });
  xTicks(s, (i, n) => x(i, n), H);
  const ends = [];
  for (const ser of series) {
    const n = ser.values.length;
    const pts = ser.values.map((v, i) => (v === null || v === undefined ? null : [x(i, n), y(v)])).filter(Boolean);
    if (!pts.length) continue;
    s.add("polyline", { points: pts.map((p) => p.join(",")).join(" "), fill: "none", stroke: ser.color, "stroke-width": 3, "stroke-linejoin": "round", "stroke-linecap": "round" });
    const [lx, ly] = pts[pts.length - 1];
    s.add("circle", { cx: lx, cy: ly, r: 4.5, fill: ser.color });
    ends.push({ x: lx, y: ly, ser });
  }
  // keep end labels from overlapping
  ends.sort((p, q) => p.y - q.y);
  for (let i = 1; i < ends.length; i++) ends[i].y = Math.max(ends[i].y, ends[i - 1].y + 17);
  for (const e of ends) s.add("text", { x: e.x + 9, y: e.y + 5, class: "value", fill: e.ser.color }, short(e.ser.name));
  box.replaceChildren(s);
}

function form(a, b) {
  const box = $("form"), fa = a.form || [], fb = (b && b.form) || [];
  if (!fa.length && !fb.length) {
    box.replaceChildren(el("p", "hint", "Needs at least 30 games."));
    return;
  }
  const series = [];
  if (fa.length) series.push({ name: a.me, color: "var(--a)", values: fa.map((p) => p.rate) });
  if (fb.length) series.push({ name: b.me, color: "var(--b)", values: fb.map((p) => p.rate) });
  lineChart(box, series, {
    max: 1, yFmt: pct, ref: 0.5, label: "Win rate over the last 30 games",
    xTicks: (s, x, H) => {
      const pts = b ? null : fa;
      const ends = pts ? [pts[0].date || "earliest", pts[pts.length - 1].date || "latest"] : ["earlier", "latest"];
      s.add("text", { x: x(0, 2), y: H - 8, "text-anchor": "start" }, ends[0]);
      s.add("text", { x: x(1, 2), y: H - 8, "text-anchor": "end" }, ends[1]);
    },
  });
}

function tempo(a, b) {
  const t = a.tempo, enough = (vals, counts) => vals.map((v, i) => (counts[i] >= 5 ? v : null));
  const series = b
    ? [{ name: a.me, color: "var(--a)", values: t.all }, { name: b.me, color: "var(--b)", values: b.tempo.all }]
    : [{ name: "Wins", color: "var(--up)", values: enough(t.wins, t.n_wins) }, { name: "Losses", color: "var(--down)", values: enough(t.losses, t.n_losses) }];
  $("tempo-note").textContent = b
    ? "Average $ on each of the player's turns, in all their games."
    : "Average $ on each turn, in wins and in losses. Turns reached in fewer than 5 games are left out.";
  const top = Math.max(8, ...series.flatMap((s) => s.values.filter((v) => v !== null && v !== undefined)));
  lineChart($("tempo"), series, {
    max: Math.ceil(top / 2) * 2, step: 2, yFmt: (v) => "$" + v, label: "$ per turn",
    xTicks: (s, x, H) => t.turn.forEach((n, i) => {
      if (n === 1 || n % 5 === 0) s.add("text", { x: x(i, t.turn.length), y: H - 8, "text-anchor": "middle" }, n === 1 ? "turn 1" : n);
    }),
  });
}

function province(d) {
  const box = $("province"), h = d.first_province;
  const W = widthOf(box), H = 220, L = 34, R = 6, T = 12, B = 30;
  const n = h.turn.length, max = Math.max(2, ...h.wins, ...h.losses);
  const bw = (W - L - R) / n, y = (v) => T + (H - T - B) * (1 - v / max);
  const s = svg(W, H, "Turn of the first Province, in wins and in losses");
  for (const v of [0, Math.round(max / 2), max]) {
    s.add("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), stroke: "var(--rule)" });
    s.add("text", { x: L - 8, y: y(v) + 4, "text-anchor": "end" }, v);
  }
  h.turn.forEach((turn, i) => {
    const x0 = L + i * bw, w = Math.max(2, bw * 0.4);
    if (h.wins[i]) s.add("rect", { x: x0 + bw * 0.1, y: y(h.wins[i]), width: w, height: y(0) - y(h.wins[i]), fill: "var(--up)", rx: 2 });
    if (h.losses[i]) s.add("rect", { x: x0 + bw * 0.1 + w, y: y(h.losses[i]), width: w, height: y(0) - y(h.losses[i]), fill: "var(--down)", rx: 2 });
    if (turn === 1 || turn % 5 === 0) s.add("text", { x: x0 + bw / 2, y: H - 8, "text-anchor": "middle" }, turn === 1 ? "turn 1" : i === n - 1 ? turn + "+" : turn);
  });
  const median = (counts) => {
    const total = counts.reduce((p, q) => p + q, 0);
    let acc = 0;
    for (let i = 0; i < counts.length; i++) if ((acc += counts[i]) >= total / 2) return h.turn[i];
    return "?";
  };
  $("province-note").textContent = `${d.me}'s median is turn ${median(h.wins)} in wins and turn ${median(h.losses)} in losses; no Province at all in ${h.never[0]} wins and ${h.never[1]} losses.`;
  box.replaceChildren(s, key([["var(--up)", "Wins"], ["var(--down)", "Losses"]]));
}

// Win rates as points on their 95% ranges, against the overall rate (dashed).
function rateRows(box, title, rows, sides, base) {
  const W = widthOf(box), narrow = W < 440, L = narrow ? 92 : 112, R = narrow ? 50 : 112;
  const rowH = sides.length > 1 ? 44 : 32, T = 44, H = T + rows.length * rowH + 4;
  const x = (v) => L + v * (W - L - R);
  const s = svg(W, H, title);
  s.add("text", { x: 0, y: 15, class: "label" }, title);
  for (const v of [0, 0.5, 1]) s.add("text", { x: x(v), y: 36, "text-anchor": v ? (v === 1 ? "end" : "middle") : "start", class: "small" }, pct(v));
  s.add("line", { x1: x(base), x2: x(base), y1: T - 6, y2: H, stroke: "var(--ink)", "stroke-dasharray": "5 5", opacity: 0.45 });
  rows.forEach(([k, label], i) => {
    const y0 = T + i * rowH;
    s.add("line", { x1: L, x2: W - R, y1: y0 + rowH / 2, y2: y0 + rowH / 2, stroke: "var(--rule)" });
    s.add("text", { x: 0, y: y0 + rowH / 2 + 5, class: "label" }, label);
    sides.forEach(([get, mine], j) => {
      const r = get(k);
      if (!r || !r.games) return;
      const side = sideOf(r.low, r.high, base);
      const color = sides.length > 1 ? mine : `var(--${side})`;
      const soft = sides.length > 1 ? mine.replace(")", "-soft)") : `var(--${side}-soft)`;
      const cy = y0 + (sides.length > 1 ? rowH / 2 + (j ? 10 : -10) : rowH / 2);
      s.add("rect", { x: x(r.low), y: cy - 6, width: Math.max(4, x(r.high) - x(r.low)), height: 12, rx: 6, fill: soft, stroke: color, "stroke-width": 1.3 });
      s.add("circle", { cx: x(r.rate), cy, r: 5.5, fill: side === "none" && sides.length === 1 ? "var(--panel)" : color, stroke: color, "stroke-width": 2 });
      s.add("text", { x: W - R + 10, y: cy + 5, class: "value", fill: color }, pct(r.rate));
      if (!narrow) s.add("text", { x: W - R + 50, y: cy + 5, class: "small" }, `${r.games} games`);
    });
  });
  box.replaceChildren(s);
}

function endingsAndOpenings(a, b) {
  const sides = (pick) => [[(k) => pick(a, k), "var(--a)"], ...(b ? [[(k) => pick(b, k), "var(--b)"]] : [])];
  rateRows($("endings"), "How the game ended", ENDINGS, sides((d, k) => d.endings[k]), a.overall.rate);
  const opens = Object.entries(a.openings).sort((x, y) => y[1].games - x[1].games).slice(0, 6).map(([k]) => [k, k + " opening"]);
  rateRows($("openings"), "$ on turns 1 and 2", opens, sides((d, k) => d.openings[k]), a.overall.rate);
}

function tiles(box, rows, d) {
  box.replaceChildren();
  const base = d.overall.rate;
  for (const c of rows) {
    const t = el("a", "card-tile " + (c.shrunk < base - 0.05 ? "down" : c.shrunk > base + 0.05 ? "up" : "none"));
    t.href = "https://wiki.dominionstrategy.com/index.php/" + encodeURIComponent(c.card.replace(/ /g, "_"));
    t.target = "_blank";
    t.rel = "noopener";
    const art = el("div", "art");
    const src = d.images && d.images[c.card];
    if (src) {
      const img = el("img");
      img.alt = "";
      img.loading = "lazy";
      img.onerror = () => img.remove();
      img.src = src;
      art.append(img);
    }
    const rate = el("div", "rate");
    rate.append(el("b", "", pct(c.present.rate)), document.createTextNode(` won of ${c.present.games}`));
    const took = c.took.games ? `took it ${pct(c.took.rate)} (${c.took.games})` : "never took it";
    const skipped = c.skipped.games ? `skipped ${pct(c.skipped.rate)} (${c.skipped.games})` : "never skipped it";
    const body = el("div", "body");
    body.append(el("div", "name", c.card), rate, el("div", "split", took), el("div", "split", skipped));
    t.append(art, body);
    box.append(t);
  }
}

function drawCharts() {
  if (!current) return;
  const [a, b] = current;
  effects(a, b);
  form(a, b);
  tempo(a, b);
  province(a);
  endingsAndOpenings(a, b);
}

async function render() {
  const name = $("player").value, vs = $("compare").value;
  if (!name) return;
  try {
    const a = await profile(name);
    const b = vs && vs !== name ? await profile(vs) : null;
    current = [a, b];
    $("title").textContent = b ? `${a.me} and ${b.me}` : a.me;
    $("meta").textContent = `${a.overall.games} saved games from ${a.first_date || "?"} to ${a.last_date || "?"}. In ${a.complete} of them the replay ends exactly at the final scores; in the rest the log misses the game's last decision.`;
    $("csv").href = "/api/features.csv?player=" + encodeURIComponent(name);
    kpis(a, b);
    takeaways(a, b);
    drawCharts();
    const cards = a.cards || [];
    $("cards-note").textContent = `${a.me}'s kingdom cards seen in at least ${a.min_games} games (${cards.length} cards), ranked by win rate pulled toward the overall ${pct(a.overall.rate)} as if 10 average games were added, so a card seen a few times can't top the list by luck. "Took it" means the card was in the deck at some point. Each card links to its wiki page.`;
    const n = Math.min(12, Math.floor(cards.length / 2));
    tiles($("worst"), cards.slice(0, n), a);
    tiles($("best"), cards.slice(cards.length - n).reverse(), a);
    $("error").hidden = true;
  } catch (e) {
    $("error").textContent = e.message;
    $("error").hidden = false;
  }
}

let lastWidth = 0, resizeTimer;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    if (window.innerWidth !== lastWidth) drawCharts();
    lastWidth = window.innerWidth;
  }, 150);
});
$("player").addEventListener("change", render);
$("compare").addEventListener("change", render);
loadPlayers().then(render, (e) => {
  $("error").textContent = e.message;
  $("error").hidden = false;
});
