import { readFile, readdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const output = fileURLToPath(new URL("../../../out/web/lab/", import.meta.url));
const entries = (await readdir(path.join(output, "lab"))).filter((name) => name.endsWith(".html"));
if (!entries.length) throw new Error("the lab build has no preview documents");
let shared;
for (const entry of entries) {
  const html = await readFile(path.join(output, "lab", entry), "utf8");
  const sheets = new Set(
    [...html.matchAll(/<link\b[^>]*rel="stylesheet"[^>]*href="([^"]+)"/g)].map((match) => match[1]),
  );
  if (!sheets.size) throw new Error(`lab document has no stylesheet: ${entry}`);
  for (const href of sheets) {
    const stylesheet = path.join(output, new URL(href, "https://lab.invalid/lab/").pathname);
    if (!(await readFile(stylesheet, "utf8")).trim())
      throw new Error(`lab document has an empty stylesheet: ${entry}`);
  }
  shared = shared ? shared.intersection(sheets) : sheets;
}
if (!shared.size) throw new Error("lab documents must load the shared extracted StyleX stylesheet");
console.log(`[check-lab-build] ${entries.length} documents load the shared stylesheet`);
