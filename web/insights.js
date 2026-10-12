// "My games": win rates of the saved games by kingdom role, seat, ending,
// opening and kingdom card, from scry web's /api/insights.
"use strict";

const $ = (id) => document.getElementById(id);
function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}
const pct = (x) => Math.round(x * 100) + "%";
const ROLE_NAMES = { village: "Village", draw: "Draw", plus_buy: "+Buy", trasher: "Trashing", curser: "Cursing", attack: "Attack" };
const ENDINGS = { provinces: "Provinces ran out", piles: "Three piles", resign: "Resignation (or unfinished log)" };

// One rate as a dot on a 0-100% track with its 95% range; base marks the overall rate.
function range(r, cls, base) {
  const box = el("div", "range " + cls);
  box.title = `${pct(r.rate)} of ${r.games} games (95% range ${pct(r.low)}-${pct(r.high)})`;
  if (base !== undefined) {
    const b = el("span", "base");
    b.style.left = base * 100 + "%";
    box.append(b);
  }
  if (r.games) {
    const band = el("span", "band");
    band.style.left = r.low * 100 + "%";
    band.style.width = Math.max(0.5, (r.high - r.low) * 100) + "%";
    const dot = el("span", "dot");
    dot.style.left = r.rate * 100 + "%";
    box.append(band, dot);
  }
  return box;
}

function num(r) {
  const n = el("span", "num");
  n.append(document.createTextNode(r.games ? pct(r.rate) + " " : "– "), el("small", "", `n=${r.games}`));
  return n;
}

// "with / without" rows for a split
function splitRows(box, s, labels, base) {
  for (const [key, cls, label] of [["with", "with", labels[0]], ["without", "without", labels[1]]]) {
    const row = el("div", "split");
    const what = el("span", "what", label);
    if (key === "with" && s.desc) what.append(el("small", "", s.desc));
    row.append(what, range(s[key], cls, base), num(s[key]));
    box.append(row);
  }
}

function simpleRow(box, label, r, base, cls = "with") {
  const row = el("div", "split");
  row.append(el("span", "what", label), range(r, cls, base), num(r));
  box.append(row);
}

function tempoChart(t) {
  const W = 640, H = 220, L = 34, R = 10, T = 10, B = 26;
  const values = [...t.wins, ...t.losses].filter((v) => v > 0);
  const max = Math.ceil(Math.max(8, ...values) / 2) * 2;
  const x = (i) => L + (i * (W - L - R)) / (t.turn.length - 1);
  const y = (v) => T + (H - T - B) * (1 - v / max);
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", `0 0 ${W} ${H}`);
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", "Average $ per turn in wins and losses");
  const add = (tag, attrs, text) => {
    const e = document.createElementNS(ns, tag);
    for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
    if (text !== undefined) e.textContent = text;
    svg.append(e);
    return e;
  };
  for (let v = 0; v <= max; v += max / 4) {
    add("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), stroke: "var(--rule)" });
    add("text", { x: L - 6, y: y(v) + 3, "text-anchor": "end", fill: "var(--muted)", "font-size": 10 }, "$" + v);
  }
  t.turn.forEach((n, i) => {
    if (n === 1 || n % 5 === 0) add("text", { x: x(i), y: H - 8, "text-anchor": "middle", fill: "var(--muted)", "font-size": 10 }, "T" + n);
  });
  for (const [vals, counts, color] of [[t.wins, t.n_wins, "var(--victory)"], [t.losses, t.n_losses, "var(--treasure)"]]) {
    const pts = vals.map((v, i) => (counts[i] >= 5 ? `${x(i)},${y(v)}` : null)).filter(Boolean);
    add("polyline", { points: pts.join(" "), fill: "none", stroke: color, "stroke-width": 2.5, "stroke-linejoin": "round" });
  }
  const legend = el("div", "legend-row");
  legend.innerHTML = '<span><i style="background:var(--victory)"></i>Wins</span><span><i style="background:var(--treasure)"></i>Losses</span><span>Turns with fewer than 5 games are left out</span>';
  $("tempo").replaceChildren(svg, legend);
}

function cardTable(table, rows) {
  table.replaceChildren();
  const head = el("tr");
  for (const h of ["Card", "Games", "Win", "Took", "Skipped"]) head.append(el("th", "", h));
  table.append(head);
  for (const c of rows) {
    const tr = el("tr");
    const name = el("td");
    const a = el("a", "", c.card);
    a.href = "https://wiki.dominionstrategy.com/index.php/" + encodeURIComponent(c.card.replace(/ /g, "_"));
    a.target = "_blank";
    a.rel = "noopener";
    name.append(a);
    const cell = (r) => el("td", "", r.games ? `${pct(r.rate)} (${r.games})` : "–");
    tr.append(name, el("td", "", String(c.present.games)), el("td", "", pct(c.present.rate)), cell(c.took), cell(c.skipped));
    table.append(tr);
  }
}

function render(d) {
  const base = d.overall.rate;
  $("title").textContent = `${d.me}'s games`;
  const dates = d.games.map((g) => g.date).filter(Boolean);
  $("meta").textContent = `${d.overall.games} games${dates.length ? ", " + dates[0] + " to " + dates[dates.length - 1] : ""}. ${d.complete} of them end exactly as the final scores say; in the others the log misses the last decision.`;
  const box = el("div", "score won");
  box.append(el("div", "who", "Won"), el("div", "vp", pct(base)));
  box.title = `95% range ${pct(d.overall.low)}-${pct(d.overall.high)}`;
  $("overall").replaceChildren(box);

  const roles = $("roles");
  roles.replaceChildren();
  for (const s of d.roles) {
    const pair = el("div", "pair");
    splitRows(pair, s, [`${ROLE_NAMES[s.name] || s.name} in the kingdom`, `no ${(ROLE_NAMES[s.name] || s.name).toLowerCase()}`], base);
    roles.append(pair);
  }

  const seat = $("seat");
  seat.replaceChildren();
  splitRows(seat, d.seat[0], ["You went first", "You went second"], base);
  const endings = $("endings");
  endings.replaceChildren(el("p", "hint", "How the game ended (at most one Province left, or one card from three empty piles):"));
  for (const [k, label] of Object.entries(ENDINGS)) if (d.endings[k]) simpleRow(endings, label, d.endings[k], base);

  const op = $("openings");
  op.replaceChildren();
  Object.entries(d.openings).sort((a, b) => b[1].games - a[1].games).forEach(([k, r]) => simpleRow(op, k, r, base));

  tempoChart(d.tempo);

  const cards = d.cards || [];
  $("cards-note").textContent = `Cards in at least ${d.min_games} of your kingdoms (${cards.length} cards), ordered by win rate pulled toward your overall ${pct(base)}, as if 10 average games were added. "Took" means the card was in your deck at some point.`;
  cardTable($("worst"), cards.slice(0, 12));
  cardTable($("best"), cards.slice(-12).reverse());
}

fetch("/api/insights")
  .then((r) => r.json().then((b) => (r.ok ? b : Promise.reject(new Error(b.error || r.status)))))
  .then(render)
  .catch((e) => {
    $("error").textContent = e.message;
    $("error").hidden = false;
  });
