import assert from "node:assert/strict";
import {
  readFile,
  writeFile,
  readdir,
  access,
  mkdtemp,
  rm,
} from "node:fs/promises";
import { createRequire } from "node:module";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { rolldown } from "rolldown";

const require = createRequire(import.meta.url);
const css = await readFile(require.resolve("@flame/ui/styles.css"), "utf8");
const root = new URL("./", import.meta.url);
for (const directory of await readdir(root, { withFileTypes: true })) {
  if (!directory.isDirectory()) continue;
  const source = new URL(`${directory.name}/src/view.tsx`, root);
  try {
    await access(source);
  } catch (error) {
    if (error.code === "ENOENT") continue;
    throw error;
  }
  const manifest = JSON.parse(
    await readFile(new URL(`${directory.name}/plugin.json`, root), "utf8"),
  );
  const views =
    manifest.extensions["io.github.tangerg.flame"].contributes.views;
  assert.equal(
    views.length,
    1,
    "a portable page source must own exactly one declared resource",
  );
  const view = views[0];
  const compiler = await rolldown({
    input: source.pathname,
    platform: view.type === "static" ? "node" : "browser",
    resolve: {
      alias: {
        react: [
          require.resolve("react/package.json").replace(/package.json$/, ""),
        ],
        "react-dom": [
          require
            .resolve("react-dom/package.json")
            .replace(/package.json$/, ""),
        ],
      },
    },
    transform: {
      jsx: { runtime: "automatic", development: false },
      define: { "process.env.NODE_ENV": JSON.stringify("production") },
    },
    // These artifacts execute in browsers; RSC directives do not define their lifecycle.
    onwarn(warning, report) {
      if (
        warning.code === "MODULE_LEVEL_DIRECTIVE" &&
        warning.message.includes("use client")
      )
        return;
      report(warning);
    },
  });
  let content;
  try {
    const bundle = await compiler.generate({
      format: view.type === "static" ? "cjs" : "iife",
      minify: true,
    });
    const script = bundle.output[0].code;
    if (view.type === "static") {
      const temporary = await mkdtemp(join(tmpdir(), "flame-static-view-"));
      let render;
      try {
        const module = join(temporary, "view.cjs");
        await writeFile(module, script);
        render = require(module).render;
      } finally {
        await rm(temporary, { recursive: true, force: true });
      }
      const { icons } = await import("@flame/icons/catalog");
      const drawings = await Promise.all(
        icons.map(async (icon) => ({
          ...icon,
          svg: await readFile(
            require.resolve(`@flame/icons/svg/24/${icon.name}.svg`),
            "utf8",
          ),
        })),
      );
      content = render(drawings);
    } else
      content = `<div id="root"></div><script>${script.replace(/<\/script/gi, "<\\/script")}</script>`;
  } finally {
    await compiler.close();
  }
  const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${view.title}</title><style>${css}</style></head><body>${content}</body></html>\n`;
  assert.ok(
    Buffer.byteLength(html) < 512 << 10,
    `${directory.name} resource exceeds the Runtime page bound`,
  );
  const target = new URL(`${directory.name}/${view.entry}`, root);
  if (process.argv.includes("--check"))
    assert.equal(
      await readFile(target, "utf8"),
      html,
      `rebuild the portable ${directory.name} view`,
    );
  else await writeFile(target, html);
  console.log(`${directory.name}: ${Buffer.byteLength(html)} bytes`);
}
