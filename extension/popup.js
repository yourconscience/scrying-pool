import { defaults, cleanFolder } from "./settings.js";

const $ = (id) => document.getElementById(id);

async function show() {
  const s = await chrome.storage.sync.get(defaults);
  $("enabled").checked = s.enabled;
  $("folder").value = s.folder;
  $("jsonl").checked = s.jsonl;
  $("log").checked = s.log;
  $("folder-hint").textContent = cleanFolder(s.folder);
  const { saved = [] } = await chrome.storage.local.get("saved");
  const list = $("saved");
  list.replaceChildren();
  if (!saved.length) list.append(Object.assign(document.createElement("li"), { textContent: "None yet." }));
  for (const g of saved.slice(0, 15)) {
    const li = document.createElement("li");
    li.append(`${g.id}: ${(g.players || []).join(" vs ")} `);
    const when = document.createElement("small");
    when.textContent = new Date(g.at).toLocaleString();
    li.append(when);
    if (g.error) li.append(Object.assign(document.createElement("div"), { className: "error", textContent: g.error }));
    list.append(li);
  }
  await chrome.storage.local.set({ unseen: 0 });
  chrome.action.setBadgeText({ text: "" });
}

async function store() {
  const folder = cleanFolder($("folder").value);
  await chrome.storage.sync.set({ enabled: $("enabled").checked, folder, jsonl: $("jsonl").checked, log: $("log").checked });
  $("folder-hint").textContent = folder;
  $("status").textContent = "Saved.";
  setTimeout(() => { $("status").textContent = ""; }, 1500);
}

for (const id of ["enabled", "jsonl", "log"]) $(id).addEventListener("change", store);
$("folder").addEventListener("change", store);
show();
