import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { fileURLToPath, pathToFileURL } from "node:url";

const require = createRequire(
  new URL("../../desktop/frontend/package.json", import.meta.url),
);
const { icons } = await import(
  pathToFileURL(require.resolve("@flame/icons/catalog")).href
);
const escape = (text) =>
  text
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
const cards = [];
for (const icon of icons) {
  const svg = await readFile(
    require.resolve(`@flame/icons/svg/24/${icon.name}.svg`),
    "utf8",
  );
  const search = [
    icon.name,
    icon.component,
    icon.label,
    icon.group,
    ...icon.tags,
  ]
    .join(" ")
    .toLowerCase();
  cards.push(
    `<article data-search="${escape(search)}" title="${escape(icon.label)}">${svg}<div>${escape(icon.component)}</div><code>${escape(icon.name)}</code><small>${escape(icon.group)}</small></article>`,
  );
}
const template = await readFile(
  new URL("src/icon-gallery.html", import.meta.url),
  "utf8",
);
assert.equal(template.split("<!-- ICONS -->").length, 2);
const html = template.replace("<!-- ICONS -->", cards.join("\n"));
assert.ok(
  Buffer.byteLength(html) < 512 << 10,
  "icon resource exceeds the Runtime page bound",
);
const output = fileURLToPath(
  new URL("views/icon-gallery.html", import.meta.url),
);
if (process.argv.includes("--check"))
  assert.equal(
    await readFile(output, "utf8"),
    html,
    "rebuild the portable icon gallery",
  );
else await writeFile(output, html);
