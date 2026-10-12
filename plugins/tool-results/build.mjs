import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { renderDocument } from "../render-document.mjs";

const template = await readFile(
  new URL("src/result.html", import.meta.url),
  "utf8",
);
const html = renderDocument(template, {
  TITLE: { id: "title", children: "Recorded tool result" },
  REFRESH: {
    id: "refresh",
    variant: "outline",
    size: "sm",
    disabled: true,
    children: "Refresh recorded result",
  },
  FILTER: { id: "filter", type: "search", size: "sm" },
  MORE: {
    id: "more",
    variant: "outline",
    size: "sm",
    hidden: true,
    children: "Show more matching rows",
  },
});
const output = new URL("renderers/result.html", import.meta.url);
if (process.argv.includes("--check"))
  assert.equal(
    await readFile(output, "utf8"),
    html,
    "rebuild the portable tool-result resource",
  );
else await writeFile(output, html);
