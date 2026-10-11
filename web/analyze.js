// Analysis page: one saved game, turn by turn, from scry web's /api/games. A game
// that isn't saved yet comes from the "Scry game" bookmark, which loads it in the
// logged-in dominion.games tab and posts it to /api/games (or to this page, when
// this page opened that tab).
"use strict";

const $ = (id) => document.getElementById(id);

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

// Card-frame colours, as in the match notes' kingdom viewer.
const FRAME = {
  Night: "#45424f", Treasure: "#efcb55", Victory: "#9cc97a", Curse: "#b48ac8",
  Reaction: "#82acd9", Duration: "#f0a35c", Reserve: "#d8b87e", Ruins: "#a8865f",
  Shelter: "#d98b6d", Action: "#ece2cb",
  event: "#c9c6bd", project: "#eba7b3", landmark: "#8fb58f", way: "#a6cfe1",
  ally: "#e8c897", trait: "#c1aedb", prophecy: "#a8c3ea",
};
const FRAME_ORDER = ["Night", "Treasure", "Victory", "Curse", "Reaction", "Duration", "Reserve", "Ruins", "Shelter", "Action"];
const BASICS = ["Copper", "Silver", "Gold", "Platinum", "Potion", "Estate", "Duchy", "Province", "Colony", "Curse"];
const ZONES = ["hand", "in play", "deck", "discard", "set aside", "revealed", "Tavern mat", "Island mat", "Native Village mat", "Exile"];
const ZONE_NAMES = { hand: "Hand", "in play": "In play", deck: "Draw pile", discard: "Discard", "set aside": "Set aside", revealed: "Revealed" };

let game = null;
let at = 0;
let waiting = null;
const cursors = [];

function frames(card) {
  if (!card) return [FRAME.Action, FRAME.Action];
  if (FRAME[card.kind]) return [FRAME[card.kind], FRAME[card.kind]];
  const hits = FRAME_ORDER.filter((t) => card.types.includes(t)).map((t) => FRAME[t]);
  return [hits[0] || FRAME.Action, hits[1] || hits[0] || FRAME.Action];
}

function wikiURL(name) {
  return "https://wiki.dominionstrategy.com/index.php/" + encodeURIComponent(name.replace(/ /g, "_"));
}

function costText(card) {
  return [card.cost && card.cost !== "0" && "$" + card.cost, card.potion && "P", card.debt && card.debt + "D"].filter(Boolean).join(" ");
}

function paint(node, name) {
  const card = game.cards[name];
  const [f1, f2] = frames(card);
  node.style.setProperty("--f1", f1);
  node.style.setProperty("--f2", f2);
  if (card && card.types.includes("Night")) node.classList.add("dark");
  node.title = card ? [name, card.types.join(" – "), costText(card)].filter(Boolean).join(" · ") : name;
  node.href = wikiURL(name);
  node.target = "_blank";
  node.rel = "noopener";
}

function chip(name, text) {
  const a = el("a", "chip", text);
  paint(a, name);
  return a;
}

// "3× Copper" chips for a {card: count} map, most copies first.
function deckChips(deck) {
  return Object.entries(deck || {})
    .filter(([, n]) => n > 0)
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([name, n]) => chip(name, n > 1 ? `${n}× ${name}` : name));
}

// chips for a turn's [{card, n}] list, adding up repeats
function listChips(list) {
  const deck = {};
  for (const c of list || []) deck[c.card || "card"] = (deck[c.card || "card"] || 0) + c.n;
  return deckChips(deck);
}

const sum = (d) => Object.values(d || {}).reduce((a, b) => a + b, 0);
const color = (p) => `var(--p${Math.max(0, game.players.indexOf(p)) % 4})`;

function swatch(p) {
  const s = el("span", "swatch");
  s.style.background = color(p);
  return s;
}

async function api(path, opts) {
  const r = await fetch(path, opts);
  const body = await r.json().catch(() => ({}));
  if (!r.ok) {
    const e = new Error(body.error || `HTTP ${r.status}`);
    e.status = r.status;
    throw e;
  }
  return body;
}

function showError(msg) {
  $("error").textContent = msg;
  $("error").hidden = !msg;
}

// ---- choosing a game

async function loadSaved() {
  let games = [];
  try {
    games = await api("/api/games");
  } catch (e) {
    return;
  }
  $("saved-count").textContent = `(${games.length})`;
  const list = $("saved-list");
  list.replaceChildren();
  for (const g of games) {
    const li = el("li");
    const a = el("a");
    a.href = "#" + g.id;
    a.append(el("span", "num", g.id), el("span", "", g.row || (g.original || []).join(" vs ")));
    li.dataset.text = [g.id, g.row, ...(g.original || [])].join(" ").toLowerCase();
    li.append(a);
    list.append(li);
  }
  if (!location.hash && games.length) $("saved").open = true;
}

$("filter").addEventListener("input", () => {
  const q = $("filter").value.trim().toLowerCase();
  for (const li of $("saved-list").children) li.hidden = !!q && !li.dataset.text.includes(q);
});

$("find").addEventListener("submit", (e) => {
  e.preventDefault();
  const id = $("gid").value.replace(/\D/g, "");
  if (id) location.hash = id;
});

function route() {
  const m = /^#(\d+)(?:\/(\d+))?/.exec(location.hash);
  if (!m) return;
  const turn = m[2] ? +m[2] - 1 : null;
  if (game && game.id === m[1]) {
    if (turn !== null) select(turn);
    return;
  }
  show(m[1], turn);
}
window.addEventListener("hashchange", route);

async function show(id, turn) {
  stopWaiting();
  showError("");
  $("gid").value = id;
  try {
    game = await api("/api/games/" + id);
  } catch (e) {
    game = null;
    if (e.status === 404) return askForGame(id);
    $("game").hidden = true;
    return showError(e.message);
  }
  $("fetch").hidden = true;
  render(turn);
}

// ---- getting a game that isn't saved

function askForGame(id) {
  $("game").hidden = true;
  for (const s of document.querySelectorAll(".gid")) s.textContent = id;
  $("fetch").hidden = false;
  $("waiting").hidden = false;
  const started = Date.now();
  waiting = setInterval(async () => {
    if (Date.now() - started > 15 * 60 * 1000) {
      stopWaiting();
      return;
    }
    try {
      await api("/api/games/" + id);
      show(id, null);
      loadSaved();
    } catch (e) {
      // not there yet
    }
  }, 3000);
}

function stopWaiting() {
  clearInterval(waiting);
  waiting = null;
  $("waiting").hidden = true;
}

async function addGame(rec) {
  const r = await api("/api/games", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(rec) });
  loadSaved();
  if (location.hash === "#" + r.id) show(r.id, null);
  else location.hash = r.id;
}

api("/api/bookmarklet").then((b) => ($("bookmark").href = b.href), () => {});
$("bookmark").addEventListener("click", (e) => {
  e.preventDefault();
  showError("Drag Scry game to your bookmarks bar, then click it in your dominion.games tab.");
});
$("copy-id").addEventListener("click", () => navigator.clipboard.writeText($("gid").value.replace(/\D/g, "")));
$("open-dg").addEventListener("click", () => {
  window.open("https://dominion.games/#scry=" + $("gid").value.replace(/\D/g, ""), "_blank");
});
window.addEventListener("message", (e) => {
  if (e.origin !== "https://dominion.games" || !e.data || !e.data.scryGame) return;
  addGame(e.data.scryGame).catch((err) => showError(err.message));
});
$("paste-go").addEventListener("click", () => {
  const line = $("paste").value.split("\n").find((l) => l.trim());
  let rec;
  try {
    rec = JSON.parse(line);
  } catch (e) {
    return showError("That isn't a copied game: paste exactly what the Scry game bookmark copied.");
  }
  addGame(rec).then(() => ($("paste").value = ""), (err) => showError(err.message));
});

// ---- the game

function render(turn) {
  const v = game;
  $("game").hidden = false;
  $("title").textContent = v.players.join(" vs ");
  const header = (v.header || []).filter((h) => !/^Game #/.test(h));
  $("meta").textContent = [`Game #${v.id}`, v.row && v.row.split(" | ")[0], ...header, `${v.turns.length} turns`].filter(Boolean).join(" · ");

  const totals = Object.fromEntries(v.players.map((p) => [p, sum(v.final_vp[p])]));
  const best = Math.max(...Object.values(totals));
  $("scores").replaceChildren(
    ...v.players.map((p) => {
      const box = el("div", totals[p] === best ? "score won" : "score");
      const who = el("div", "who");
      who.append(swatch(p), el("span", "", p));
      if (p === v.account) who.append(el("span", "you", "you"));
      box.append(who, el("div", "vp", `${totals[p]} VP`));
      return box;
    }),
  );

  const probs = $("problems");
  probs.replaceChildren();
  if (v.problems && v.problems.length) {
    const p = el("p", "warning");
    p.append(
      el("strong", "", "The replay doesn't match the score screen. "),
      document.createTextNode("Usually the game's last decision is missing from a loaded game, so the final buy may not show. "),
      el("span", "hint", v.problems.join("; ")),
    );
    probs.append(p);
  }

  if (!v.turns.length) {
    showError("This game has no turns.");
    return;
  }
  drawCharts();
  $("turn").max = v.turns.length - 1;
  renderDecks();
  select(turn === null || turn === undefined ? v.turns.length - 1 : turn);
}

function select(i) {
  const v = game;
  at = Math.max(0, Math.min(v.turns.length - 1, i));
  $("turn").value = at;
  const t = v.turns[at];
  const label = $("turn-label");
  label.replaceChildren(swatch(t.player), el("span", "", `Turn ${t.number}${t.extra ? " (" + t.extra + ")" : ""} · ${t.player}`), el("span", "hint", `${at + 1}/${v.turns.length}`));
  for (const c of cursors) c.move(at);
  renderSummary(t);
  renderLog(t);
  renderZones(t);
  renderSupply();
  history.replaceState(null, "", `#${v.id}/${at + 1}`);
}

$("turn").addEventListener("input", () => select(+$("turn").value));
$("prev").addEventListener("click", () => select(at - 1));
$("next").addEventListener("click", () => select(at + 1));
document.addEventListener("keydown", (e) => {
  if (!game || $("game").hidden || (e.target instanceof Element && e.target.matches("input, textarea, select")) || e.metaKey || e.ctrlKey || e.altKey) return;
  const to = { ArrowLeft: at - 1, ArrowRight: at + 1, Home: 0, End: game.turns.length - 1 }[e.key];
  if (to === undefined) return;
  e.preventDefault();
  select(to);
});

function renderSummary(t) {
  const box = $("turn-sum");
  box.replaceChildren();
  const stat = (label, value) => {
    const s = el("span");
    s.append(el("b", "", String(value)), document.createTextNode(" " + label));
    box.append(s);
  };
  stat("$", t.coins);
  if (t.actions) stat(t.actions === 1 ? "Action played" : "Actions played", t.actions);
  stat("drawn", t.drawn);
  if (t.shuffles) stat(t.shuffles === 1 ? "shuffle" : "shuffles", t.shuffles);
  for (const [label, list] of [["bought", t.bought], ["gained", t.gained], ["trashed", t.trashed]]) {
    if (!list || !list.length) continue;
    const s = el("span");
    s.append(document.createTextNode(label + " "), ...listChips(list));
    box.append(s);
  }
}

// pieces of a log line, spaced like the client shows them
function lineNodes(parts) {
  const out = [];
  parts.forEach((p, k) => {
    const prev = parts[k - 1];
    if (k > 0 && !/^[.,;:)!?]/.test(p.text) && !/[($+]$/.test(prev.text)) out.push(document.createTextNode(" "));
    if (p.player) {
      const s = el("span", "who", p.player);
      s.style.color = color(p.player);
      out.push(s);
    } else if (p.cards && p.cards.length) {
      out.push(chip(p.cards[0], p.text));
    } else {
      out.push(document.createTextNode(p.text));
    }
  });
  return out;
}

function renderLog(t) {
  const box = $("turn-log");
  box.replaceChildren();
  const lines = at === 0 ? [...(game.setup || []).filter((l) => !/^Game #|^Timer:/.test(l.parts[0].text)), ...(t.log || [])] : t.log || [];
  for (const l of lines) {
    const d = el("div", "line");
    d.style.paddingLeft = l.level * 1.2 + "em";
    d.append(...lineNodes(l.parts));
    box.append(d);
  }
  box.scrollTop = 0;
}

function renderZones(t) {
  const box = $("zones");
  box.replaceChildren();
  for (const p of game.players) {
    const sec = el("div", "zones-player");
    const h = el("h3");
    h.append(swatch(p), el("span", "", p), el("span", "counters", `${t.vp[p] ?? 0} VP · ${sum(t.decks[p])} cards`));
    const c = t.counters[p] || {};
    const extra = [["Coffers", c.coffers], ["Villagers", c.villagers], ["Favors", c.favors], ["debt", c.debt], ["VP tokens", c.vp_tokens]]
      .filter(([, n]) => n)
      .map(([k, n]) => `${n} ${k}`);
    if (extra.length) h.append(el("span", "counters", extra.join(" · ")));
    sec.append(h);
    const zones = t.zones[p] || {};
    const names = [...ZONES.filter((z) => zones[z]), ...Object.keys(zones).filter((z) => !ZONES.includes(z)).sort()];
    for (const z of names) {
      const cards = zones[z];
      const row = el("div", "zone");
      const name = el("span", "name");
      name.append(document.createTextNode((ZONE_NAMES[z] || z) + " "), el("b", "", String(sum(cards.known) + (cards.unknown || 0))));
      const list = el("span", "cards");
      list.append(...deckChips(cards.known));
      if (cards.unknown) {
        const u = el("span", "chip unknown", `${cards.unknown} unnamed`);
        u.title = "The log doesn't name these cards";
        list.append(u);
      }
      row.append(name, list);
      sec.append(row);
    }
    if (t.unnamed && t.unnamed[p]) {
      const pool = el("p", "pool");
      pool.append(document.createTextNode("Unnamed cards are among: "), ...deckChips(t.unnamed[p]).flatMap((c) => [c, document.createTextNode(" ")]));
      sec.append(pool);
    }
    box.append(sec);
  }
}

function pileOrder(name) {
  const card = game.cards[name];
  const b = BASICS.indexOf(name);
  if (b >= 0) return [1, b, name];
  if (!card) return [0, 99, name];
  if (card.kind === "other") return [2, 0, name];
  return [0, parseInt(card.cost || "0", 10) + (card.potion ? 0.5 : 0) + (card.debt || 0) / 2, name];
}

function renderSupply() {
  const box = $("supply");
  box.replaceChildren();
  const t = game.turns[at];
  const prev = at > 0 ? game.turns[at - 1].supply : {};
  const piles = Object.keys(game.piles).sort((a, b) => {
    const x = pileOrder(a), y = pileOrder(b);
    return x[0] - y[0] || x[1] - y[1] || x[2].localeCompare(y[2]);
  });
  for (const name of piles) {
    const left = t.supply[name] ?? game.piles[name];
    const before = prev[name] ?? game.piles[name];
    const card = game.cards[name];
    const a = el("a", "pile");
    paint(a, name);
    if (left <= 0) a.classList.add("empty");
    if (card && card.art && card.kind !== "basic") { // the wiki has no art pages for basic cards
      const art = el("span", "art");
      const img = el("img");
      img.alt = "";
      img.loading = "lazy";
      img.src = "/art/" + encodeURIComponent(card.art);
      img.onerror = () => art.remove();
      art.append(img);
      a.append(art);
    }
    const count = el("span", "count");
    count.append(el("b", "", String(left)), document.createTextNode(` / ${game.piles[name]}`));
    if (left !== before) count.append(el("span", "delta", (left > before ? "+" : "−") + Math.abs(left - before)));
    a.append(el("span", "name", name), count);
    box.append(a);
  }
  $("supply-note").textContent = game.kingdom && game.kingdom.length ? "(after the turn)" : "(the piles this game touched, after the turn)";
  const lands = game.landscapes || [];
  $("lands").hidden = !lands.length;
  $("lands").replaceChildren(document.createTextNode("Also in the game: "), ...lands.flatMap((c) => [chip(c, c), document.createTextNode(" ")]));
}

function renderDecks() {
  const box = $("decks");
  box.replaceChildren();
  const last = game.turns[game.turns.length - 1];
  for (const p of game.players) {
    const sec = el("div", "deck");
    const h = el("h3");
    h.append(swatch(p), el("span", "", `${p} · ${sum(last.decks[p])} cards`));
    const cards = el("div", "cards");
    cards.append(...deckChips(last.decks[p]));
    const table = el("table");
    const vp = Object.entries(game.final_vp[p] || {}).sort((a, b) => b[1] - a[1]);
    for (const [k, n] of vp) {
      const tr = el("tr");
      tr.append(el("td", "", k), el("td", "", String(n)));
      table.append(tr);
    }
    const tr = el("tr", "total");
    tr.append(el("td", "", "Total"), el("td", "", `${sum(game.final_vp[p])} VP`));
    table.append(tr);
    sec.append(h, cards, table);
    box.append(sec);
  }
}

// ---- charts: inline SVG over the turn sequence; click or drag to pick a turn

const SVG = "http://www.w3.org/2000/svg";
function svgEl(tag, attrs) {
  const e = document.createElementNS(SVG, tag);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  return e;
}

function niceMax(v) {
  if (v <= 5) return 5;
  const step = v <= 20 ? 5 : v <= 60 ? 10 : 20;
  return Math.ceil(v / step) * step;
}

function chart(box, { lines, bars }) {
  const turns = game.turns;
  const W = 320, H = 150, L = 26, R = 6, T = 8, B = 18;
  const n = turns.length;
  const values = [...(lines || []).flatMap((s) => s.values), ...(bars || []).map((b) => b.value)];
  const lo = Math.min(0, ...values);
  const hi = niceMax(Math.max(1, ...values));
  const step = (W - L - R) / Math.max(1, n - 1);
  const x = (i) => L + (n === 1 ? (W - L - R) / 2 : i * step);
  const y = (v) => T + (H - T - B) * (1 - (v - lo) / (hi - lo || 1));
  const svg = svgEl("svg", { viewBox: `0 0 ${W} ${H}`, role: "img" });
  for (const v of [lo, (lo + hi) / 2, hi]) {
    svg.append(svgEl("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), class: "axis" }));
    const t = svgEl("text", { x: L - 4, y: y(v) + 3, "text-anchor": "end", class: "tick" });
    t.textContent = Math.round(v);
    svg.append(t);
  }
  turns.forEach((t, i) => {
    if (t.player === game.players[0] && !t.extra && t.number % 5 === 0) {
      const tx = svgEl("text", { x: x(i), y: H - 5, "text-anchor": "middle", class: "tick" });
      tx.textContent = "T" + t.number;
      svg.append(tx);
    }
  });
  for (const b of bars || []) {
    const w = Math.max(1.5, step * 0.7);
    svg.append(svgEl("rect", { x: x(b.i) - w / 2, y: y(Math.max(0, b.value)), width: w, height: Math.abs(y(b.value) - y(0)), fill: b.color, opacity: 0.85 }));
  }
  for (const s of lines || []) {
    svg.append(svgEl("polyline", { points: s.values.map((v, i) => `${x(i)},${y(v)}`).join(" "), fill: "none", stroke: s.color, "stroke-width": 2, "stroke-linejoin": "round", "stroke-linecap": "round" }));
  }
  const cursor = svgEl("line", { y1: T, y2: H - B, class: "cursor" });
  svg.append(cursor);
  cursors.push({ move: (i) => { cursor.setAttribute("x1", x(i)); cursor.setAttribute("x2", x(i)); } });
  svg.setAttribute("aria-label", box.parentElement.querySelector("figcaption").textContent);
  const pick = (e) => {
    const r = svg.getBoundingClientRect();
    const px = ((e.clientX - r.left) / r.width) * W;
    select(Math.round((px - L) / step));
  };
  svg.addEventListener("pointerdown", (e) => {
    pick(e);
    svg.setPointerCapture(e.pointerId);
  });
  svg.addEventListener("pointermove", (e) => {
    if (e.buttons) pick(e);
  });
  box.replaceChildren(svg);
}

function drawCharts() {
  cursors.length = 0;
  const ps = game.players;
  chart($("chart-vp"), { lines: ps.map((p) => ({ color: color(p), values: game.turns.map((t) => t.vp[p] ?? 0) })) });
  chart($("chart-coins"), { bars: game.turns.map((t, i) => ({ i, value: t.coins, color: color(t.player) })) });
  chart($("chart-deck"), { lines: ps.map((p) => ({ color: color(p), values: game.turns.map((t) => sum(t.decks[p])) })) });
}

loadSaved();
route();
