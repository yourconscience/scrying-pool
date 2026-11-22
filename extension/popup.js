import { defaults, cleanFolder } from "./settings.js";

const $ = (id) => document.getElementById(id);

// The folder as a shell word, quoted when it has a space or another special character.
const shellWord = (s) => /^[\w.\/-]+$/.test(s) ? s : `'${s.replaceAll("'", `'\\''`)}'`;

const item = (g) => {
  const li = document.createElement("li");
  li.append(`${g.id}: ${(g.players || []).join(" vs ")} `);
  const when = document.createElement("small");
  when.textContent = new Date(g.at).toLocaleString();
  li.append(when);
  return li;
};

async function show() {
  const s = await chrome.storage.sync.get(defaults);
  $("enabled").checked = s.enabled;
  $("folder").value = s.folder;
  $("jsonl").checked = s.jsonl;
  $("log").checked = s.log;
  $("folder-hint").textContent = shellWord(cleanFolder(s.folder));
  warnNoFormat();
  const { saved = [], failed = {} } = await chrome.storage.local.get(["saved", "failed"]);
  const fails = Object.values(failed).sort((a, b) => b.at.localeCompare(a.at));
  $("failed-box").hidden = !fails.length;
  $("failed").replaceChildren(...fails.map((g) => {
    const li = item(g);
    li.append(Object.assign(document.createElement("div"), { className: "error", textContent: g.error }));
    const again = Object.assign(document.createElement("button"), { textContent: "Save again" });
    again.addEventListener("click", async () => {
      again.disabled = true;
      await chrome.runtime.sendMessage({ type: "retry", id: g.id });
      setTimeout(show, 1500);
    });
    li.append(again);
    return li;
  }));
  const list = $("saved");
  list.replaceChildren(...saved.slice(0, 15).map(item));
  if (!saved.length) list.append(Object.assign(document.createElement("li"), { textContent: "None yet." }));
  await chrome.storage.local.set({ unseen: 0 });
  chrome.action.setBadgeText({ text: "" });
}

const warnNoFormat = () => {
  const none = !$("jsonl").checked && !$("log").checked;
  $("status").className = none ? "warn" : "";
  $("status").textContent = none ? "Pick at least one file, or nothing is saved." : "";
};

async function store() {
  const folder = cleanFolder($("folder").value);
  await chrome.storage.sync.set({ enabled: $("enabled").checked, folder, jsonl: $("jsonl").checked, log: $("log").checked });
  $("folder-hint").textContent = shellWord(folder);
  warnNoFormat();
  if ($("status").textContent) return;
  $("status").textContent = "Saved.";
  setTimeout(() => { if ($("status").textContent === "Saved.") $("status").textContent = ""; }, 1500);
}

for (const id of ["enabled", "jsonl", "log"]) $(id).addEventListener("change", store);
$("folder").addEventListener("change", store);
show();
