import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { rolldown } from "rolldown";

const require = createRequire(import.meta.url);
const css = await readFile(require.resolve("@flame/ui/styles.css"), "utf8");
const compiler = await rolldown({
  input: new URL("src/memory.tsx", import.meta.url).pathname,
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
    jsx: { runtime: "automatic", development: false },
    define: { "process.env.NODE_ENV": JSON.stringify("production") },
  }, // The artifact runs only in a browser; RSC directives have no role in its IIFE.
  onwarn(warning, report) {
    if (
      warning.code === "MODULE_LEVEL_DIRECTIVE" &&
      warning.message.includes("use client")
    )
      return;
    report(warning);
  },
});
const bundle = await compiler.generate({ format: "iife", minify: true });
await compiler.close();
const script = bundle.output[0].code.replace(/<\/script/gi, "<\\/script");
const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Agent memory</title><style>${css}</style></head><body><div id="root"></div><script>${script}</script></body></html>\n`;
assert.ok(
  Buffer.byteLength(html) < 512 << 10,
  "memory resource exceeds the Runtime page bound",
);
const target = new URL("views/memory.html", import.meta.url);
if (process.argv.includes("--check"))
  assert.equal(
    await readFile(target, "utf8"),
    html,
    "rebuild the portable Memory view",
  );
else await writeFile(target, html);
console.log(`Memory view: ${Buffer.byteLength(html)} bytes.`);
