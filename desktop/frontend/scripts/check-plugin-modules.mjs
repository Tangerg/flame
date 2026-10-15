import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { build } from "vite";

const root = fileURLToPath(new URL("../", import.meta.url));
const dist = resolve(root, "dist");
const html = await readFile(resolve(dist, "index.html"), "utf8");
const map = html.match(/<script\b[^>]*type="importmap"[^>]*>([\s\S]*?)<\/script>/);
assert(map, "the production workbench must publish its shared module import map");
const { imports } = JSON.parse(map[1]);
const modules = [
  "react",
  "react-dom",
  "react-dom/client",
  "react/jsx-runtime",
  "react/jsx-dev-runtime",
];
for (const module of modules)
  assert.equal(typeof imports[module], "string", `the public module ${module} must be mapped`);

// Link a real author's named imports against emitted ESM, where CommonJS star
// re-exports can disappear even though the host's own build still succeeds.
const entry = resolve(root, "flame-public-react-consumer.mjs");
const result = await build({
  configFile: false,
  root,
  logLevel: "error",
  resolve: {
    alias: modules
      .toSorted((left, right) => right.length - left.length)
      .map((module) => ({
        find: module,
        replacement: resolve(dist, imports[module].replace(/^\/+/, "")),
      })),
  },
  plugins: [
    {
      name: "flame-public-react-consumer",
      resolveId(id) {
        if (id === entry) return `\0${entry}`;
      },
      load(id) {
        if (id !== `\0${entry}`) return;
        return `
          import React, { useState, useEffect } from "react";
          import { createPortal } from "react-dom";
          import { createRoot, hydrateRoot } from "react-dom/client";
          import { Fragment, jsx, jsxs } from "react/jsx-runtime";
          import { jsxDEV } from "react/jsx-dev-runtime";
          export { useState, useEffect, createPortal, createRoot, hydrateRoot, jsx, jsxs, jsxDEV };
          export const sharedFragment = React.Fragment === Fragment;
        `;
      },
    },
  ],
  build: {
    write: false,
    minify: false,
    lib: { entry, formats: ["es"] },
  },
});
assert(Array.isArray(result), "the library build must return its format outputs");
assert.equal(result.length, 1, "the public consumer must produce one ESM output");
const chunk = result[0].output.find((output) => output.type === "chunk" && output.isEntry);
assert(chunk, "the public consumer entry must be emitted");
assert.deepEqual(
  chunk.exports.toSorted(),
  [
    "useState",
    "useEffect",
    "createPortal",
    "createRoot",
    "hydrateRoot",
    "jsx",
    "jsxs",
    "jsxDEV",
    "sharedFragment",
  ].toSorted(),
  "the author must retain every requested public binding",
);
console.log("[check-plugin-modules] production React and JSX named imports link successfully");
