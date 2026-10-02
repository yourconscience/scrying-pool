const $ = (id) => document.getElementById(id);
let markdown = "";
let fileName = "match.md";

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

api("/api/status").then((s) => {
  $("ask").disabled = !s.discord;
  $("ask-hint").textContent = s.discord
    ? "(DMs DomBot as you; against Discord's ToS for automation)"
    : "(needs a Discord token on this machine, see README)";
});

$("demo").addEventListener("click", async () => {
  const s = await api("/api/sample");
  $("chat").value = s.chat;
  $("dombot").value = s.dombot;
  $("paste-box").open = true;
  $("ask").checked = false;
  $("form").requestSubmit();
});

$("form").addEventListener("submit", async (e) => {
  e.preventDefault();
  showError("");
  const ask = $("ask").checked && !$("dombot").value.trim();
  $("go").disabled = true;
  $("go").textContent = ask ? "Asking DomBot…" : "Formatting…";
  try {
    const r = await api("/api/format", { chat: $("chat").value, dombot: $("dombot").value, ask });
    markdown = r.markdown;
    const title = markdown.split("\n", 1)[0].replace(/^# /, "").replace(/\\/g, "");
    fileName = title.replace(/:.*/, "").replace(/[^\w.-]+/g, "_") + ".md";
    $("rendered").innerHTML = marked.parse(markdown);
    $("source-md").textContent = markdown;
    $("source").textContent = {
      pasted: "Game results: pasted DomBot reply",
      discord: "Game results: fetched from DomBot",
      none: "Chat only (no DomBot results)",
    }[r.source];
    if (r.source === "discord") {
      $("dombot").value = r.dombot;
      $("paste-box").open = true;
    }
    $("warnings").innerHTML = "";
    for (const w of r.warnings || []) {
      const p = document.createElement("p");
      p.className = "warning";
      p.textContent = w;
      $("warnings").append(p);
    }
    $("output").hidden = false;
    $("output").scrollIntoView({ behavior: "smooth" });
  } catch (err) {
    showError(err.message);
  } finally {
    $("go").disabled = false;
    $("go").textContent = "Format notes";
  }
});

$("copy").addEventListener("click", async () => {
  await navigator.clipboard.writeText(markdown);
  $("copy").textContent = "Copied";
  setTimeout(() => ($("copy").textContent = "Copy Markdown"), 1500);
});

$("download").addEventListener("click", () => {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(new Blob([markdown], { type: "text/markdown" }));
  a.download = fileName;
  a.click();
  URL.revokeObjectURL(a.href);
});

$("toggle").addEventListener("click", () => {
  const showSrc = $("source-md").hidden;
  $("source-md").hidden = !showSrc;
  $("rendered").hidden = showSrc;
  $("toggle").textContent = showSrc ? "Show rendered" : "Show Markdown source";
});
