// Recent games page: a player's latest saved games summed up the way DomBot's
// !results does (the score against what the rating levels predicted, the
// opponents), their wins against their losses, and each game. The Recent games
// bookmark (scry-recent.js), clicked in a dominion.games tab, opens this page,
// sends it the profile's latest games and loads the missing ones; this page
// saves what it gets through the app's own /api.
"use strict";

const $ = (id) => document.getElementById(id);
const DG = "https://dominion.games";
const player = new URLSearchParams(location.search).get("player") || "";
const pending = {}; // id -> {profile, status, error}: games the bookmark is still loading

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
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}
function showError(msg) {
  $("error").textContent = msg;
  $("error").hidden = !msg;
}
function live(msg) {
  $("live").hidden = !msg;
  $("live-text").textContent = msg || "";
}
const games_ = (n) => n + (n === 1 ? " game" : " games");
const one = (x) => (Math.round(x * 10) / 10).toFixed(1);
const score = (x) => (Number.isInteger(x) ? String(x) : x.toFixed(1));
const day = (d) => (d ? d.replace(/^\d{4}-/, "").replace("-", " ") : "?"); // "2026-Oct-11" -> "Oct 11"
const ENDINGS = { provinces: "Provinces", piles: "three piles", resign: "resigned" };
const RESULT = { 1: ["W", "win"], 0.5: ["T", "tie"], 0: ["L", "loss"] };

// ---- what the bookmark sends

const account = api("/api/players").then((p) => p.default, () => "");

window.addEventListener("message", async (e) => {
  if (e.origin !== DG || !e.data || !e.data.scry) return;
  const { scry: type, seq, data } = e.data;
  const reply = (d) => seq && e.source.postMessage({ scryReply: seq, ...d }, e.origin);
  try {
    if (type === "hello") {
      live("Connected to your dominion.games tab; it's reading your profile…");
      return reply({ account: await account });
    }
    if (type === "games") {
      await api("/api/profiles", data.games);
      const saved = new Set((await api("/api/games")).map((g) => g.id));
      const need = data.games.map((g) => g.id).filter((id) => !saved.has(id));
      for (const g of data.games) if (need.includes(g.id)) pending[g.id] = { profile: g.profile, status: "waiting" };
      live(need.length ? `Loading ${need.length} of the ${data.games.length} latest rated games…` : "");
      reply({ need });
      return refresh();
    }
    if (type === "game") {
      await api("/api/games", data);
      delete pending[data.id];
      reply({ ok: true });
      return refresh();
    }
    if (type === "progress") {
      const p = pending[data.id];
      if (p && data.status !== "ok") Object.assign(p, { status: data.status, error: data.error });
      const left = Object.values(pending).filter((x) => x.status === "waiting").length;
      live(left ? `Loading ${left} more…` : "");
      return refresh();
    }
  } catch (err) {
    reply({ error: err.message });
    showError(err.message);
  }
});

api("/api/bookmarklet?for=recent").then((b) => ($("bookmark").href = b.href), () => {});
$("bookmark").addEventListener("click", (e) => {
  e.preventDefault();
  showError("Drag Recent games to your bookmarks bar, then click it in your dominion.games tab.");
});

// ---- the page

async function refresh() {
  try {
    const r = await api("/api/recent" + (player ? "?player=" + encodeURIComponent(player) : ""));
    showError("");
    render(r);
  } catch (err) {
    showError(err.message);
  }
}

// A game the bookmark hasn't delivered yet, as far as the profile tells.
function pendingRow(id, p) {
  const me = p.profile.players.find((n) => n.toLowerCase() === (p.me || "").toLowerCase()) || Object.keys(p.profile.levels)[0];
  const opp = p.profile.players.find((n) => n !== me);
  return { id, pending: p, date: p.profile.row.split(" | ")[0], opponent: opp, my_level: p.profile.levels[me], opp_level: p.profile.levels[opp] };
}

function render(r) {
  $("title").textContent = `${r.me}'s recent games`;
  const n = r.games.length;
  const dates = r.games.map((g) => g.date).filter(Boolean);
  $("meta").textContent = n
    ? `${n} latest saved games, from ${day(dates[dates.length - 1])} to ${day(dates[0])}.`
    : "No saved games yet: load some with the bookmark below.";
  kpis(r);
  sides(r);
  const saved = new Set(r.games.map((g) => g.id));
  const extra = Object.entries(pending).filter(([id]) => !saved.has(id)).map(([id, p]) => pendingRow(id, { ...p, me: r.me }));
  games([...extra, ...r.games].sort((a, b) => (a.id.length - b.id.length) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0)).reverse());
}

function kpi(label, value, sub) {
  const k = el("div", "kpi");
  k.append(el("div", "label", label), el("div", "value", value), el("div", "sub", sub));
  return k;
}

function kpis(r) {
  const n = r.games.length, box = $("kpis");
  box.replaceChildren();
  if (!n) return;
  const expected = r.leveled === n
    ? `expected ${one(r.expected)} from the levels`
    : r.leveled ? `${score(r.level_score)} in the ${games_(r.leveled)} with levels, expected ${one(r.expected)}` : "no levels yet: the bookmark adds them";
  box.append(kpi("Score", `${score(r.score)}/${n}`, expected));
  box.append(kpi("Opponents", `${r.opponents} players`, r.leveled ? `levels ${one(r.opp_low)} to ${one(r.opp_high)}` + (r.leveled < n ? ` in ${games_(r.leveled)}` : "") : ""));
  const lv = r.games.filter((g) => g.my_level);
  if (lv.length) {
    const now = lv[0].my_level, then = lv[lv.length - 1].my_level, d = now - then;
    box.append(kpi("Your level", one(now), lv.length > 1 ? `${d >= 0 ? "+" : "−"}${one(Math.abs(d))} since ${day(lv[lv.length - 1].date)}` : "in the latest game"));
  }
}

function sides(r) {
  const t = $("sides"), w = r.wins, l = r.losses;
  t.replaceChildren();
  const head = el("tr");
  head.append(el("th", "", ""), el("th", "", `Wins (${w.games})`), el("th", "", `Losses (${l.games})`));
  t.append(el("thead"));
  t.tHead.append(head);
  const body = el("tbody");
  const row = (label, a, b) => {
    const tr = el("tr");
    const th = el("th", "", label);
    th.scope = "row";
    tr.append(th, el("td", "", a), el("td", "", b));
    body.append(tr);
  };
  const turn = (s) => (s.first_province ? "turn " + one(s.first_province) : "–");
  row("First Province", turn(w), turn(l));
  row("$ per turn, turns 1–10", w.games ? one(w.coins) : "–", l.games ? one(l.coins) : "–");
  row("Length", w.games ? one(w.turns) + " turns" : "–", l.games ? one(l.turns) + " turns" : "–");
  row("No Province bought", `${w.no_province} of ${w.games}`, `${l.no_province} of ${l.games}`);
  t.append(body);
}

function games(rows) {
  const t = $("games");
  t.replaceChildren();
  const head = el("tr");
  for (const h of ["", "Opponent", "Score", "Win chance", "Seat", "Turns", "Ending", "Opening", "1st Province"]) head.append(el("th", "", h));
  t.append(el("thead"));
  t.tHead.append(head);
  const body = el("tbody");
  let last = null;
  for (const g of rows) {
    if (g.date !== last) {
      last = g.date;
      const same = rows.filter((x) => x.date === g.date && !x.pending);
      const sum = same.reduce((s, x) => s + x.result, 0), exp = same.reduce((s, x) => s + (x.chance || 0), 0);
      const tr = el("tr", "day");
      const td = el("td", "", day(g.date) + (same.length ? ` · ${score(sum)}/${same.length}` + (same.every((x) => x.chance) ? `, expected ${one(exp)}` : "") : ""));
      td.colSpan = 9;
      tr.append(td);
      body.append(tr);
    }
    body.append(g.pending ? pendingTr(g) : gameTr(g));
  }
  t.append(body);
}

function opponentTd(g) {
  const td = el("td", "opp", g.opponent || "?");
  if (g.opp_level) td.append(el("span", "lvl", " " + one(g.opp_level)));
  return td;
}

function gameTr(g) {
  const tr = el("tr");
  const [mark, word] = RESULT[g.result];
  const res = el("td");
  const a = el("a", "res " + word, mark);
  a.href = "analyze.html#" + g.id;
  a.title = `Game ${g.id}: open it on the analysis page`;
  res.append(a);
  const vp = g.final || [g.my_vp, g.opp_vp];
  tr.append(res, opponentTd(g), el("td", "", `${vp[0]}–${vp[1]}`), el("td", "", g.chance ? Math.round(g.chance * 100) + "%" : "–"),
    el("td", "", g.first ? "1st" : "2nd"), el("td", "", String(g.turns)), el("td", "", ENDINGS[g.ending] || g.ending),
    el("td", "", g.opening || "–"), el("td", "", g.first_province ? "turn " + g.first_province : "–"));
  return tr;
}

function pendingTr(g) {
  const tr = el("tr", "pending");
  const what = { waiting: "loading…", short: "AFK game, not saved", server: "the server can't load it", error: "couldn't load it: " + (g.pending.error || "") }[g.pending.status] || g.pending.status;
  const note = el("td", "", what);
  note.colSpan = 7;
  tr.append(el("td", "res", "·"), opponentTd(g), note);
  return tr;
}

refresh();
