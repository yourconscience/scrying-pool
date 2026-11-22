// Scrying Pool extension: passes each finished game from page.js to background.js,
// which saves it. Page scripts can't reach the extension's APIs; this one can.
window.addEventListener("message", (e) => {
  if (e.source !== window || !e.data || e.data.scrySaveLogs !== "game") return;
  chrome.runtime.sendMessage({ type: "game", record: e.data.record });
});
