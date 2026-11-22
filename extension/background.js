// Scrying Pool extension: saves each finished game as files in a folder inside the
// browser's download folder. Extensions can't write anywhere else.
//   <id>.jsonl  one line for `scry import` (log, score screen, kingdom, players)
//   <id>.log    the log alone, for `scry review` or reading
// A game whose files didn't all finish is kept, with its record, for the popup's
// Save again button; only fully saved games count as saved.
import { defaults, cleanFolder } from "./settings.js";

// Resolves once the download is complete; download() itself resolves when it starts.
const finished = (id) => new Promise((resolve, reject) => {
  const settle = (state, error) => {
    if (state === "complete") { off(); resolve(); }
    else if (state === "interrupted") { off(); reject(new Error(`download interrupted${error ? ": " + error : ""}`)); }
  };
  const changed = (d) => { if (d.id === id && d.state) settle(d.state.current, d.error && d.error.current); };
  const off = () => chrome.downloads.onChanged.removeListener(changed);
  chrome.downloads.onChanged.addListener(changed);
  chrome.downloads.search({ id }).then(([d]) => d && settle(d.state, d.error)); // it may have finished already
});

const download = async (filename, text, type) => finished(await chrome.downloads.download({
  url: `data:${type};charset=utf-8,` + encodeURIComponent(text),
  filename, conflictAction: "overwrite", saveAs: false,
}));

async function save(r) {
  if (!r || !/^\d{1,12}$/.test(r.id) || typeof r.log !== "string") return;
  const s = await chrome.storage.sync.get(defaults);
  if (!s.enabled || (!s.jsonl && !s.log)) return;
  const { saved = [], failed = {}, unseen = 0 } = await chrome.storage.local.get(["saved", "failed", "unseen"]);
  if (saved.some((g) => g.id === r.id)) return;
  const folder = cleanFolder(s.folder);
  const entry = { id: r.id, players: r.original, at: new Date().toISOString(), files: [] };
  // kept as failed until its files finish, in case the browser stops the worker meanwhile
  failed[r.id] = { ...entry, record: r, error: "not finished: the browser stopped the save" };
  await chrome.storage.local.set({ failed });
  try {
    if (s.jsonl) { await download(`${folder}/${r.id}.jsonl`, JSON.stringify(r) + "\n", "application/json"); entry.files.push(`${r.id}.jsonl`); }
    if (s.log) { await download(`${folder}/${r.id}.log`, r.log, "text/plain"); entry.files.push(`${r.id}.log`); }
  } catch (e) {
    entry.error = String(e && e.message || e);
  }
  if (entry.error) failed[r.id] = { ...entry, record: r };
  else delete failed[r.id];
  await chrome.storage.local.set({ saved: entry.error ? saved : [entry, ...saved].slice(0, 200), failed, unseen: unseen + 1 });
  chrome.action.setBadgeBackgroundColor({ color: entry.error ? "#b3261e" : "#3d6b35" });
  chrome.action.setBadgeText({ text: entry.error ? "!" : String(unseen + 1) });
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg) return;
  if (msg.type === "game" && sender.tab) save(msg.record);
  if (msg.type === "retry" && !sender.tab) { // answers once the save is over, so the popup can show the outcome
    chrome.storage.local.get("failed")
      .then(({ failed = {} }) => failed[msg.id] && save(failed[msg.id].record))
      .finally(() => sendResponse({ done: true }));
    return true;
  }
});
