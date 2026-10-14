// "Copy log" bookmarklet for dominion.games. Reads the game log panel of a game you
// loaded with My Table > Load Old Game and copies it as a "scrying-pool log v1"
// text, one log line per line: <indent level><T for turn | S for shuffle>\t<blocks joined by |>.
// It only reads the page. It refuses rated games, because a game you load yourself
// to review is always unrated, so a rated log means a game in progress.
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
  const rows = [...document.querySelectorAll(".log-scroll-container .log-line")];
  if (!rows.length) return say("Scrying Pool: no game log on this page. Load a finished game first.");
  const lines = rows.map((r) => {
    const m = /padding-left:\s*([\d.]+)%/.exec(r.getAttribute("style") || "");
    const level = m ? Math.round(parseFloat(m[1]) / 4.54545) : 0;
    const kind = r.classList.contains("new-turn-line") ? "T" : r.classList.contains("shuffles-line") ? "S" : "";
    return level + kind + "\t" + [...r.children].map((c) => c.textContent.trim()).filter(Boolean).join("|");
  });
  if (/\brated\b/.test(lines[0]) && !/\bunrated\b/.test(lines[0]))
    return say("Scrying Pool: this is a rated game. Only finished games loaded for review can be copied.");
  const text = "# scrying-pool log v1\n" + lines.join("\n") + "\n";
  window.__scryLog = text;
  navigator.clipboard.writeText(text).then(
    () => say(`Scrying Pool: copied ${lines.length} log lines.`),
    () => say("Scrying Pool: the browser refused the clipboard. Try again with the page focused."),
  );
})();
