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
      out.append(el("p", "", l.slice(2)));
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
let allGames = false; // Battles also lists the games without a saved log

function setView(v) {
  $("view-report").hidden = v !== "report";
  $("view-battles").hidden = v !== "battles";
  $("tab-report").setAttribute("aria-pressed", String(v === "report"));
  $("tab-battles").setAttribute("aria-pressed", String(v === "battles"));
}

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
  const logged = rows.filter((g) => g.log);
  $("battles-note").textContent = `${rows.length} of ${battles.games.length} listed games${q ? ` against "${$("opp").value.trim()}"` : ""}; ${logged.length} with a saved log. Newest first.`;
  // the games without a log wait behind the toggle, unless they are all there is
  const bare = rows.length - logged.length;
  const games = bare === 1 ? "game" : "games";
  const all = allGames || !logged.length;
  $("unlogged").hidden = !bare || !logged.length;
  $("unlogged").textContent = allGames ? `Hide the ${bare} ${games} without a log` : `Show the ${bare} ${games} without a log`;
  box.replaceChildren(...(all ? rows : logged).map((g) => battle(g, battles.player, battles.images)));
  const url = new URL(location.href);
  if (q) url.searchParams.set("vs", $("opp").value.trim());
  else url.searchParams.delete("vs");
  history.replaceState(null, "", url);
}

function setBattles(b) {
  battles = b;
  $("tab-battles").hidden = !b;
  if (!b) return setView("report");
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

// ---- Analysis: the matchup, from the report's data (scoutReport as JSON)

const THIN = 5; // a rate over fewer games is drawn faded
const MIN_SIDE = 8; // a finding needs this many games on each side, or it says "few games"
const part = (r) => `${Math.round(r.wins * 2) / 2} of ${r.games}`;
const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);
const pc = (x) => Math.round(100 * x) + "%";

// The charts take their look from the stylesheet, as on the Profiles page.
const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const charts = {};
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

// Paired bars, one row per thing: the scouted player's win rate over yours, each with its 80% range, and
// its count as text ("64%  7/11") at the right so a phone needs no hover. Rows under THIN games are faded.
// rows: [{ label, sub, a, b }], a and b are Rates ({games, wins, rate, low, high}) or missing.
const WIDE = { BAR: 7, GAP: 4, ROW: 29, TEXT: 104, FONT: 12 }, PHONE = { BAR: 9, GAP: 5, ROW: 44, TEXT: 116, FONT: 13.5 }; // TEXT: room for the count at the right
function pairChart(id, rows, names) {
  const ch = chart(id);
  if (!ch) return;
  const { BAR, GAP, ROW, TEXT, FONT } = window.innerWidth < 640 ? PHONE : WIDE;
  $(id).style.height = rows.length * ROW + 22 + "px";
  ch.resize();
  const c = { ink: css("--ink"), ink2: css("--ink-2"), muted: css("--muted"), rule: css("--rule"), axis: css("--axis"), them: css("--them"), you: css("--you"), tipBg: css("--tip-bg"), tipInk: css("--tip-ink"), num: css("--num"), sans: css("--sans") };
  const thin = (x) => x.games < THIN;
  const key = (k) => (k ? "b" : "a");
  const side = (k, color, off) => ({
    name: names[k], type: "custom", color, z: 2,
    data: rows.map((r, i) => [(r[key(k)] || { rate: 0 }).rate, i]),
    renderItem: (_, api) => {
      const x = rows[api.value(1)][key(k)];
      if (!x || !x.games) return null;
      const [x0, y] = api.coord([0, api.value(1)]), x1 = api.coord([x.rate, 0])[0], lo = api.coord([x.low, 0])[0], hi = api.coord([x.high, 0])[0];
      const cy = y + off, op = thin(x) ? 0.4 : 1, end = api.coord([1, 0])[0] + 10;
      return {
        type: "group", children: [
          { type: "rect", shape: { x: x0, y: cy - BAR / 2, width: Math.max(x1 - x0, 1), height: BAR, r: [0, 3, 3, 0] }, style: { fill: color, opacity: op } },
          { type: "line", shape: { x1: lo, y1: cy, x2: hi, y2: cy }, style: { stroke: c.ink, lineWidth: 1.5, opacity: op } },
          { type: "line", shape: { x1: lo, y1: cy - 4, x2: lo, y2: cy + 4 }, style: { stroke: c.ink, lineWidth: 1.5, opacity: op } },
          { type: "line", shape: { x1: hi, y1: cy - 4, x2: hi, y2: cy + 4 }, style: { stroke: c.ink, lineWidth: 1.5, opacity: op } },
          { type: "text", style: { x: end, y: cy, text: `${pc(x.rate)}  ${Math.round(x.wins)}/${x.games}`, fill: c.ink2, font: `500 ${FONT}px ${c.num}`, textVerticalAlign: "middle", opacity: op } },
        ],
      };
    },
    tooltip: {
      formatter: (p) => {
        const x = rows[p.value[1]][key(k)];
        if (!x || !x.games) return "";
        return `<b>${names[k]}: ${rows[p.value[1]].label}</b><br>won ${part(x)} games (${pc(x.rate)}), 80% range ${pc(x.low)} to ${pc(x.high)}${thin(x) ? "<br>few games" : ""}`;
      },
    },
  });
  const sh = (BAR + GAP) / 2;
  ch.setOption({
    animationDuration: 300,
    textStyle: { fontFamily: c.num, color: c.muted, fontSize: 13 },
    tooltip: { trigger: "item", backgroundColor: c.tipBg, borderWidth: 0, padding: [8, 10], textStyle: { color: c.tipInk, fontFamily: c.sans, fontSize: 14.5 }, extraCssText: "border-radius:10px;box-shadow:none;", confine: true },
    grid: { left: 4, right: TEXT, top: 2, bottom: 20, containLabel: true },
    xAxis: { type: "value", min: 0, max: 1, interval: 0.5, axisLine: { show: false }, axisTick: { show: false }, axisLabel: { formatter: pc, color: c.muted, fontSize: 13 }, splitLine: { lineStyle: { color: c.rule } } },
    yAxis: {
      type: "category", inverse: true, data: rows.map((r) => r.label), axisLine: { show: false }, axisTick: { show: false },
      axisLabel: {
        color: c.ink, fontFamily: c.sans, fontSize: 14, lineHeight: 15, interval: 0, hideOverlap: false,
        formatter: (v, i) => (rows[i] && rows[i].sub ? `${v}\n{s|${rows[i].sub}}` : v),
        rich: { s: { color: c.muted, fontSize: 12.5, fontFamily: c.sans, lineHeight: 14 } },
      },
    },
    series: [side(0, c.them, -sh), side(1, c.you, sh)],
  }, true);
}

function chartPanel(title, id, note) {
  const p = el("section", "panel ch");
  const box = el("div", "ec");
  box.id = id;
  p.append(el("h2", "", title), box);
  if (note) p.append(el("p", "note", note));
  return p;
}

const SHAPES = [
  ["two kingdom cards", "Two kingdom cards"], ["a kingdom card and Silver", "Card and Silver"], ["a kingdom card, no Silver", "Card, no Silver"],
  ["Silver and Silver", "Silver and Silver"], ["one Silver, no kingdom card", "One Silver only"], ["no kingdom card, no Silver", "Neither"],
];
const STYLES = { engine: "Engine", "draw, no village": "Draw, no village", "no village or draw": "No village or draw", trashing: "Trashing", attacking: "Attacking", cursing: "Cursing", "+Buy": "+Buy" };
const END = [["provinces", "Ends on Provinces"], ["piles", "Ends on three piles"], ["resign", "Resignation"]];
const LENGTH = { "up to 14 turns": "Up to 14 turns", "15-19 turns": "15-19 turns", "20 turns or more": "20 turns or more" };

// the strongest differences in how the scouted player does: splits with at least
// 4 games on a side and 20 points between them, one per theme, those with
// MIN_SIDE games on both sides first
function findings(t) {
  const out = [];
  const add = (label, other, s) => {
    const few = Math.min(s.with.games, s.without.games);
    if (few < 4 || Math.abs(s.diff) < 0.2) return;
    const theme = /trash/.test(s.name) ? "trash" : /curs/.test(s.name) ? "curse" : /attack/.test(s.name) ? "attack" : /village/.test(s.name) ? "village" : /draw/.test(s.name) ? "draw" : /buy/i.test(s.name) ? "buy" : s.name;
    out.push({ label, other, s, few: few < MIN_SIDE, theme });
  };
  add("As first player", "as second player", t.seat);
  add("Against a higher-rated player", "against a lower-rated one", t.stronger);
  for (const s of t.opened_as) add(cap(s.name), "otherwise", s);
  for (const s of t.styles) add(cap(s.desc.replace(/ \(.*\)$/, "")), "otherwise", s);
  for (const s of t.kingdoms) add(cap(s.name), "without", s);
  out.sort((a, b) => a.few - b.few || Math.abs(b.s.diff) - Math.abs(a.s.diff));
  const seen = new Set();
  return out.filter((f) => !seen.has(f.theme) && seen.add(f.theme)).slice(0, 4);
}

function chip(c, text, images) {
  const x = el("span", "cc");
  if (images[c.card]) {
    const im = el("img");
    im.src = images[c.card];
    im.alt = "";
    im.loading = "lazy";
    x.append(im);
  }
  x.append(el("b", "", c.card), el("span", "", text));
  if (c.present < 3) x.classList.add("thin");
  return x;
}

function setDash(r, images) {
  const box = $("dash");
  for (const id of Object.keys(charts)) { charts[id].dispose(); delete charts[id]; }
  box.replaceChildren();
  $("legend").replaceChildren();
  $("full").hidden = !r;
  if (!r) return;
  const t = r.them, me = r.me, d = r.dominion_games, names = [t.name, me.name];
  const rank = (n) => (n ? "#" + n : "unranked");
  const s = d && d.summary, ms = d && d.my_summary, h = d && d.headToHead;

  // matchup strip
  const strip = el("div", "d-strip");
  const tile = (label, value, ...sub) => {
    const k = el("div", "panel kpi");
    k.append(el("div", "l", label), el("div", "v", value), ...sub.map((x) => (typeof x === "string" ? el("div", "s", x) : x)));
    return k;
  };
  const lvl = (a, b) => (a && b ? `${a.toFixed(1)} vs ${b.toFixed(1)}` : `${(t.level || 0).toFixed(1)} vs ${(me.level || 0).toFixed(1)}`);
  strip.append(tile("Level", s ? lvl(s.currentLevel, ms && ms.currentLevel) : lvl(), s ? `${t.name} ${rank(s.currentRank)} · you ${ms ? rank(ms.currentRank) : "–"}` : `${t.name} · you`));
  if (h && h.expected) {
    const a = Math.round(100 * h.expected), sp = el("div", "split");
    const l = el("i", "them", String(a)), rr = el("i", "you", String(100 - a));
    l.style.width = a + "%";
    rr.style.width = 100 - a + "%";
    sp.append(l, rr);
    strip.append(tile(`Expected score, ${t.name} / you`, `${a} / ${100 - a}`, sp));
    strip.lastChild.title = "from both ratings";
  } else {
    strip.append(tile("Expected score", "–", "needs a scout fetch"));
  }
  strip.append(tile("Head to head", h ? `${h.wins}–${h.losses}${h.ties ? `–${h.ties}` : ""}` : "–", h ? `${t.name} against you on dominion.games` : "needs a scout fetch"));
  const form = el("div", "panel kpi form");
  const fline = (cls, x, n, what) => { const p = el("div", "fl"); p.append(el("i", cls), el("b", "", x.games ? pc(x.rate) : "–"), ` ${what}, ${n} games`); return p; };
  form.append(el("div", "l", "Recent form, win rate"), fline("them", t.record, t.games, t.name + " (scouted)"), fline("you", me.record, me.games, "you (90 days)"));
  strip.append(form);
  box.append(strip);

  // key differences
  const fs = findings(t);
  const key = el("section", "panel ch keys");
  const kh = el("div", "head");
  kh.append(el("h2", "", `Where ${t.name} differs most`));
  if (fs.length) kh.append(el("span", "hint", `Points: the gap in their win rate with and without it. Under ${MIN_SIDE} games on a side is marked.`));
  key.append(kh);
  if (!fs.length) key.append(el("p", "note", `Too few games for a clear difference yet: a finding needs 4 games on each side and 20 points between them (${t.games} games scouted).`));
  const ul = el("ul");
  for (const f of fs) {
    const li = el("li");
    const pts = Math.round(100 * f.s.diff);
    li.append(el("span", "pts " + (pts > 0 ? "up" : "down"), (pts > 0 ? "+" : "−") + Math.abs(pts)), el("b", "", f.label), `: won ${part(f.s.with)}, ${f.other} ${part(f.s.without)}.`);
    if (f.few) li.append(" ", el("span", "few", "few games"));
    ul.append(li);
  }
  key.append(ul);
  box.append(key);

  // openings
  const shapes = (x) => Object.fromEntries((x.shapes || []).map((r) => [r.name, r.rate]));
  const ts = shapes(t), ms2 = shapes(me);
  const opened = t.opened.filter((o) => o.rate.games >= 2).slice(0, 2).map((o) => `${o.name} ${o.rate.games}`);
  const open = chartPanel("How they open", "c-open");
  if (opened.length) open.querySelector("h2").after(el("span", "hint", `most: ${opened.join(", ")}`));
  const openRows = SHAPES.filter(([k]) => (ts[k] && ts[k].games) || (ms2[k] && ms2[k].games)).map(([k, l]) => ({ label: l, a: ts[k], b: ms2[k] }));
  const mid = el("div", "d-mid");
  mid.append(open);

  // what they build
  const mst = Object.fromEntries(me.styles.map((x) => [x.name, x]));
  const buildRows = t.styles.map((x) => ({ label: STYLES[x.name] || x.name, sub: `${x.with.games} of ${t.games} games`, a: x.with, b: mst[x.name] && mst[x.name].with }));
  mid.append(chartPanel("What they build", "c-build"));

  // endings and length
  const ml = Object.fromEntries(me.length.map((x) => [x.name, x.rate]));
  const endRows = [...END.map(([k, l]) => ({ label: l, a: t.endings[k], b: me.endings[k] })), ...t.length.map((x) => ({ label: LENGTH[x.name] || x.name, a: x.rate, b: ml[x.name] }))];
  mid.append(chartPanel("How games end and run", "c-end"));
  box.append(mid);
  pairChart("c-open", openRows, names);
  pairChart("c-build", buildRows, names);
  pairChart("c-end", endRows, names);

  // cards
  const cards = el("section", "panel ch cards2");
  const ch2 = el("div", "head");
  ch2.append(el("h2", "", "Kingdom cards"), el("span", "hint", "taken or left alone / games in the kingdom"));
  cards.append(ch2);
  const took = t.took.slice(0, 7), left = t.skipped.slice(0, 6);
  const line = (title, cs, text) => {
    const p = el("div", "cl");
    p.append(el("span", "ct", title), ...cs.map((c) => chip(c, text(c), images)));
    if (!cs.length) p.append(el("span", "note", "none in 2 or more kingdoms"));
    return p;
  };
  cards.append(line("Takes most", took, (c) => `${c.took.games}/${c.present}`), line("Leaves alone", left, (c) => `${c.skipped.games}/${c.present}`));
  box.append(cards);
  const lg = $("legend");
  lg.replaceChildren(el("i", "them"), ` ${t.name}  `, el("i", "you"), ` ${me.name} (you, last 90 days) · bar: win rate · line: 80% range · numbers: games won/played · faded: under ${THIN} games`);
}

async function showReport(player) {
  shown = player;
  const seq = ++reportSeq;
  $("report-box").hidden = false;
  $("tabs").hidden = false;
  $("report-h").textContent = player;
  $("report-note").textContent = "Counting their games against yours…";
  $("report").replaceChildren();
  $("dash").replaceChildren();
  $("full").hidden = true;
  $("tab-battles").hidden = true;
  battles = null;
  allGames = false;
  $("opp").value = vs;
  setView(vs ? "battles" : "report"); // a link with a filter opens on Battles
  vs = ""; // the link's filter is for the first report only
  history.replaceState(null, "", "?player=" + encodeURIComponent(player));
  for (const b of $("list").querySelectorAll("button")) b.setAttribute("aria-pressed", String(b.dataset.player === player));
  try {
    const d = await api("/api/scout?player=" + encodeURIComponent(player) + "&n=" + encodeURIComponent($("n").value));
    if (seq !== reportSeq) return;
    $("report-note").textContent = "";
    render(d.markdown);
    setBattles(d.battles);
    setDash(d.report, d.battles ? d.battles.images : {});
  } catch (e) {
    if (seq === reportSeq) $("report-note").textContent = e.message;
  }
}

function showList(players) {
  const list = $("list");
  list.replaceChildren();
  $("list-note").hidden = players.length > 0;
  $("list-note").textContent = players.length
    ? "Each fetch is kept in the games folder; a report is made again from the saved games."
    : "No one yet: scout a player.";
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

$("tab-report").addEventListener("click", () => setView("report"));
$("tab-battles").addEventListener("click", () => setView("battles"));
$("unlogged").addEventListener("click", () => {
  allGames = !allGames;
  showBattles();
});
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
    else if (d.players.length) $("load-box").open = false; // the analysis comes first once someone is scouted
    const p = params.get("player") || d.players[0]?.player;
    if (p) showReport(p);
  } catch (e) {
    showError(e.message);
  }
})();
