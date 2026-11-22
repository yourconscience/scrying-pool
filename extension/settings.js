// Settings shared by background.js and the popup.
export const defaults = { enabled: true, folder: "scrying-pool", jsonl: true, log: false };

// A folder inside the download folder: no leading or trailing slashes, no "..".
export const cleanFolder = (f) => (f || "").split(/[\\/]+/).map((p) => p.trim()).filter((p) => p && p !== "." && p !== "..").join("/") || defaults.folder;
