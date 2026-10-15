// Recent games page: a player's latest games summed up the way DomBot's !results
// does (the score against what the rating levels predicted, the opponents), their
// wins against their losses, and each game.
//
// In the local app it lists the saved games (/api/recent). The Recent games
// bookmark (scry-recent.js), clicked in a dominion.games tab, sends it the
// profile's latest games and loads the missing ones, which this page saves
// through the app's /api.
//
// The live site's copy (from `scry site`) shows the published data/recent.json.
// Its Update button opens dominion.games, where the same bookmark sends the
// profile's latest games back; this browser keeps them, and Publish writes them
// to the repository's site-data/recent.json for every device.
"use strict";

const $ = (id) => document.getElementById(id);
const STATIC = document.querySelector('meta[name="scry-data"]').content === "static";
if (STATIC) {
  // the hosted site has Profiles as its index; the app's other pages aren't there
  document.body.classList.add("static");
  document.querySelector(".top .brand").href = "./";
  document.querySelector('.top a[href="insights.html"]').href = "./";
}
const DG = "https://dominion.games";
const LATEST = 25; // a dominion.games profile lists 25 games
const KEPT = "scry-recent-profile"; // localStorage: the latest profile list this browser got
const TOKEN = "scry-github-token"; // localStorage: the token Publish uses
const player = new URLSearchParams(location.search).get("player") || "";
const pending = {}; // local app: id -> {profile, status, error}, games the bookmark is still loading
let data = null; // what was loaded: {me, games}
let fromProfile = null; // live site: {me, at, games: [{id, profile}]} from the bookmark
let shown = { me: "", games: [] }; // the games on the page, newest first

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
function live(msg) {
  const box = STATIC ? "live-update" : "live";
  $(box).hidden = !msg;
  $(box + "-text").textContent = msg || "";
}
const store = {
  get(k) { try { return localStorage.getItem(k); } catch (e) { return null; } },
  set(k, v) { try { localStorage.setItem(k, v); } catch (e) { /* private window */ } },
  del(k) { try { localStorage.removeItem(k); } catch (e) { /* private window */ } },
};
const games_ = (n) => n + (n === 1 ? " game" : " games");
const one = (x) => (Math.round(x * 10) / 10).toFixed(1);
const score = (x) => (Number.isInteger(x) ? String(x) : x.toFixed(1));
const day = (d) => (d ? d.replace(/^\d{4}-/, "").replace("-", " ") : "?"); // "2026-Oct-11" -> "Oct 11"
const newestFirst = (a, b) => (b.id.length - a.id.length) || (a.id < b.id ? 1 : a.id > b.id ? -1 : 0);
const ENDINGS = { provinces: "Provinces", piles: "three piles", resign: "resigned" };
const RESULT = { 1: ["W", "win"], 0.5: ["T", "tie"], 0: ["L", "loss"] };

// The win chance two rating levels give, as advisor.WinChance computes it.
const winChance = (mine, theirs) => 1 / (1 + Math.exp(-(mine - theirs) / 7.5));

// A game as a profile lists it, for games without a saved log. Rows like this
// have no turns, so the wins-and-losses table leaves them out.
function profileRow(id, p, me) {
  const names = Array.isArray(p.players) ? p.players : [];
  const mine = names.find((n) => n.toLowerCase() === (me || "").toLowerCase()) || names[0] || "?";
  const opp = names.find((n) => n !== mine) || "?";
  const scores = p.scores || {}, levels = p.levels || {};
  const a = scores[mine], b = scores[opp];
  const row = { id, date: String(p.row || "").split(" | ")[0], played: p.time, me: mine, opponent: opp, my_level: levels[mine], opp_level: levels[opp], profile_only: true };
  if (typeof a === "number" && typeof b === "number") Object.assign(row, { result: a > b ? 1 : a === b ? 0.5 : 0, final: [a, b] });
  if (row.my_level && row.opp_level) row.chance = winChance(row.my_level, row.opp_level);
  return row;
}

// What DomBot's !results shows for these games, and wins against losses (over
// the games with a saved log).
function summarize(games) {
  const s = { games: 0, score: 0, leveled: 0, level_score: 0, expected: 0, opp_low: 0, opp_high: 0 };
  const opps = new Set();
  for (const g of games) {
    if (g.result === undefined) continue;
    s.games++;
    s.score += g.result;
    opps.add(g.opponent);
    if (g.chance) {
      if (!s.leveled || g.opp_level < s.opp_low) s.opp_low = g.opp_level;
      if (!s.leveled || g.opp_level > s.opp_high) s.opp_high = g.opp_level;
      s.leveled++;
      s.level_score += g.result;
      s.expected += g.chance;
    }
  }
  s.opponents = opps.size;
  const logged = games.filter((g) => g.turns > 0);
  s.wins = side(logged.filter((g) => g.result === 1));
  s.losses = side(logged.filter((g) => g.result === 0));
  return s;
}
function side(gs) {
  const s = { games: gs.length, turns: 0, coins: 0, first_province: 0, no_province: 0 };
  let coins = 0, nCoins = 0, provinces = 0, nProvinces = 0;
  for (const g of gs) {
    s.turns += g.turns / gs.length;
    for (const c of (g.coins || []).slice(0, 10)) { coins += c; nCoins++; }
    if (g.first_province > 0) { provinces += g.first_province; nProvinces++; } else s.no_province++;
  }
  if (nCoins) s.coins = coins / nCoins;
  if (nProvinces) s.first_province = provinces / nProvinces;
  return s;
}

// ---- what the bookmark sends

const account = STATIC ? Promise.resolve("") : api("/api/players").then((p) => p.default, () => "");

window.addEventListener("message", async (e) => {
  if (e.origin !== DG || !e.data || !e.data.scry) return;
  const { scry: type, seq } = e.data, msg = e.data.data;
  const reply = (d) => seq && e.source.postMessage({ scryReply: seq, ...d }, e.origin);
  try {
    if (type === "hello") {
      live("Connected to your dominion.games tab; it's reading your profile…");
      return reply({ account: STATIC ? (data && data.me) || "" : await account });
    }
    if (STATIC) {
      if (type !== "games") return;
      fromProfile = { me: msg.me, at: new Date().toISOString(), games: msg.games };
      store.set(KEPT, JSON.stringify(fromProfile));
      live("");
      reply({ need: [] }); // this page can't keep logs
      return render();
    }
    if (type === "games") {
      await api("/api/profiles", msg.games);
      const saved = new Set((await api("/api/games")).map((g) => g.id));
      const need = msg.games.map((g) => g.id).filter((id) => !saved.has(id));
      for (const g of msg.games) if (need.includes(g.id)) pending[g.id] = { profile: g.profile, status: "waiting" };
      live(need.length ? `Loading ${need.length} of the ${msg.games.length} latest rated games…` : "");
      reply({ need });
      return refresh();
    }
    if (type === "game") {
      await api("/api/games", msg);
      delete pending[msg.id];
      reply({ ok: true });
      return refresh();
    }
    if (type === "progress") {
      const p = pending[msg.id];
      if (p && msg.status !== "ok") Object.assign(p, { status: msg.status, error: msg.error });
      const left = Object.values(pending).filter((x) => x.status === "waiting").length;
      live(left ? `Loading ${left} more…` : "");
      return refresh();
    }
  } catch (err) {
    reply({ error: err.message });
    showError(err.message);
  }
});

// ---- the bookmarks, Update and Publish

const noClick = (id) => $(id).addEventListener("click", (e) => {
  e.preventDefault();
  showError("Drag Recent games to your bookmarks bar, then click it in your dominion.games tab.");
});
if (STATIC) {
  // the published script reports to whatever address this page has
  const here = location.href.replace(/\/[^/]*([?#].*)?$/, "");
  api("bookmark.json").then((b) => ($("bookmark-live").href = "javascript:" + encodeURIComponent(b.code.replaceAll("SCRY_ORIGIN", here))), () => {});
  noClick("bookmark-live");
  $("update").addEventListener("click", () => {
    if (!window.open(DG + "/#scry-recent", "scry-dg")) return showError("The browser blocked the new tab: allow pop-ups for this site.");
    live("Waiting for the Recent games bookmark in the dominion.games tab…");
  });
  try { fromProfile = JSON.parse(store.get(KEPT) || "null"); } catch (e) { fromProfile = null; }
  $("token").value = store.get(TOKEN) || "";
  $("publish").addEventListener("click", () => publish().catch((err) => ($("publish-note").textContent = "Not published: " + err.message)));
  $("forget").addEventListener("click", () => {
    store.del(TOKEN);
    $("token").value = "";
    $("publish-note").textContent = "This browser no longer keeps the token.";
  });
} else {
  api("/api/bookmarklet?for=recent").then((b) => ($("bookmark").href = b.href), () => {});
  noClick("bookmark");
}

// Writes the games on the page to site-data/recent.json in the repository that
// serves this site (owner.github.io/repo), through the GitHub API.
async function publish() {
  const token = $("token").value.trim();
  if (!token) throw new Error("paste a token first");
  const owner = location.hostname.split(".")[0], repo = location.pathname.split("/").filter(Boolean)[0];
  if (!location.hostname.endsWith(".github.io") || !repo) throw new Error("this page isn't on GitHub Pages");
  const url = `https://api.github.com/repos/${owner}/${repo}/contents/site-data/recent.json`;
  const headers = { Authorization: "Bearer " + token, Accept: "application/vnd.github+json" };
  $("publish-note").textContent = "Publishing…";
  const cur = await fetch(url + "?ref=main", { headers });
  if (!cur.ok && cur.status !== 404) throw new Error(`GitHub answered ${cur.status} reading the file`);
  const sha = cur.ok ? (await cur.json()).sha : undefined;
  const body = JSON.stringify({ me: shown.me, games: shown.games }) + "\n";
  let bin = "";
  for (const b of new TextEncoder().encode(body)) bin += String.fromCharCode(b);
  const put = await fetch(url, {
    method: "PUT",
    headers,
    body: JSON.stringify({ message: "Recent games: the latest from the profile", content: btoa(bin), sha, branch: "main" }),
  });
  if (!put.ok) throw new Error(`GitHub answered ${put.status}: ${(await put.json().catch(() => ({}))).message || ""}`);
  store.set(TOKEN, token);
  $("publish-note").textContent = "Published. The site rebuilds in a few minutes; this browser keeps the token.";
}

// ---- the page

async function refresh() {
  try {
    data = await api(STATIC ? "data/recent.json" : "/api/recent" + (player ? "?player=" + encodeURIComponent(player) : ""));
    showError("");
    render();
  } catch (err) {
    showError(err.message);
  }
}

// The games to show: the saved (or published) ones, with the bookmark's news.
function rows() {
  const me = data.me;
  if (STATIC && fromProfile && Array.isArray(fromProfile.games) && fromProfile.games.length) {
    // the profile's latest games, with what the published data adds for each
    const known = new Map(data.games.map((g) => [g.id, g]));
    const list = fromProfile.games.filter((g) => g && g.id && g.profile).map((g) => known.get(g.id) || profileRow(g.id, g.profile, fromProfile.me || me));
    return { me: fromProfile.me || me, games: list.sort(newestFirst).slice(0, LATEST) };
  }
  const saved = new Set(data.games.map((g) => g.id));
  const extra = Object.entries(pending).filter(([id, p]) => !saved.has(id) && p.profile).map(([id, p]) => ({ ...profileRow(id, p.profile, me), pending: p }));
  return { me, games: [...extra, ...data.games].sort(newestFirst) };
}

function render() {
  if (!data) return;
  shown = rows();
  const all = shown.games, done = all.filter((g) => !g.pending);
  $("title").textContent = `${shown.me}'s recent games`;
  const dates = done.map((g) => g.date).filter(Boolean);
  let meta = done.length
    ? `${done.length} latest games, from ${day(dates[dates.length - 1])} to ${day(dates[0])}.`
    : "No saved games yet: load some with the bookmark below.";
  if (STATIC && fromProfile && fromProfile.at) meta += ` Updated from your profile in this browser on ${fromProfile.at.slice(0, 10)}.`;
  $("meta").textContent = meta;
  const s = summarize(done);
  kpis(s, done);
  sides(s, done);
  games(all);
}

function kpi(label, value, sub) {
  const k = el("div", "kpi");
  k.append(el("div", "label", label), el("div", "value", value), el("div", "sub", sub));
  return k;
}

function kpis(s, list) {
  const n = s.games, box = $("kpis");
  box.replaceChildren();
  if (!n) return;
  const expected = s.leveled === n
    ? `expected ${one(s.expected)} from the levels`
    : s.leveled ? `${score(s.level_score)} in the ${games_(s.leveled)} with levels, expected ${one(s.expected)}` : "no levels yet: the bookmark adds them";
  box.append(kpi("Score", `${score(s.score)}/${n}`, expected));
  box.append(kpi("Opponents", `${s.opponents} players`, s.leveled ? `levels ${one(s.opp_low)} to ${one(s.opp_high)}` + (s.leveled < n ? ` in ${games_(s.leveled)}` : "") : ""));
  const lv = list.filter((g) => g.my_level);
  if (lv.length) {
    const now = lv[0].my_level, then = lv[lv.length - 1].my_level, d = now - then;
    box.append(kpi("Your level", one(now), lv.length > 1 ? `${d >= 0 ? "+" : "−"}${one(Math.abs(d))} since ${day(lv[lv.length - 1].date)}` : "in the latest game"));
  }
}

function sides(s, list) {
  const t = $("sides"), w = s.wins, l = s.losses;
  const missing = list.filter((g) => !(g.turns > 0)).length;
  $("sides-note").textContent = "Averages over these games; ties are left out. Turns and $ are yours." +
    (missing ? ` ${games_(missing)} without a saved log ${missing === 1 ? "isn't" : "aren't"} counted here.` : "");
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
  const turn = (x) => (x.first_province ? "turn " + one(x.first_province) : "–");
  row("First Province", turn(w), turn(l));
  row("$ per turn, turns 1–10", w.games ? one(w.coins) : "–", l.games ? one(l.coins) : "–");
  row("Length", w.games ? one(w.turns) + " turns" : "–", l.games ? one(l.turns) + " turns" : "–");
  row("No Province bought", `${w.no_province} of ${w.games}`, `${l.no_province} of ${l.games}`);
  t.append(body);
}

function games(list) {
  const t = $("games");
  t.replaceChildren();
  const head = el("tr");
  for (const h of ["", "Opponent", "Score", "Win chance", "Seat", "Turns", "Ending", "Opening", "1st Province"]) head.append(el("th", "", h));
  t.append(el("thead"));
  t.tHead.append(head);
  const body = el("tbody");
  let last = null;
  for (const g of list) {
    if (g.date !== last) {
      last = g.date;
      const same = list.filter((x) => x.date === g.date && !x.pending && x.result !== undefined);
      const sum = same.reduce((a, x) => a + x.result, 0), exp = same.reduce((a, x) => a + (x.chance || 0), 0);
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
  const [mark, word] = RESULT[g.result] || ["?", ""];
  const res = el("td");
  const linked = !STATIC && !g.profile_only;
  const a = el(linked ? "a" : "span", "res " + word, mark);
  a.title = `Game ${g.id}`;
  if (linked) {
    a.href = "analyze.html#" + g.id;
    a.title += ": open it on the analysis page";
  }
  res.append(a);
  const vp = g.final || [g.my_vp, g.opp_vp];
  const dash = (v, f) => (v ? f(v) : "–");
  tr.append(res, opponentTd(g), el("td", "", vp[0] !== undefined ? `${vp[0]}–${vp[1]}` : "–"), el("td", "", g.chance ? Math.round(g.chance * 100) + "%" : "–"),
    el("td", "", g.profile_only ? "–" : g.first ? "1st" : "2nd"), el("td", "", dash(g.turns, String)), el("td", "", ENDINGS[g.ending] || g.ending || "–"),
    el("td", "", g.opening || "–"), el("td", "", dash(g.first_province, (x) => "turn " + x)));
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
