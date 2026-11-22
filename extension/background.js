// Scrying Pool extension: saves each finished game as files in a folder inside the
// browser's download folder. Extensions can't write anywhere else.
//   <id>.jsonl  one line for `scry import` (log, score screen, kingdom, players)
//   <id>.log    the log alone, for `scry review` or reading
import { defaults, cleanFolder } from "./settings.js";

const download = (filename, text, type) => chrome.downloads.download({
  url: `data:${type};charset=utf-8,` + encodeURIComponent(text),
  filename, conflictAction: "overwrite", saveAs: false,
});

async function save(r) {
  if (!r || !/^\d{1,12}$/.test(r.id) || typeof r.log !== "string") return;
  const s = await chrome.storage.sync.get(defaults);
  if (!s.enabled) return;
  const { saved = [], unseen = 0 } = await chrome.storage.local.get(["saved", "unseen"]);
  if (saved.some((g) => g.id === r.id)) return;
  const folder = cleanFolder(s.folder);
  const entry = { id: r.id, players: r.original, at: new Date().toISOString(), files: [] };
  try {
    if (s.jsonl) { await download(`${folder}/${r.id}.jsonl`, JSON.stringify(r) + "\n", "application/json"); entry.files.push(`${r.id}.jsonl`); }
    if (s.log) { await download(`${folder}/${r.id}.log`, r.log, "text/plain"); entry.files.push(`${r.id}.log`); }
  } catch (e) {
    entry.error = String(e && e.message || e);
  }
  await chrome.storage.local.set({ saved: [entry, ...saved].slice(0, 200), unseen: unseen + 1 });
  chrome.action.setBadgeBackgroundColor({ color: entry.error ? "#b3261e" : "#3d6b35" });
  chrome.action.setBadgeText({ text: entry.error ? "!" : String(unseen + 1) });
}

chrome.runtime.onMessage.addListener((msg, sender) => {
  if (msg && msg.type === "game" && sender.tab) save(msg.record);
});
