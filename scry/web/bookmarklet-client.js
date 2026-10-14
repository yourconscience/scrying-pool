// "Copy client tables" bookmarklet for dominion.games. Copies the web client's own
// log templates and card tables, which `scry client save` stores for the log parser.
// They belong to 2nd Shuffle, so each player exports them from their own browser
// instead of the repo shipping them. It only reads the page.
(() => {
  const say = (msg) => {
    const d = document.createElement("div");
    d.textContent = msg;
    d.style.cssText = "position:fixed;z-index:99999;top:16px;left:50%;transform:translateX(-50%);" +
      "background:#1c201a;color:#e5e8de;font:14px system-ui;padding:10px 16px;border-radius:6px;" +
      "box-shadow:0 2px 12px rgba(0,0,0,.4)";
    document.body.append(d);
    setTimeout(() => d.remove(), 4000);
  };
  const E = window.EnglishLogEntries, N = window.LogEntryNames;
  if (!E || !N || !window.EnglishCardNames || !window.CardNames) return say("Scrying Pool: open dominion.games and log in first.");
  const script = [...document.scripts].map((s) => s.src).find((s) => /dominion-webclient-body-([\d.]+)/.test(s)) || "";
  const version = (script.match(/dominion-webclient-body-([\d.]+?)(\.min)?\.js/) || [])[1] || "unknown";
  const templates = Object.entries(N).map(([name, v]) => ({ name, english: E[v.uuid], lineClass: v.lineClass || "" }));
  const names = Object.entries(window.EnglishCardNames).map(([k, v]) => ({ key: k, singular: v.singular, plural: v.plural, noun: v.noun }));
  const cards = Object.entries(window.CardNames).map(([k, c]) => ({
    key: k, name: c.name, expansion: c.expansion && c.expansion.name,
    cost: c.cost && { coin: c.cost.coin, potion: c.cost.potion, debt: c.cost.debt, special: c.cost.isSpecial },
    types: (c.types || []).map((t) => t.name),
  }));
  const text = JSON.stringify({ source: "dominion.games web client " + version + ", read from the page", version, templates, names, cards, zones: Object.keys(window.EnglishZoneNames || {}) });
  navigator.clipboard.writeText(text).then(
    () => say(`Scrying Pool: copied ${templates.length} templates and ${cards.length} cards (client ${version}). Now run: scry client save`),
    () => say("Scrying Pool: the browser refused the clipboard. Try again with the page focused."),
  );
})();
