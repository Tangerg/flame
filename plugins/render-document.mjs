import assert from "node:assert/strict";
import { readFile, writeFile, mkdtemp, rm } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { rolldown } from "rolldown";

const require = createRequire(import.meta.url);
const stylesheet = await readFile(
  require.resolve("@flame/ui/styles.css"),
  "utf8",
);
const compiler = await rolldown({
  input: new URL("render-controls.mjs", import.meta.url).pathname,
  platform: "node",
  resolve: {
    alias: {
      react: [
        require.resolve("react/package.json").replace(/package.json$/, ""),
      ],
      "react-dom": [
        require.resolve("react-dom/package.json").replace(/package.json$/, ""),
      ],
    },
  },
  transform: {
    define: { "process.env.NODE_ENV": JSON.stringify("production") },
  },
  onwarn(warning, report) {
    if (
      warning.code === "MODULE_LEVEL_DIRECTIVE" &&
      warning.message.includes("use client")
    )
      return;
    report(warning);
  },
});
let renderControls;
let directory;
try {
  directory = await mkdtemp(join(tmpdir(), "flame-renderer-controls-"));
  const bundle = await compiler.generate({ format: "cjs" });
  const module = join(directory, "controls.cjs");
  await writeFile(module, bundle.output[0].code);
  ({ renderControls } = require(module));
} finally {
  await compiler.close();
  if (directory) await rm(directory, { recursive: true, force: true });
}

export function renderDocument(template, controls) {
  const replace = (document, marker, content) => {
    const slot = `<!-- ${marker} -->`;
    assert.equal(
      document.split(slot).length,
      2,
      `renderer requires exactly one ${marker} slot`,
    );
    return document.replace(slot, () => content);
  };
  let document = replace(template, "UI_STYLES", `<style>${stylesheet}</style>`);
  for (const [marker, markup] of Object.entries(renderControls(controls)))
    document = replace(document, marker, markup);
  return document;
}
