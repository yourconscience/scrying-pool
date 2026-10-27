// Engine page: what the rules engine adds to the Profiles, from data/engine.json,
// which "scry sim probe" writes: Big Money with one card against plain Big Money
// in each kingdom of a player's latest saved games, next to what the player
// bought there, and how far the engine replays the saved games.
"use strict";

const $ = (id) => document.getElementById(id);
const STATIC = document.querySelector('meta[name="scry-data"]').content === "static";
if (STATIC) {
  // the hosted site has Profiles as its index; the app's other pages aren't there
  document.body.classList.add("static");
  document.querySelector(".top .brand").href = "./";
  document.querySelector('.top a[href="insights.html"]').href = "./";
}

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

function kpi(label, value, sub) {
  const k = el("div", "kpi");
  k.append(el("div", "label", label), el("div", "value", value), el("div", "sub", sub));
  return k;
}

const pct = (x) => `${Math.round(x * 100)}%`;
const key = (name) => name.toLowerCase().replace(/[^a-z0-9]/g, "");
const MONTHS = { Jan: 0, Feb: 1, Mar: 2, Apr: 3, May: 4, Jun: 5, Jul: 6, Aug: 7, Sep: 8, Oct: 9, Nov: 10, Dec: 11 };
function day(date) { // "2026-Oct-14" as "Oct 14"
  const [, m, d] = date.split("-");
  return MONTHS[m] === undefined ? date : `${m} ${Number(d)}`;
}
const result = (g) => (g.me > g.them ? "win" : g.me < g.them ? "loss" : "tie");
// how many of the card the player bought: the log names cards as the client does
function bought(g, card) {
  let n = 0;
  for (const [name, k] of Object.entries(g.bought || {})) if (key(name) === key(card)) n += k;
  return n;
}
const count = (games, r) => games.filter((g) => result(g) === r).length;
function tally(games) {
  const parts = [`${count(games, "win")} won`, `${count(games, "loss")} lost`];
  if (count(games, "tie")) parts.push(`${count(games, "tie")} tied`);
  return parts.join(", ");
}

function render(d) {
  const games = d.games.filter((g) => g.cards && g.cards.length);
  const cards = games.reduce((n, g) => n + g.cards.length, 0);
  $("meta").textContent = `${d.player}'s ${games.length} latest saved games, ${d.per.toLocaleString("en")} games per card, made ${d.made}.`;
  const r = d.replay;
  $("kpis").replaceChildren(
    kpi("Replayed turns", pct(r.matched / r.turns), `${r.matched.toLocaleString("en")} of ${r.turns.toLocaleString("en")} turns of ${r.logs} saved games match the log; ${r.done} games to the end`),
    kpi("Kingdoms played out", `${games.length}`, `${cards} cards, ${(cards * d.per).toLocaleString("en")} games of Big Money with one card`),
  );

  const took = games.filter((g) => bought(g, g.cards[0].card) > 0);
  const not = games.filter((g) => bought(g, g.cards[0].card) === 0);
  const strong = games.filter((g) => g.cards[0].rate >= 0.8)
    .map((g) => `${g.cards[0].card} (${pct(g.cards[0].rate)}, ${result(g)} ${g.me}:${g.them}, bought ${bought(g, g.cards[0].card)})`);
  const findings = [
    `In ${took.length} of ${games.length} games ${d.player} bought the card that helps Big Money most at least once: ${tally(took)}. In the other ${not.length}: ${tally(not)}. On so few games this doesn't predict the result; other plans win too.`,
  ];
  if (strong.length) findings.push(`A simple money plan is strongest with ${strong.join("; ")}.`);
  $("findings").replaceChildren(...findings.map((t) => el("li", "", t)));

  const table = $("probe");
  const head = el("thead");
  const hr = el("tr");
  for (const h of ["Date", "Opponent", "Result", "Best card for Big Money, then the next", "Bought it", "Opening", "Bought most"]) hr.append(el("th", "", h));
  head.append(hr);
  const body = el("tbody");
  for (const g of games) {
    const tr = el("tr");
    tr.append(el("td", "", day(g.date)), el("td", "", g.opponent));
    const res = el("td");
    res.append(el("span", `res ${result(g)}`, result(g)[0].toUpperCase()), document.createTextNode(` ${g.me}:${g.them}`));
    tr.append(res);
    const [a, b] = g.cards;
    const best = el("td");
    best.append(el("span", "best" + (a.rate >= 0.8 ? " strong" : ""), `${a.card} ${pct(a.rate)}`));
    if (b) best.append(el("br"), el("span", "sub", `${b.card} ${pct(b.rate)}`));
    tr.append(best);
    const n = bought(g, a.card);
    const t = el("td");
    t.append(n ? el("span", "took", `×${n}`) : el("span", "skip", "no"));
    tr.append(t);
    tr.append(el("td", "sub", (g.opening || []).join(" / ")));
    const kingdom = new Set(g.cards.map((c) => key(c.card)));
    const most = Object.entries(g.bought || {}).filter(([name]) => kingdom.has(key(name)))
      .sort((x, y) => y[1] - x[1]).slice(0, 3).map(([name, k]) => `${name} ×${k}`).join(", ");
    tr.append(el("td", "sub", most || "none"));
    body.append(tr);
  }
  table.replaceChildren(head, body);
}

fetch("data/engine.json")
  .then((r) => { if (!r.ok) throw new Error(r.status); return r.json(); })
  .then(render)
  .catch(() => {
    $("meta").textContent = "No engine numbers here yet.";
    const e = $("error");
    e.textContent = "The page reads data/engine.json, which \"scry sim probe -player NAME -o site-data/engine.json\" writes; the hosted site has it.";
    e.hidden = false;
  });
