// Profiles page: one player's saved games. Every number on it is counted here
// from the games the filters keep, the way advisor.Summarize counts them
// (scry/internal/advisor/insights.go). The local app serves the data from /api;
// `scry site` writes the same data as static JSON files next to the page, each
// game without its opponent or id. The charts that zoom (the games over time,
// $ per turn, the first Province) are ECharts; the rest are SVG.
"use strict";

const $ = (id) => document.getElementById(id);
const STATIC = document.querySelector('meta[name="scry-data"]').content === "static";
if (STATIC) {
  // the hosted site has this page as its index and the Match notes page; the app's other pages aren't there
  document.body.classList.add("static");
  document.querySelector(".top .brand").href = "./";
  document.querySelector('.top a[aria-current="page"]').href = "./";
}

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}
const NS = "http://www.w3.org/2000/svg";
function svg(w, h, label) {
  const s = document.createElementNS(NS, "svg");
  s.setAttribute("viewBox", `0 0 ${w} ${h}`);
  s.setAttribute("width", w);
  s.setAttribute("height", h);
  s.setAttribute("role", "img");
  s.setAttribute("aria-label", label);
  s.add = (tag, attrs = {}, text) => {
    const e = document.createElementNS(NS, tag);
    for (const [k, v] of Object.entries(attrs)) if (v !== undefined && v !== null) e.setAttribute(k, v);
    if (text !== undefined) e.textContent = text;
    s.append(e);
    return e;
  };
  return s;
}
const pct = (x) => Math.round(x * 100) + "%";
const pp = (x) => (Math.round(x * 100) > 0 ? "+" : Math.round(x * 100) < 0 ? "−" : "") + Math.abs(Math.round(x * 100));
const money = (x) => "$" + x.toFixed(1);
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const shortDate = (d) => d.replace(/^\d+-(\w+)-0?(\d+)$/, "$1 $2"); // "2026-Oct-08" as "Oct 8"
const wiki = (c) => "https://wiki.dominionstrategy.com/index.php/" + encodeURIComponent(c.replace(/ /g, "_"));

const ROLES = { village: "with a village", draw: "with card draw", plus_buy: "with +Buy", trasher: "with trashing", curser: "with a curser", attack: "with an Attack" };
const ENDINGS = { provinces: "Provinces ran out", piles: "Three piles ran out", resign: "Someone resigned" };
// how the 7 starting Coppers fell between turns 1 and 2; "other" is a start with an Heirloom or Baker's Coffers
const OPENINGS = ["5/2", "4/3", "3/4", "2/5", "other"];
const SHORT_ENDINGS = { provinces: "Provinces", piles: "three piles", resign: "resigned" };

// ---- the statistics

const Z = 1.2816, PRIOR = 10, MIN_CARD_GAMES = 8, TEMPO_TURNS = 15, MAX_TURN = 20, FORM_WINDOW = 30;

// a share of games won, ties counting half, with an 80% Wilson interval
function rate(results) {
  const n = results.length, wins = results.reduce((a, b) => a + b, 0);
  if (!n) return { games: 0, wins: 0, rate: 0, low: 0, high: 0 };
  const p = wins / n, mid = (p + (Z * Z) / (2 * n)) / (1 + (Z * Z) / n);
  const half = (Z * Math.sqrt((p * (1 - p)) / n + (Z * Z) / (4 * n * n))) / (1 + (Z * Z) / n);
  return { games: n, wins, rate: p, low: mid - half, high: mid + half };
}
// games with something against games without it, with an 80% range of the difference
function split(withIt, without) {
  const a = rate(withIt), b = rate(without);
  if (!a.games || !b.games) return null;
  const diff = a.rate - b.rate, se = Math.sqrt((a.rate * (1 - a.rate)) / a.games + (b.rate * (1 - b.rate)) / b.games);
  return { with: a, without: b, diff, low: diff - Z * se, high: diff + Z * se };
}
const res = (gs) => gs.map((g) => g.result);

// A card counts once it was in MIN_CARD_GAMES games, or in fewer when the
// filters keep fewer games: about 1 in 35 kingdoms has a given card, so 100
// games rarely hold any card 8 times.
function count(gs) {
  const minCard = Math.max(3, Math.min(MIN_CARD_GAMES, Math.round((MIN_CARD_GAMES * gs.length) / Math.max(1, all.length))));
  const s = { n: gs.length, overall: rate(res(gs)), endings: {}, openings: {}, roles: [], cards: [], minCard };
  const base = s.overall.rate;
  s.first = rate(res(gs.filter((g) => g.first)));
  s.second = rate(res(gs.filter((g) => !g.first)));
  s.seat = split(res(gs.filter((g) => g.first)), res(gs.filter((g) => !g.first)));
  s.avgTurns = gs.length ? gs.reduce((a, g) => a + g.turns, 0) / gs.length : 0;
  for (const k of Object.keys(ENDINGS)) s.endings[k] = rate(res(gs.filter((g) => g.ending === k)));
  const byOpen = {};
  for (const g of gs) if (g.opening) (byOpen[g.opening] ||= []).push(g.result);
  s.openingUnknown = gs.filter((g) => !g.opening).length;
  for (const [k, v] of Object.entries(byOpen)) s.openings[k] = rate(v);
  const withKingdom = gs.filter((g) => g.kingdom && g.kingdom.length);
  for (const name of Object.keys(ROLES)) {
    const sp = split(res(withKingdom.filter((g) => g.roles && g.roles[name])), res(withKingdom.filter((g) => !(g.roles && g.roles[name]))));
    if (sp) s.roles.push({ name, ...sp });
  }
  const cards = {};
  for (const g of gs) {
    for (const k of g.kingdom || []) {
      const a = (cards[k] ||= { present: [], took: [], skipped: [] });
      a.present.push(g.result);
      (g.took && g.took[k] ? a.took : a.skipped).push(g.result);
    }
  }
  for (const [card, a] of Object.entries(cards)) {
    if (a.present.length < minCard) continue;
    const p = rate(a.present);
    s.cards.push({ card, present: p, took: rate(a.took), skipped: rate(a.skipped), shrunk: (p.wins + PRIOR * base) / (p.games + PRIOR) });
  }
  s.cards.sort((a, b) => a.shrunk - b.shrunk || a.card.localeCompare(b.card));

  s.tempo = { turn: [], wins: [], losses: [], nWins: [], nLosses: [] };
  for (let t = 1; t <= TEMPO_TURNS; t++) {
    let w = 0, l = 0, nw = 0, nl = 0;
    for (const g of gs) {
      if (t > g.coins.length) continue;
      if (g.result === 1) { w += g.coins[t - 1]; nw++; }
      if (g.result === 0) { l += g.coins[t - 1]; nl++; }
    }
    s.tempo.turn.push(t);
    s.tempo.wins.push(nw ? w / nw : 0);
    s.tempo.losses.push(nl ? l / nl : 0);
    s.tempo.nWins.push(nw);
    s.tempo.nLosses.push(nl);
  }
  const h = { turn: [], wins: Array(MAX_TURN).fill(0), losses: Array(MAX_TURN).fill(0), never: [0, 0] };
  for (let t = 1; t <= MAX_TURN; t++) h.turn.push(t);
  for (const g of gs) {
    if (g.result !== 1 && g.result !== 0) continue;
    const k = g.result === 1 ? 0 : 1;
    if (!g.first_province) { h.never[k]++; continue; }
    (k ? h.losses : h.wins)[Math.min(g.first_province, MAX_TURN) - 1]++;
  }
  s.province = h;
  return s;
}

function medianTurn(turns, counts) {
  const total = counts.reduce((p, q) => p + q, 0);
  let acc = 0;
  for (let i = 0; i < counts.length; i++) if ((acc += counts[i]) >= total / 2) return turns[i];
  return null;
}

// ---- filters

let D = null; // the profile on screen
let all = []; // its games, oldest first
const F = { lo: 0, hi: 0, seat: "any", opp: "any", picks: [] }; // lo..hi: the indices of the games the period keeps
const EXCLUSIVE = { ending: true, opening: true }; // one pick of these at a time; cards and roles add up

function keep(g, i, period = true) {
  if (period && (i < F.lo || i > F.hi)) return false;
  if (F.seat !== "any" && g.first !== (F.seat === "first")) return false;
  if (F.opp !== "any") {
    if (!(g.my_level > 0 && g.opp_level > 0)) return false;
    if (F.opp === "stronger" ? g.opp_level <= g.my_level : g.opp_level >= g.my_level) return false;
  }
  for (const p of F.picks) {
    if (p.kind === "card" && !(g.kingdom || []).includes(p.key)) return false;
    if (p.kind === "ending" && g.ending !== p.key) return false;
    if (p.kind === "opening" && g.opening !== p.key) return false;
    if (p.kind === "role" && !(g.kingdom && g.kingdom.length && g.roles && g.roles[p.key])) return false;
  }
  return true;
}
const picked = (kind, key) => F.picks.some((p) => p.kind === kind && p.key === key);
function toggle(kind, key) {
  if (picked(kind, key)) F.picks = F.picks.filter((p) => !(p.kind === kind && p.key === key));
  else F.picks = [...F.picks.filter((p) => !(EXCLUSIVE[kind] && p.kind === kind)), { kind, key }];
  update(true);
}
const pickLabel = (p) => (p.kind === "card" ? `${p.key} in the kingdom` : p.kind === "ending" ? ENDINGS[p.key] : p.kind === "opening" ? (p.key === "other" ? "Other openings" : `${p.key} opening`) : `Kingdoms ${ROLES[p.key]}`);

function setPeriod(lo, hi) {
  F.lo = Math.max(0, lo);
  F.hi = Math.min(all.length - 1, hi);
  if (charts.timeline) charts.timeline.dispatchAction({ type: "dataZoom", startValue: F.lo, endValue: F.hi });
  else update(false);
}

function filterBar() {
  const N = all.length;
  const period = F.lo === 0 && F.hi === N - 1 ? "all" : F.hi === N - 1 && F.lo === N - 100 ? "100" : F.hi === N - 1 && F.lo === N - 30 ? "30" : "";
  for (const [id, v] of [["f-period", period], ["f-seat", F.seat], ["f-opp", F.opp]]) {
    for (const b of $(id).querySelectorAll("button")) b.setAttribute("aria-pressed", String(b.dataset.v === v));
  }
  $("f-period").querySelector('[data-v="100"]').disabled = N <= 100;
  $("f-period").querySelector('[data-v="30"]').disabled = N <= 30;
  const rated = all.filter((g) => g.my_level > 0 && g.opp_level > 0).length;
  $("f-opp").hidden = rated === 0;
  $("f-opp").title = `rating levels are known for ${rated} of ${N} games`;
  const chips = [];
  if (!period) { // a period picked on the games over time: shown here for the other views
    const b = el("button", "chip");
    b.type = "button";
    const label = `${shortDate(all[F.lo].date)} to ${shortDate(all[F.hi].date)}`;
    b.setAttribute("aria-label", "Remove filter: " + label);
    b.append(document.createTextNode(label), el("b", "", "×"));
    b.onclick = () => setPeriod(0, N - 1);
    chips.push(b);
  }
  $("chips").replaceChildren(...chips, ...F.picks.map((p) => {
    const b = el("button", "chip");
    b.type = "button";
    b.setAttribute("aria-label", "Remove filter: " + pickLabel(p));
    if (p.kind === "card" && D.images && D.images[p.key]) {
      const im = el("img");
      im.alt = "";
      im.src = D.images[p.key];
      b.append(im);
    }
    b.append(document.createTextNode(pickLabel(p)), el("b", "", "×"));
    b.onclick = () => toggle(p.kind, p.key);
    return b;
  }));
  $("reset").hidden = period === "all" && F.seat === "any" && F.opp === "any" && !F.picks.length;
}

for (const [id, key] of [["f-seat", "seat"], ["f-opp", "opp"]]) {
  $(id).addEventListener("click", (e) => {
    const b = e.target.closest("button");
    if (!b) return;
    F[key] = b.dataset.v;
    update(true);
  });
}
$("f-period").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  const N = all.length;
  setPeriod(b.dataset.v === "all" ? 0 : N - Number(b.dataset.v), N - 1);
});
$("reset").addEventListener("click", () => {
  F.seat = "any";
  F.opp = "any";
  F.picks = [];
  update(true);
  setPeriod(0, all.length - 1);
});

// ---- one tooltip for the SVG parts: any element with data-tip shows it on hover or focus

const tip = el("div", "tip");
tip.setAttribute("role", "tooltip");
tip.hidden = true;
document.body.append(tip);
let tipFor = null, tipHTML = "";
function showTip(t, x, y) {
  if (t !== tipFor || t.dataset.tip !== tipHTML) { // rebuild only when the target or its text changes
    tipFor = t;
    tipHTML = t.dataset.tip;
    tip.innerHTML = tipHTML;
  }
  tip.hidden = false;
  const r = tip.getBoundingClientRect();
  tip.style.left = Math.min(window.innerWidth - r.width - 8, Math.max(8, x + 14)) + "px";
  tip.style.top = (y + 18 + r.height > window.innerHeight ? y - r.height - 12 : y + 18) + "px";
}
const hideTip = () => { tip.hidden = true; tipFor = null; };
document.addEventListener("pointermove", (e) => {
  const t = e.target.closest && e.target.closest("[data-tip]");
  if (t) showTip(t, e.clientX, e.clientY);
  else hideTip();
});
document.addEventListener("focusin", (e) => {
  const t = e.target.closest && e.target.closest("[data-tip]");
  if (!t) return;
  const r = t.getBoundingClientRect();
  showTip(t, r.left + r.width / 2, r.bottom - 10);
});
document.addEventListener("focusout", hideTip);
window.addEventListener("scroll", hideTip, { passive: true });

// ---- ECharts

const charts = {};
function chartTheme() {
  return {
    ink: css("--ink"), ink2: css("--ink-2"), muted: css("--muted"), rule: css("--rule"), axis: css("--axis"), up: css("--up"), down: css("--down"),
    panel: css("--panel"), neutral: css("--neutral"), tipBg: css("--tip-bg"), tipInk: css("--tip-ink"), num: css("--num"), sans: css("--sans"),
  };
}
function baseOption(c) {
  return {
    animationDuration: 300,
    textStyle: { fontFamily: c.num, color: c.muted, fontSize: 11.5 },
    tooltip: {
      backgroundColor: c.tipBg, borderWidth: 0, padding: [8, 10], textStyle: { color: c.tipInk, fontFamily: c.sans, fontSize: 13 },
      extraCssText: "border-radius:10px;box-shadow:none;", confine: true,
    },
  };
}
function chart(id) {
  if (!window.echarts) {
    $(id).replaceChildren(el("p", "note", "This chart needs the ECharts script from cdn.jsdelivr.net, which didn't load."));
    return null;
  }
  if (!charts[id]) {
    charts[id] = echarts.init($(id), null, { renderer: "svg" });
    new ResizeObserver(() => charts[id] && charts[id].resize()).observe($(id));
  }
  return charts[id];
}

let formWindow = FORM_WINDOW; // the line's window, which the tooltip names

// The games over time: each game a tick at the top (won) or bottom (lost), the
// win rate over the last 30 of them as a line. The slider under it and dragging
// or pinching the plot pick the period the rest of the page counts.
function timeline() {
  const ch = chart("timeline");
  const pts = [], line = [];
  all.forEach((g, i) => { if (keep(g, i, false)) pts.push([i, g.result, g]); });
  const W = (formWindow = Math.min(FORM_WINDOW, Math.max(5, Math.floor(pts.length / 3))));
  let sum = 0;
  pts.forEach((p, k) => {
    sum += p[1];
    if (k >= W) sum -= pts[k - W][1];
    if (k >= W - 1) line.push([p[0], sum / W]);
  });
  const base = rate(pts.map((p) => p[1])).rate;
  $("time-note").textContent = pts.length >= 10 ? `the line is the win rate over the last ${W} games; drag the handles, or drag and pinch the plot, to pick a period` : "too few games for a line";
  if (!ch) return;
  const c = chartTheme(), N = all.length;
  const dateOf = (v) => (all[Math.round(v)] ? shortDate(all[Math.round(v)].date) : "");
  const series = [
    { id: "won", type: "scatter", data: pts.filter((p) => p[1] === 1).map((p) => [p[0], 1.06]), symbolSize: 5, itemStyle: { color: c.up, opacity: 0.8 }, silent: true, clip: false },
    { id: "lost", type: "scatter", data: pts.filter((p) => p[1] === 0).map((p) => [p[0], -0.06]), symbolSize: 5, itemStyle: { color: c.down, opacity: 0.8 }, silent: true, clip: false },
    {
      id: "form", type: "line", data: pts.length >= 10 ? line : [], showSymbol: false, lineStyle: { width: 2.5, color: c.ink }, itemStyle: { color: c.ink }, z: 3,
      markLine: { silent: true, symbol: "none", lineStyle: { color: c.axis, type: "dashed" }, label: { show: true, position: "insideEndTop", formatter: () => `average ${pct(base)}`, color: c.ink2, fontFamily: c.sans }, data: pts.length ? [{ yAxis: base }] : [] },
    },
  ];
  if (ch.getOption()) { // the period stays where the slider is
    ch.setOption({ series });
    return;
  }
  ch.setOption({
    ...baseOption(c),
    grid: { left: 44, right: 16, top: 22, bottom: 80 },
    xAxis: { type: "value", min: 0, max: N - 1, axisLine: { lineStyle: { color: c.axis } }, axisTick: { show: false }, splitLine: { show: false }, axisLabel: { formatter: dateOf, hideOverlap: true, margin: 26, showMinLabel: false, showMaxLabel: false } },
    yAxis: { type: "value", min: 0, max: 1, interval: 0.25, axisLabel: { formatter: (v) => pct(v) }, splitLine: { lineStyle: { color: c.rule } } },
    tooltip: {
      ...baseOption(c).tooltip, trigger: "axis", axisPointer: { type: "line", lineStyle: { color: c.axis } },
      formatter: (ps) => {
        const p = ps.find((x) => x.seriesId === "form");
        if (!p) return "";
        const i = p.data[0], g = all[i];
        const how = g.result === 1 ? "won" : g.result === 0 ? "lost" : "tied";
        return `<b>Game ${i + 1}, ${esc(g.date)}</b><br>${how}, ${g.first ? "first" : "second"} seat, ${esc(SHORT_ENDINGS[g.ending] || g.ending)}${g.opening ? ", opened " + esc(g.opening) : ""}<br>last ${formWindow} games: ${pct(p.data[1])}`;
      },
    },
    dataZoom: [
      { type: "slider", xAxisIndex: 0, filterMode: "none", startValue: F.lo, endValue: F.hi, height: 26, bottom: 8, labelFormatter: dateOf, borderColor: c.rule, fillerColor: "rgba(17,148,106,0.14)", dataBackground: { lineStyle: { color: c.axis }, areaStyle: { color: c.neutral } }, textStyle: { color: c.ink2 }, brushSelect: false },
      { type: "inside", xAxisIndex: 0, filterMode: "none", zoomOnMouseWheel: "ctrl", moveOnMouseWheel: false, moveOnMouseMove: true },
    ],
    series,
  });
  ch.on("datazoom", () => {
    const dz = ch.getOption().dataZoom[0], last = N - 1;
    const lo = dz.startValue !== undefined ? dz.startValue : (dz.start / 100) * last;
    const hi = dz.endValue !== undefined ? dz.endValue : (dz.end / 100) * last;
    F.lo = Math.max(0, Math.ceil(lo - 1e-6));
    F.hi = Math.min(last, Math.floor(hi + 1e-6));
    update(false);
  });
}

function tempo(s) {
  const ch = chart("tempo");
  if (!ch) return;
  const c = chartTheme(), t = s.tempo, ok = (v, n) => v.map((x, i) => (n[i] >= 5 ? +x.toFixed(2) : null));
  ch.setOption({
    ...baseOption(c),
    grid: { left: 40, right: 12, top: 34, bottom: 26 },
    legend: { top: 0, right: 0, itemWidth: 14, itemHeight: 3, textStyle: { color: c.ink2, fontFamily: c.sans, fontSize: 13 } },
    xAxis: { type: "category", data: t.turn, boundaryGap: false, axisLine: { lineStyle: { color: c.axis } }, axisTick: { show: false }, axisLabel: { formatter: (v) => (v === "1" || v === 1 ? "turn 1" : v) } },
    yAxis: { type: "value", min: 0, axisLabel: { formatter: (v) => "$" + v }, splitLine: { lineStyle: { color: c.rule } } },
    tooltip: {
      ...baseOption(c).tooltip, trigger: "axis", axisPointer: { type: "line", lineStyle: { color: c.axis } },
      formatter: (ps) => {
        const i = ps[0].dataIndex;
        return `<b>Turn ${t.turn[i]}</b><br>wins: ${t.nWins[i] >= 5 ? money(t.wins[i]) + ` (${t.nWins[i]} games)` : "few games"}<br>losses: ${t.nLosses[i] >= 5 ? money(t.losses[i]) + ` (${t.nLosses[i]} games)` : "few games"}`;
      },
    },
    series: [
      { name: "wins", type: "line", data: ok(t.wins, t.nWins), symbol: "circle", symbolSize: 6, lineStyle: { width: 2.5 }, color: c.up },
      { name: "losses", type: "line", data: ok(t.losses, t.nLosses), symbol: "circle", symbolSize: 6, lineStyle: { width: 2.5 }, color: c.down },
    ],
  });
}

function province(s) {
  const h = s.province;
  const mw = medianTurn(h.turn, h.wins), ml = medianTurn(h.turn, h.losses);
  $("prov-note").textContent = s.n ? `Median: ${mw ? "turn " + mw : "none"} in wins, ${ml ? "turn " + ml : "none"} in losses. No Province at all in ${h.never[0]} wins and ${h.never[1]} losses.` : "";
  const ch = chart("prov");
  if (!ch) return;
  const c = chartTheme();
  ch.setOption({
    ...baseOption(c),
    grid: { left: 40, right: 12, top: 34, bottom: 26 },
    legend: { top: 0, right: 0, itemWidth: 12, itemHeight: 12, textStyle: { color: c.ink2, fontFamily: c.sans, fontSize: 13 } },
    xAxis: { type: "category", data: h.turn.map((t) => (t === MAX_TURN ? MAX_TURN + "+" : t)), axisLine: { lineStyle: { color: c.axis } }, axisTick: { show: false }, axisLabel: { interval: (i, v) => v === "1" || v === 1 || Number(v) % 5 === 0 || v === MAX_TURN + "+", formatter: (v) => (v === "1" || v === 1 ? "turn 1" : v) } },
    yAxis: { type: "value", axisLabel: { formatter: (v) => Math.abs(v) }, splitLine: { lineStyle: { color: c.rule } } },
    tooltip: {
      ...baseOption(c).tooltip, trigger: "axis", axisPointer: { type: "shadow", shadowStyle: { color: c.neutral, opacity: 0.5 } },
      formatter: (ps) => { const i = ps[0].dataIndex; return `<b>Turn ${h.turn[i]}${h.turn[i] === MAX_TURN ? " or later" : ""}</b><br>${h.wins[i]} wins, ${h.losses[i]} losses`; },
    },
    series: [
      { name: "wins", type: "bar", stack: "p", data: h.wins, color: c.up, itemStyle: { borderRadius: [3, 3, 0, 0] }, barCategoryGap: "18%" },
      { name: "losses", type: "bar", stack: "p", data: h.losses.map((v) => -v), color: c.down, itemStyle: { borderRadius: [0, 0, 3, 3] } },
    ],
  });
}

// ---- summary tiles

const side = (r, base) => (r > base + 0.05 ? "up" : r < base - 0.05 ? "down" : "none");
function kpis(s, gs) {
  const box = $("kpis"), base = s.overall.rate, everyone = rate(res(all)).rate;
  const tile = (l, v, cls, sub) => {
    const t = el("div", "panel kpi");
    t.append(el("div", "l", l), el("div", "v " + cls, v), el("div", "s", sub));
    box.append(t);
    return t;
  };
  const bars = (t, rows) => {
    const m = el("div", "mini");
    for (const [label, r] of rows) {
      const row = el("div", "r"), tr = el("div", "t"), f = el("i");
      f.style.width = r.games ? (r.rate * 100).toFixed(1) + "%" : "0";
      f.style.background = `var(--${side(r.rate, base)})`;
      tr.append(f);
      row.append(el("span", "", label), tr, el("span", "p", r.games ? pct(r.rate) : "-"));
      row.dataset.tip = `<div><b>${esc(label)}</b><br>${r.games ? `won ${pct(r.rate)} of ${r.games} games` : "no games"}</div>`;
      m.append(row);
    }
    t.append(m);
  };
  box.replaceChildren();
  const span = gs.length ? `${gs[0].date} to ${gs[gs.length - 1].date}` : "no games match the filters";
  tile("Games", String(s.n), "", s.n === all.length ? span : `of ${all.length}; ${span}`);
  const clear = s.n && (s.overall.low > everyone ? "up" : s.overall.high < everyone ? "down" : "");
  tile("Won", s.n ? pct(base) : "-", clear, s.n ? `80% range ${pct(s.overall.low)} to ${pct(s.overall.high)}${s.n < all.length ? `; all games ${pct(everyone)}` : ""}` : "");
  bars(tile("By seat", s.n ? `${s.first.games ? pct(s.first.rate) : "-"} / ${s.second.games ? pct(s.second.rate) : "-"}` : "-", "", "first / second"), [["going first", s.first], ["going second", s.second]]);
  const E = s.endings, top = Object.keys(E).filter((k) => E[k].games).sort((a, b) => E[b].rate - E[a].rate)[0];
  bars(tile("By ending", top ? pct(E[top].rate) : "-", "", top ? `best when ${ENDINGS[top].toLowerCase()}` : ""), [["Provinces", E.provinces], ["three piles", E.piles], ["resigned", E.resign]]);
  tile("Turns per game", s.n ? s.avgTurns.toFixed(1) : "-", "", "your own turns");
}

// ---- the kingdom cards: faces at both ends, every card as one square in 2-point bins

const img = (c) => (D.images && D.images[c]) || "";
const cardTip = (c) => `${img(c.card) ? `<img src="${img(c.card)}" alt="">` : ""}<div><b>${esc(c.card)}</b><br>adjusted ${pct(c.shrunk)}<br>won ${pct(c.present.rate)} of ${c.present.games}<br>${c.took.games ? `bought: ${pct(c.took.rate)} of ${c.took.games}` : "never bought"}<br>${c.skipped.games ? `skipped: ${pct(c.skipped.rate)} of ${c.skipped.games}` : "never skipped"}<br><i>click to show only its games</i></div>`;

function spectrum(s) {
  const cards = s.cards, base = s.overall.rate;
  $("spec-sub").textContent = `${cards.length} cards in at least ${s.minCard} of the games shown, by your win rate with them in the kingdom. One square per card; click a card to show only the games it was in.`;
  const face = (c, cls) => {
    const b = el("button", "face " + cls);
    b.type = "button";
    b.dataset.tip = `<div><b>${esc(c.card)}</b><br>adjusted ${pct(c.shrunk)}, won ${pct(c.present.rate)} of ${c.present.games}</div>`;
    b.setAttribute("aria-pressed", String(picked("card", c.card)));
    b.onclick = () => toggle("card", c.card);
    const im = el("img");
    im.alt = c.card;
    im.src = img(c.card);
    b.append(im, el("div", "p", pct(c.shrunk)));
    return b;
  };
  const n = Math.min(6, Math.floor(cards.length / 2));
  $("worst").replaceChildren(...cards.slice(0, n).map((c) => face(c, "down")));
  $("best").replaceChildren(...cards.slice(cards.length - n).reverse().map((c) => face(c, "up")));

  const box = $("waffle");
  if (!cards.length) {
    box.replaceChildren(el("p", "empty", `No kingdom card was in ${s.minCard} or more of these games.`));
    return;
  }
  const Wd = Math.max(300, box.clientWidth), narrow = Wd < 600;
  // a phone gets 6-point bins of smaller squares, so the columns stay short
  const lo = 0.24, hi = 0.72, step = narrow ? 0.06 : 0.02, bins = Math.round((hi - lo) / step);
  const sq = narrow ? 6 : 9, gap = narrow ? 1.5 : 2, cell = sq + gap, L = 6, colW = (Wd - 2 * L) / bins, per = Math.max(1, Math.floor((colW - 4) / cell));
  const groups = Array.from({ length: bins }, () => []);
  for (const c of cards) groups[Math.max(0, Math.min(bins - 1, Math.floor((c.shrunk - lo) / step)))].push(c);
  const tall = Math.max(...groups.map((g) => Math.ceil(g.length / per)));
  const T = 26, plotH = tall * cell, H = T + plotH + 26;
  const g = svg(Wd, H, "Kingdom cards by adjusted win rate");
  const xv = (v) => L + ((v - lo) / step) * colW;
  for (let v = 0.3; v <= 0.701; v += 0.1) {
    g.add("line", { x1: xv(v) - 2, x2: xv(v) - 2, y1: T - 4, y2: T + plotH + 4, stroke: "var(--field-line)" });
    g.add("text", { x: xv(v) - 2, y: H - 6, "text-anchor": "middle" }, pct(v));
  }
  const bx = xv(Math.max(lo, Math.min(hi, base))) - 2;
  g.add("line", { x1: bx, x2: bx, y1: 14, y2: T + plotH + 4, stroke: "var(--field-ink)", "stroke-dasharray": "4 3", "stroke-width": 1.5 });
  g.add("text", { x: bx > Wd - 150 ? bx - 4 : bx + 4, y: 12, class: "avg", "text-anchor": bx > Wd - 150 ? "end" : "start" }, `average ${pct(base)}`);
  groups.forEach((grp, b) => {
    const mid = lo + (b + 0.5) * step, k = Math.max(-1, Math.min(1, (mid - base) / 0.16));
    const fill = Math.abs(k) < 0.12 ? "var(--field-muted)" : `color-mix(in oklab, ${k > 0 ? "#22b382" : "#ec7746"} ${Math.round(45 + 55 * Math.abs(k))}%, #2b4a3c)`;
    const x0 = L + b * colW + (colW - Math.min(per, Math.max(1, grp.length)) * cell) / 2;
    grp.sort((a, c) => a.shrunk - c.shrunk).forEach((c, i) => {
      g.add("rect", { x: x0 + (i % per) * cell, y: T + plotH - (Math.floor(i / per) + 1) * cell, width: sq, height: sq, rx: 2, fill, "data-tip": cardTip(c), "data-card": c.card, class: picked("card", c.card) ? "on" : null });
    });
  });
  g.addEventListener("click", (e) => {
    const r = e.target.closest("rect[data-card]");
    if (r) toggle("card", r.dataset.card);
  });
  box.replaceChildren(g);
}

// ---- every comparison: a dot on its 80% range; openings drawn as coin pairs

function effects(s) {
  const base = s.overall.rate, out = [];
  const add = (e) => out.push({ ...e, clear: e.low > 0 ? "up" : e.high < 0 ? "down" : null });
  if (s.seat) {
    add({ kind: "seat", key: "first", title: "Going first", diff: s.seat.diff, low: s.seat.low, high: s.seat.high, games: s.n,
      detail: `${pct(s.first.rate)} going first (${s.first.games} games), ${pct(s.second.rate)} going second (${s.second.games})` });
  }
  for (const r of s.roles) {
    add({ kind: "role", key: r.name, title: ROLES[r.name], diff: r.diff, low: r.low, high: r.high, games: r.with.games,
      detail: `${pct(r.with.rate)} in ${r.with.games} games with it, ${pct(r.without.rate)} in ${r.without.games} without` });
  }
  const vs = (kind, key, title, r) => add({ kind, key, title, diff: r.rate - base, low: r.low - base, high: r.high - base, rate: r.rate, games: r.games, detail: `won ${pct(r.rate)} of ${r.games} games` });
  if (!F.picks.some((p) => p.kind === "ending")) for (const [k, r] of Object.entries(s.endings)) if (r.games) vs("ending", k, ENDINGS[k], r);
  if (!F.picks.some((p) => p.kind === "opening")) for (const k of OPENINGS) if (s.openings[k]) vs("opening", k, k === "other" ? "Other openings" : `${k} opening`, s.openings[k]);
  return out;
}

function forest(s) {
  const box = $("forest"), es = effects(s);
  $("open-note").textContent = s.openingUnknown ? ` ${s.openingUnknown} of these games are left out of the openings: their log comes from the other player's side, which hides your hands, and you didn't play every Copper.` : "";
  const groups = [["seat", "Seat"], ["ending", "Endings"], ["opening", "Openings"], ["role", "Kingdom"]];
  const rows = [];
  for (const [k, name] of groups) {
    const gr = es.filter((e) => e.kind === k);
    if (k !== "opening") gr.sort((a, b) => b.diff - a.diff); // openings keep their order: 5/2 to 2/5, then other
    if (gr.length) rows.push({ head: name }, ...gr);
  }
  if (!rows.length) {
    box.replaceChildren(el("p", "note", "Not enough games to compare anything."));
    return;
  }
  const W = Math.max(300, box.clientWidth), narrow = W < 560;
  // wide: label | chart | numbers; narrow: the label above its line
  const rowH = narrow ? 44 : 28, headH = 30, L = narrow ? 0 : 168, R = narrow ? 70 : 118, T = 22;
  const H = T + rows.reduce((h, r) => h + (r.head ? headH : rowH), 0) + 6;
  const lim = 0.4, x0 = L + (narrow ? 18 : 6), x1 = W - R - 16, x = (v) => x0 + ((Math.max(-lim, Math.min(lim, v)) + lim) / (2 * lim)) * (x1 - x0);
  const g = svg(W, H, "Win rate differences with 80% ranges");
  const defs = g.add("defs");
  defs.innerHTML = `<radialGradient id="coin" cx="35%" cy="30%" r="75%"><stop offset="0" stop-color="#ffe08a"/><stop offset="1" stop-color="#d79b12"/></radialGradient>`;
  let y = T;
  for (const r of rows) {
    if (r.head) { y += headH; continue; }
    g.add("rect", { x: 0, y, width: W, height: rowH, rx: 6, class: "hit" + (r.kind === "seat" ? " fixed" : ""), "data-kind": r.kind, "data-key": r.key,
      "data-tip": `<div><b>${esc(r.title)}</b><br>${esc(r.detail)}<br>${pp(r.diff)} pp, likely ${pp(r.low)} to ${pp(r.high)}${r.kind === "seat" ? "" : "<br><i>click to show only these games</i>"}</div>` });
    y += rowH;
  }
  for (const v of [-0.4, -0.2, 0, 0.2, 0.4]) {
    g.add("line", { x1: x(v), x2: x(v), y1: T - 6, y2: H - 4, stroke: v === 0 ? "var(--axis)" : "var(--rule)", "stroke-width": v === 0 ? 1.5 : 1, "pointer-events": "none" });
    g.add("text", { x: x(v), y: T - 10, "text-anchor": "middle" }, pp(v));
  }
  g.add("text", { x: W - R + 10, y: T - 10 }, narrow ? "pp games" : "pp  rate games");
  y = T;
  for (const r of rows) {
    if (r.head) { g.add("text", { x: 0, y: y + 21, class: "grp", "pointer-events": "none" }, r.head); y += headH; continue; }
    const cy = narrow ? y + 31 : y + rowH / 2, ly = narrow ? y + 13 : cy;
    const color = r.clear ? `var(--${r.clear})` : "var(--none)";
    if (r.kind === "opening" && r.key === "other") {
      g.add("text", { x: narrow ? 2 : 0, y: ly + 5, class: "rl", "pointer-events": "none" }, "other");
    } else if (r.kind === "opening") {
      r.key.split("/").forEach((v, i) => {
        g.add("circle", { cx: 11 + i * 25, cy: ly, r: narrow ? 9 : 11, fill: "url(#coin)", "pointer-events": "none" });
        g.add("text", { x: 11 + i * 25, y: ly + 4, "text-anchor": "middle", class: "coin-t", "pointer-events": "none" }, v);
      });
    } else {
      g.add("text", { x: narrow ? 2 : 0, y: ly + 5, class: "rl", "pointer-events": "none" }, r.title);
    }
    g.add("line", { x1: x(r.low), x2: x(r.high), y1: cy, y2: cy, stroke: color, "stroke-width": 3, "stroke-linecap": "round", opacity: 0.45, "pointer-events": "none" });
    g.add("circle", { cx: x(r.diff), cy, r: 6, fill: color, stroke: "var(--panel)", "stroke-width": 2, "pointer-events": "none" });
    const nums = narrow ? `${pp(r.diff).padStart(3)} ${String(r.games).padStart(4)}` : `${pp(r.diff).padStart(3)} ${r.rate !== undefined ? pct(r.rate).padStart(4) : "    "} ${String(r.games).padStart(4)}`;
    g.add("text", { x: W - R + 10, y: cy + 4, class: "vv" + (r.clear ? " " + r.clear : ""), "xml:space": "preserve", "pointer-events": "none" }, nums);
    y += rowH;
  }
  g.addEventListener("click", (e) => {
    const r = e.target.closest("rect.hit");
    if (r && r.dataset.kind !== "seat") toggle(r.dataset.kind, r.dataset.key);
  });
  box.replaceChildren(g);
}

// ---- the card table: sortable, searchable; a row shows only that card's games

let sort = "shrunk", dir = -1;
const COLS = [
  ["card", "Card", (c) => c.card, false],
  ["games", "Games", (c) => c.present.games, true],
  ["shrunk", "Adjusted", (c) => c.shrunk, true],
  ["present", "In kingdom", (c) => c.present.rate, true],
  ["took", "Bought it", (c) => (c.took.games ? c.took.rate : -1), true],
  ["skipped", "Skipped it", (c) => (c.skipped.games ? c.skipped.rate : -1), true],
];
function table(s) {
  const base = s.overall.rate, cards = s.cards;
  const tr = el("tr");
  for (const [k, label, , num] of COLS) {
    const th = el("th", num ? "r" : "");
    const b = el("button", "", label + (sort === k ? (dir < 0 ? " ↓" : " ↑") : ""));
    b.type = "button";
    if (sort === k) th.setAttribute("aria-sort", dir < 0 ? "descending" : "ascending");
    b.onclick = () => { dir = sort === k ? -dir : num ? -1 : 1; sort = k; table(s); };
    th.append(b);
    tr.append(th);
  }
  $("tbl").tHead.replaceChildren(tr);
  const q = $("q").value.trim().toLowerCase(), get = COLS.find((c) => c[0] === sort)[2];
  const rows = cards.filter((c) => !q || c.card.toLowerCase().includes(q)).sort((a, b) => {
    const va = get(a), vb = get(b);
    return (typeof va === "string" ? va.localeCompare(vb) : va - vb) * dir;
  });
  $("cards-count").textContent = `${rows.length} of ${cards.length} cards`;
  const rateTd = (r) => {
    const td = el("td", "n");
    if (!r.games) { td.textContent = "-"; td.className += " none-t"; return td; }
    td.className += " " + side(r.rate, base) + "-t";
    td.append(document.createTextNode(pct(r.rate) + " "), el("span", "none-t", `(${r.games})`));
    return td;
  };
  const body = rows.map((c) => {
    const row = el("tr", picked("card", c.card) ? "on" : ""), name = el("td", "card"), im = el("img");
    im.alt = "";
    im.loading = "lazy";
    if (img(c.card)) im.src = img(c.card);
    const a = el("a", "", c.card);
    a.href = wiki(c.card);
    a.target = "_blank";
    a.rel = "noopener";
    a.onclick = (e) => e.stopPropagation();
    name.append(im, a);
    const adj = el("td", "adj"), w = el("div", "adj-w"), t = el("div", "t"), f = el("i"), tick = el("b");
    f.style.width = (c.shrunk * 100).toFixed(1) + "%";
    f.style.background = `var(--${side(c.shrunk, base)})`;
    tick.style.left = (base * 100).toFixed(1) + "%";
    t.append(f, tick);
    w.append(t, el("span", side(c.shrunk, base) + "-t", pct(c.shrunk)));
    adj.append(w);
    row.append(name, el("td", "n", String(c.present.games)), adj, rateTd(c.present), rateTd(c.took), rateTd(c.skipped));
    row.onclick = () => toggle("card", c.card);
    return row;
  });
  if (!body.length) {
    const row = el("tr"), td = el("td", "empty", cards.length ? "No card has that name." : `No kingdom card was in ${s.minCard} or more of these games.`);
    td.colSpan = COLS.length;
    row.append(td);
    body.push(row);
  }
  $("tbl").tBodies[0].replaceChildren(...body);
  $("cards-note").textContent = `Cards in at least ${s.minCard} of the games shown (${MIN_CARD_GAMES} with every game shown, fewer when the filters keep fewer). Adjusted: the win rate with the card in the kingdom, pulled toward the ${pct(base)} of these games as if ${PRIOR} average games were added, so a card seen a few times can't top the list by luck; the tick on each bar is that ${pct(base)}. Bought and skipped split those games by whether you had a copy at some point. Click a row to show only its games.`;
}
$("q").addEventListener("input", () => current && table(current));

// ---- drawing

// ---- views: Overview, Cards and Over time, as tabs; the address keeps the one on screen (#cards, #time)

const VIEWS = ["overview", "cards", "time"];
let view = "overview";
function showView(v) {
  view = VIEWS.includes(v) ? v : "overview";
  for (const name of VIEWS) {
    $("v-" + name).hidden = name !== view;
    $("t-" + name).setAttribute("aria-selected", String(name === view));
  }
  hideTip();
  if (D) update(true);
}
document.querySelector(".views").addEventListener("click", (e) => {
  const b = e.target.closest("button[data-view]");
  if (b) location.hash = b.dataset.view === "overview" ? "" : b.dataset.view;
});
window.addEventListener("hashchange", () => showView(location.hash.slice(1)));
showView(location.hash.slice(1));

let current = null, pending = 0;
// update redraws what the filters change in the view on screen, at most every
// 30 ms while the slider moves; withTime also redraws the games over time,
// which its own slider doesn't change. Charts are drawn only while their view
// shows, since a hidden one has no size.
function update(withTime) {
  if (withTime && view === "time") timeline();
  if (!pending) pending = setTimeout(() => { pending = 0; draw(); }, 30);
}
function draw() {
  const gs = all.filter((g, i) => keep(g, i));
  const s = count(gs);
  current = s;
  filterBar();
  $("count").textContent = gs.length ? `${gs.length} of ${all.length} games, won ${pct(s.overall.rate)}` : `0 of ${all.length} games`;
  if (view === "overview") { forest(s); tempo(s); province(s); }
  if (view === "cards") { spectrum(s); table(s); }
  if (view === "time") kpis(s, gs);
}

let lastWidth = window.innerWidth, resizeTimer;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    if (window.innerWidth !== lastWidth && current) draw();
    lastWidth = window.innerWidth;
  }, 150);
});
// the charts take their colours from the stylesheet: draw them again when the theme changes
window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
  for (const id of Object.keys(charts)) { charts[id].dispose(); delete charts[id]; }
  if (D) update(true);
});

// ---- data

let players = [];
async function getJSON(url) {
  const r = await fetch(url);
  const b = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(b.error || `HTTP ${r.status}`);
  return b;
}
async function show(name) {
  const p = players.find((x) => x.name === name);
  D = await getJSON(STATIC ? p.file : "/api/insights?player=" + encodeURIComponent(name));
  all = D.games || [];
  F.lo = 0;
  F.hi = all.length - 1;
  F.seat = "any";
  F.opp = "any";
  F.picks = [];
  $("title").textContent = D.me;
  $("meta").textContent = `${all.length} saved games, ${D.first_date || "?"} to ${D.last_date || "?"}`;
  $("csv").href = "/api/features.csv?player=" + encodeURIComponent(name);
  if (charts.timeline) { charts.timeline.dispose(); delete charts.timeline; }
  update(true);
}
async function start() {
  const d = await getJSON(STATIC ? "data/players.json" : "/api/players");
  players = d.players || [];
  for (const p of players) {
    const o = el("option", "", `${p.name} (${p.games} games)`);
    o.value = p.name;
    $("player").append(o);
  }
  $("player").value = d.default || (players[0] && players[0].name) || "";
  $("player").closest("label").hidden = players.length < 2;
  $("player").addEventListener("change", () => show($("player").value).catch(fail));
  await show($("player").value);
}
function fail(e) {
  $("error").textContent = e.message;
  $("error").hidden = false;
}
start().catch(fail);
