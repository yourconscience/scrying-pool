const $ = (id) => document.getElementById(id);
// The hosted build sets DOMBOT_STATIC and runs the formatter as WebAssembly in the
// browser; the local app (dombot web) talks to its own server, which can also ask DomBot.
const STATIC = window.DOMBOT_STATIC === true;
let markdown = "";
let fileName = "match.md";

let wasmReady = null;
function loadWasm() {
  wasmReady ??= (async () => {
    const go = new Go();
    const bytes = await (await fetch("dombot.wasm")).arrayBuffer();
    const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
    go.run(instance);
  })();
  return wasmReady;
}

function fromWasm(json) {
  const data = JSON.parse(json);
  if (data.error) throw new Error(data.error);
  return data;
}

const backend = STATIC
  ? {
      status: async () => ({ discord: false }),
      sample: async () => (await loadWasm(), fromWasm(dombotSample())),
      format: async (req) => (await loadWasm(), fromWasm(dombotFormat(req.chat, req.dombot))),
    }
  : {
      status: () => api("/api/status"),
      sample: () => api("/api/sample"),
      format: (req) => api("/api/format", req),
    };

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

if (STATIC) {
  $("ask-row").hidden = true;
  $("download").hidden = true; // the hosted viewer blocks downloads; Copy works
  $("paste-box").open = true;
  $("static-note").hidden = false;
}

backend.status().then((s) => {
  $("ask").disabled = !s.discord;
  $("ask-hint").textContent = s.discord
    ? "(DMs DomBot as you; against Discord's ToS for automation)"
    : "(needs a Discord token on this machine, see README)";
});

async function loadDemo() {
  const s = await backend.sample();
  $("chat").value = s.chat;
  $("dombot").value = s.dombot;
  $("paste-box").open = true;
  $("ask").checked = false;
  $("form").requestSubmit();
}
$("demo").addEventListener("click", loadDemo);

$("form").addEventListener("submit", async (e) => {
  e.preventDefault();
  showError("");
  const ask = $("ask").checked && !$("dombot").value.trim();
  $("go").disabled = true;
  $("go").textContent = ask ? "Asking DomBot…" : "Formatting…";
  try {
    const r = await backend.format({ chat: $("chat").value, dombot: $("dombot").value, ask });
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
  } catch (err) {
    showError(err.message);
  } finally {
    $("go").disabled = false;
    $("go").textContent = "Format notes";
  }
});

$("copy").addEventListener("click", () => {
  navigator.clipboard.writeText(markdown).then(
    () => {
      $("copy").textContent = "Copied";
      setTimeout(() => ($("copy").textContent = "Copy Markdown"), 1500);
    },
    () => {
      // clipboard refused: show the source selected so the viewer can copy it
      $("source-md").hidden = false;
      $("rendered").hidden = true;
      getSelection().selectAllChildren($("source-md"));
    },
  );
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

// The hosted page opens with the demo already formatted, so visitors see what it does.
if (STATIC) loadDemo().catch((err) => showError(err.message));
