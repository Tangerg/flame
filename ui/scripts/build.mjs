import { readFile, writeFile, mkdir, readdir, rm } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import path from "node:path";
import * as babel from "@babel/core";
import stylexPlugin from "@stylexjs/babel-plugin";
import { rolldown } from "rolldown";

const root = fileURLToPath(new URL("../", import.meta.url));
async function files(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  return (
    await Promise.all(
      entries.map((entry) =>
        entry.isDirectory()
          ? files(path.join(dir, entry.name))
          : [path.join(dir, entry.name)],
      ),
    )
  ).flat();
}
await rm(path.join(root, "dist"), { recursive: true, force: true });
await mkdir(path.join(root, "dist/primitives"), { recursive: true });
await mkdir(path.join(root, "src/generated"), { recursive: true });
const sources = (await files(path.join(root, "src"))).filter(
  (file) => /\.tsx?$/.test(file) && !file.includes("/generated/"),
);
const documentStyles = await readFile(
  path.join(root, "src/globals.css"),
  "utf8",
);
const text =
  documentStyles +
  (await Promise.all(sources.map((file) => readFile(file, "utf8")))).join("\n");
const tokens = [
  ...new Set(
    [...text.matchAll(/var\((--[a-z][a-z0-9-]*)/g)].map((match) => match[1]),
  ),
].sort();
await writeFile(
  path.join(root, "src/generated/themeTokenNames.ts"),
  `export const themeTokenNames = ${JSON.stringify(tokens)} as const;\n`,
);
const rules = [];
for (const file of [
  ...sources,
  path.join(root, "src/generated/themeTokenNames.ts"),
]) {
  const result = await babel.transformFileAsync(file, {
    babelrc: false,
    configFile: false,
    presets: [
      "@babel/preset-typescript",
      ["@babel/preset-react", { runtime: "automatic", development: false }],
    ],
    plugins: [
      [
        stylexPlugin,
        {
          dev: false,
          runtimeInjection: false,
          unstable_moduleResolution: { type: "commonJS", rootDir: root },
        },
      ],
    ],
  });
  rules.push(...(result.metadata.stylex ?? []));
  const target = path.join(
    root,
    "dist",
    path.relative(path.join(root, "src"), file).replace(/\.tsx?$/, ".js"),
  );
  await mkdir(path.dirname(target), { recursive: true });
  await writeFile(
    target,
    result.code.replace(
      /(from\s+["']|import\s+["'])(\.[^"']+)(["'])/g,
      (_, prefix, specifier, quote) =>
        `${prefix}${specifier.endsWith(".js") ? specifier : specifier + ".js"}${quote}`,
    ) + "\n",
  );
}
await writeFile(
  path.join(root, "dist/styles.css"),
  documentStyles +
    "\n" +
    stylexPlugin.processStylexRules(rules, { useLayers: false }),
);
const compiler = await rolldown({
  input: path.join(root, "dist/guest-theme.js"),
});
const guest = await compiler.generate({ format: "iife", minify: true });
await compiler.close();
const source = guest.output[0].code;
if (/<\/script/i.test(source)) throw new Error("Invalid guest theme script.");
await writeFile(
  path.join(root, "dist/theme-bootstrap.js"),
  `export const themeBootstrapSource = ${JSON.stringify(source)};\n`,
);
await writeFile(
  path.join(root, "dist/theme-bootstrap.d.ts"),
  "export declare const themeBootstrapSource: string;\n",
);
execFileSync(
  path.join(root, "node_modules/.bin/tsc"),
  ["-p", path.join(root, "tsconfig.json")],
  { stdio: "inherit" },
);
console.log(`Built @flame/ui with ${tokens.length} appearance tokens.`);
