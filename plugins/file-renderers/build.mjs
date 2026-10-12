import assert from "node:assert/strict";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { renderDocument } from "../render-document.mjs";

const root = dirname(fileURLToPath(import.meta.url));
const require = createRequire(
  new URL("../../desktop/frontend/package.json", import.meta.url),
);
const { build } = await import(pathToFileURL(require.resolve("vite")).href);
const parser = require.resolve("marked");
const license = await readFile(
  join(dirname(require.resolve("marked/package.json")), "LICENSE"),
  "utf8",
);
const bundle = await build({
  root,
  configFile: false,
  logLevel: "warn",
  resolve: { alias: { marked: parser } },
  build: {
    write: false,
    minify: true,
    sourcemap: false,
    lib: {
      entry: join(root, "src/markdown.mjs"),
      name: "FlameMarkdown",
      formats: ["iife"],
    },
  },
});
assert.ok(Array.isArray(bundle));
assert.equal(bundle.length, 1);
const scripts = bundle[0].output.filter((entry) => entry.type === "chunk");
assert.equal(scripts.length, 1);
const template = renderDocument(
  await readFile(join(root, "src/markdown.html"), "utf8"),
  {
    TITLE: { id: "title", children: "Captured Markdown file" },
    REFRESH: {
      id: "refresh",
      variant: "outline",
      size: "sm",
      disabled: true,
      children: "Refresh captured file",
    },
  },
);
assert.equal(template.split("<!-- SCRIPT -->").length, 2);
const html = template.replace(
  "<!-- SCRIPT -->",
  `<script>\n/*\n${license}\n*/\n${scripts[0].code.replaceAll(/<\/script/gi, "<\\/script")}\n</script>`,
);
const output = join(root, "renderers/markdown.html");
if (process.argv.includes("--check")) {
  assert.equal(
    await readFile(output, "utf8"),
    html,
    "rebuild the portable Markdown resource",
  );
} else {
  await mkdir(dirname(output), { recursive: true });
  await writeFile(output, html);
}
