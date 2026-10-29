import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync, unlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const script = fileURLToPath(new URL("./check-bundle-size.mjs", import.meta.url));
const lazyChunks = [
  "AppearancePane",
  "BrandPane",
  "ApprovalsPane",
  "ConnectionPane",
  "HooksPane",
  "McpServersPane",
  "PersonalizationPane",
  "PluginsPane",
  "ProvidersPane",
  "FileWorkspace",
  "SubagentsPanel",
  "shiki",
  "mermaid",
].map((name) => `${name}-fixture.js`);

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "flame bundle budget "));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const assets = join(root, "dist/assets");
  mkdirSync(assets, { recursive: true });
  for (const name of ["entry.js", "entry.css", "katex-fixture.css", ...lazyChunks]) {
    writeFileSync(join(assets, name), "");
  }
  writeFileSync(
    join(root, "dist/index.html"),
    '<script src="/assets/entry.js"></script><link rel="stylesheet" href="/assets/entry.css">',
  );
  return {
    assets,
    run() {
      const result = spawnSync(process.execPath, [script], { cwd: root, encoding: "utf8" });
      assert.ifError(result.error);
      return result;
    },
    size(name, bytes) {
      writeFileSync(join(assets, name), Buffer.alloc(bytes));
    },
    index(html) {
      writeFileSync(join(root, "dist/index.html"), html);
    },
  };
}

for (const [label, asset, limit] of [
  ["JS", "entry.js", 2_785_000],
  ["CSS", "entry.css", 146_000],
  ["math stylesheet", "katex-fixture.css", 100_000],
  ["settings panes", "AppearancePane-fixture.js", 200_000],
  ["workspace views", "FileWorkspace-fixture.js", 250_000],
  ["syntax highlighting", "shiki-fixture.js", 3_000_000],
  ["diagram rendering", "mermaid-fixture.js", 3_000_000],
]) {
  test(`${label}: the exact budget passes and one extra byte fails independently`, (t) => {
    const build = fixture(t);
    build.size(asset, limit);
    const equal = build.run();
    assert.equal(equal.status, 0, equal.stdout + equal.stderr);
    assert.doesNotMatch(equal.stdout, /FAIL/);
    build.size(asset, limit + 1);
    const exceeded = build.run();
    assert.equal(exceeded.status, 1, exceeded.stdout + exceeded.stderr);
    assert.match(exceeded.stdout, new RegExp(`${label}[^\\n]*FAIL`));
    assert.match(exceeded.stderr, /FAIL/);
  });
}

test("a missing lazy chunk fails even when every byte budget passes", (t) => {
  const build = fixture(t);
  unlinkSync(join(build.assets, "mermaid-fixture.js"));
  const result = build.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /diagram rendering: no chunk found/);
});

test("eager math styles fail even below their size budget", (t) => {
  const build = fixture(t);
  build.index(
    '<script src="/assets/entry.js"></script><link rel="stylesheet" href="/assets/entry.css"><link rel="prefetch" href="/assets/katex-fixture.css">',
  );
  const result = build.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /math stylesheet: index.html references/);
});
