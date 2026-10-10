const $ = (id) => document.getElementById(id);
// The hosted build sets SCRY_STATIC and runs the formatter as WebAssembly in the
// browser; the local app (scry web) talks to its own server, which can also ask DomBot.
const STATIC = window.SCRY_STATIC === true;
let markdown = "";
let fileName = "match.md";

let wasmReady = null;
function loadWasm() {
  wasmReady ??= (async () => {
    const go = new Go();
    const bytes = await (await fetch("scry.wasm")).arrayBuffer();
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
      status: async () => ({ discord: false, art: false }),
      sample: async () => (await loadWasm(), fromWasm(scrySample())),
      format: async (req) => (await loadWasm(), fromWasm(scryFormat(req.chat, req.dombot))),
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

let showArt = false;
backend.status().then((s) => {
  showArt = s.art;
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
    renderKingdoms(r.games || []);
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

// Kingdom viewer: one tile per pile, framed in the colours of its card types.
// Art is only available in the local app, which keeps a private copy per machine.

const FRAME = {
  Night: "#45424f", Treasure: "#efcb55", Victory: "#9cc97a", Curse: "#b48ac8",
  Reaction: "#82acd9", Duration: "#f0a35c", Reserve: "#d8b87e", Ruins: "#a8865f",
  Shelter: "#d98b6d", Action: "#ece2cb",
  event: "#c9c6bd", project: "#eba7b3", landmark: "#8fb58f", way: "#a6cfe1",
  ally: "#e8c897", trait: "#c1aedb", prophecy: "#a8c3ea",
};
const FRAME_ORDER = ["Night", "Treasure", "Victory", "Curse", "Reaction", "Duration", "Reserve", "Ruins", "Shelter", "Action"];

function frames(card) {
  if (FRAME[card.kind]) return [FRAME[card.kind], FRAME[card.kind]];
  const hits = FRAME_ORDER.filter((t) => card.types.includes(t)).map((t) => FRAME[t]);
  return [hits[0] || FRAME.Action, hits[1] || hits[0] || FRAME.Action];
}

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

function costBadges(card) {
  const box = el("span", "cost");
  if (card.cost) box.append(el("span", "coin", card.cost));
  if (card.potion) box.append(el("span", "potion", "P"));
  if (card.debt) box.append(el("span", "debt", String(card.debt)));
  return box;
}

function wikiURL(card) {
  return "https://wiki.dominionstrategy.com/index.php/" + encodeURIComponent(card.name.split("/")[0].replace(/ /g, "_"));
}

function cardTile(card) {
  const landscape = !!FRAME[card.kind];
  const a = el("a", landscape ? "tile landscape" : "tile");
  a.href = wikiURL(card);
  a.target = "_blank";
  a.rel = "noopener";
  const [f1, f2] = frames(card);
  a.style.setProperty("--f1", f1);
  a.style.setProperty("--f2", f2);
  if (card.types.includes("Night")) a.classList.add("dark");
  const typeLine = card.types.join(" – ");
  a.title = `${card.name} · ${typeLine}${card.cost || card.debt || card.potion ? " · " + costText(card) : ""} · ${card.set}`;

  if (!showArt) {
    // Compact chip: cost, then name over "types · set".
    a.classList.add("chip");
    const info = el("span", "info");
    info.append(el("span", "name", card.name), el("span", "meta", `${typeLine} · ${card.set}`));
    a.append(costBadges(card), info);
    return a;
  }
  const art = el("span", "art");
  const img = el("img");
  img.alt = "";
  img.loading = "lazy";
  img.src = "/art/" + encodeURIComponent(card.art);
  img.onerror = () => img.remove();
  art.append(img, el("span", "set", card.set));
  a.append(art, el("span", "name", card.name));
  const foot = el("span", "foot");
  foot.append(costBadges(card), el("span", "types", typeLine));
  a.append(foot);
  return a;
}

function costText(card) {
  return [card.cost && "$" + card.cost, card.potion && "P", card.debt && card.debt + "D"].filter(Boolean).join(" ");
}

function renderKingdoms(games) {
  const heads = [...$("rendered").querySelectorAll("h2")];
  games.forEach((k, i) => {
    if (!k || !heads[i]) return;
    const box = el("section", "kingdom");
    box.setAttribute("aria-label", `Game ${i + 1} kingdom`);
    const cards = el("div", "piles");
    k.cards.forEach((c) => cards.append(cardTile(c)));
    box.append(cards);
    if (k.landscapes.length) {
      const lands = el("div", "landscapes");
      k.landscapes.forEach((c) => lands.append(cardTile(c)));
      box.append(lands);
    }
    const details = heads[i].nextElementSibling;
    (details && details.tagName === "UL" ? details : heads[i]).after(box);
  });
}

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
