import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
  unlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { build } from "vite";
import {
  ARTIFACT_GRAPH,
  LAZY_BUDGETS,
  analyzeArtifactGraph,
  isStylesheetRequest,
  readArtifactGraph,
} from "../artifactGraph.mjs";
import { artifactDocument, artifactGraph } from "../artifactGraph.vite.mjs";

const script = fileURLToPath(new URL("./check-bundle-size.mjs", import.meta.url));
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const index =
  '<script src="/appearance.generated.js"></script><script src="/focus-modality.js"></script><script type="module" src="/assets/entry.js"></script><link rel="stylesheet" href="/assets/entry.css">';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "flame bundle budget "));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const dist = join(root, "dist");
  const assets = join(dist, "assets");
  mkdirSync(assets, { recursive: true });
  const files = {
    "appearance.generated.js": {},
    "focus-modality.js": {},
    "assets/entry.js": { modules: ["src/bootstrap.js"], imports: [] },
    "assets/main.js": { modules: ["src/main.js"], imports: [] },
    "assets/entry.css": {},
    "assets/math.css": {},
    "assets/math-owner.js": {
      modules: ["src/math-owner.js"],
      imports: [],
      css: ["assets/math.css"],
    },
  };
  const declarations = [{ kind: "startup", importer: "src/bootstrap.js", source: "src/main.js" }];
  for (const [group, policy] of Object.entries(LAZY_BUDGETS)) {
    if (policy.kind === "css")
      declarations.push({
        kind: "lazy",
        group,
        importer: "src/math-owner.js",
        source: "src/math.css",
      });
    else {
      files[`assets/${group}.js`] = { modules: [`src/${group}.js`], imports: [] };
      declarations.push({
        kind: "lazy",
        group,
        importer: "src/main.js",
        source: `src/${group}.js`,
      });
    }
  }
  for (const name of Object.keys(files)) writeFileSync(join(dist, name), "");
  writeFileSync(join(dist, "index.html"), index);
  const graph = {
    files,
    declarations,
    cssImporters: { "src/math.css": ["src/math-owner.js"] },
    cssContains: {},
  };
  return {
    root,
    dist,
    assets,
    graph,
    run() {
      const manifest = {
        document: artifactDocument(readFileSync(join(dist, "index.html"), "utf8")),
        declarations,
        cssImporters: graph.cssImporters,
        cssContains: graph.cssContains,
        files: Object.fromEntries(
          Object.entries(files)
            .filter(([name]) => existsSync(join(dist, name)))
            .map(([name, metadata]) => {
              const bytes = readFileSync(join(dist, name));
              return [
                name,
                {
                  path: name,
                  bytes: bytes.length,
                  sha256: hash(bytes),
                  modulePackages: Object.fromEntries(
                    (metadata.modules ?? []).map((source) => [source, "fixture"]),
                  ),
                  ...metadata,
                },
              ];
            }),
        ),
      };
      mkdirSync(dirname(join(dist, ARTIFACT_GRAPH)), { recursive: true });
      writeFileSync(join(dist, ARTIFACT_GRAPH), JSON.stringify(manifest));
      const result = spawnSync(process.execPath, [script], { cwd: root, encoding: "utf8" });
      assert.ifError(result.error);
      return result;
    },
    size(name, bytes) {
      writeFileSync(join(assets, name), Buffer.alloc(bytes));
    },
    index(html) {
      writeFileSync(join(dist, "index.html"), html);
    },
  };
}

for (const [label, asset, limit] of [
  ["JS", "entry.js", 2_785_000],
  ["CSS", "entry.css", 147_000],
  ["math stylesheet", "math.css", 100_000],
  ["settings panes", "settings.js", 200_000],
  ["workspace views", "workspace.js", 250_000],
  ["syntax highlighting", "syntax-highlighting.js", 3_000_000],
  ["diagram rendering", "diagram-rendering.js", 3_000_000],
]) {
  test(`${label}: the exact budget passes and one extra byte fails independently`, (t) => {
    const output = fixture(t);
    output.size(asset, limit);
    const equal = output.run();
    assert.equal(equal.status, 0, equal.stdout + equal.stderr);
    assert.doesNotMatch(equal.stdout, /FAIL/);
    output.size(asset, limit + 1);
    const exceeded = output.run();
    assert.equal(exceeded.status, 1, exceeded.stdout + exceeded.stderr);
    assert.match(exceeded.stdout, new RegExp(`${label}[^\\n]*FAIL`));
    assert.match(exceeded.stderr, /FAIL/);
  });
}

test("a missing lazy chunk fails even when every byte budget passes", (t) => {
  const output = fixture(t);
  unlinkSync(join(output.assets, "diagram-rendering.js"));
  const result = output.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /diagram rendering: no chunk found/);
});

test("eager math styles fail even below their size budget", (t) => {
  const output = fixture(t);
  output.index(index + '<link rel="prefetch" href="/assets/math.css">');
  const result = output.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /math stylesheet: index.html references/);
});

test("an HTML preload cannot bypass a lazy source boundary", (t) => {
  const output = fixture(t);
  output.index(index + '<link rel="preload" as="script" href="/assets/settings.js">');
  const result = output.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(
    result.stderr,
    /settings panes: declared source is on the startup or HTML request path/,
  );
});

test("import-map declarations and inert template tags are not startup requests", (t) => {
  const output = fixture(t);
  output.graph.files["assets/address-only.js"] = { modules: ["npm:unused/index.js"], imports: [] };
  output.size("address-only.js", 2_785_001);
  output.index(
    index +
      '<script type="importmap">{"imports":{"unused":"/assets/address-only.js"}}</script><template><script src="/assets/address-only.js"></script></template>',
  );
  const declared = output.run();
  assert.equal(declared.status, 0, declared.stdout + declared.stderr);
  output.graph.files["assets/main.js"].imports.push("assets/address-only.js");
  const imported = output.run();
  assert.equal(imported.status, 1, imported.stdout + imported.stderr);
  assert.match(imported.stdout, /JS[^\n]*FAIL/);
});

test("both public bootstrap scripts and the immediate main closure count once", (t) => {
  const output = fixture(t);
  writeFileSync(join(output.dist, "appearance.generated.js"), Buffer.alloc(10));
  writeFileSync(join(output.dist, "focus-modality.js"), Buffer.alloc(11));
  output.size("main.js", 2_785_000 - 21);
  output.index(
    index +
      '<script type="module" src="/assets/entry.js"></script><link rel="modulepreload" href="/assets/entry.js">',
  );
  const equal = output.run();
  assert.equal(equal.status, 0, equal.stdout + equal.stderr);
  output.size("main.js", 2_785_000 - 20);
  const exceeded = output.run();
  assert.equal(exceeded.status, 1, exceeded.stdout + exceeded.stderr);
});

test("an ordinary dynamic edge is not silently promoted to startup", (t) => {
  const output = fixture(t);
  output.graph.files["assets/on-demand.js"] = { modules: ["src/on-demand.js"], imports: [] };
  output.size("on-demand.js", 2_785_001);
  output.graph.files["assets/main.js"].dynamicImports = ["assets/on-demand.js"];
  const result = output.run();
  assert.equal(result.status, 0, result.stdout + result.stderr);
});

test("a removed pure-CSS mapping fails instead of reporting a zero-byte lazy stylesheet", (t) => {
  const output = fixture(t);
  output.graph.files["assets/math-owner.js"].css = [];
  const result = output.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /math stylesheet: no emitted CSS mapping/);
});

test("CSS source importer provenance rejects an eager chain even while owner CSS remains", (t) => {
  const output = fixture(t);
  output.graph.cssImporters["src/math.css"].push("src/intermediate.css");
  output.graph.cssImporters["src/intermediate.css"] = ["src/main.js"];
  const result = output.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(
    result.stderr,
    /math stylesheet: CSS source src\/intermediate.css has startup importer src\/main.js/,
  );
});

test("missing CSS source provenance cannot borrow another stylesheet's owner mapping", (t) => {
  const output = fixture(t);
  delete output.graph.cssImporters["src/math.css"];
  const result = output.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /math stylesheet: no CSS source provenance/);
});

test("lazy closures include dependencies with unrelated filenames and deduplicate shared chunks", (t) => {
  const output = fixture(t);
  output.graph.files["assets/unrelated.js"] = { modules: ["src/support.js"], imports: [] };
  output.size("unrelated.js", 200_000);
  output.graph.files["assets/settings.js"].imports.push(
    "assets/unrelated.js",
    "assets/unrelated.js",
  );
  const equal = output.run();
  assert.equal(equal.status, 0, equal.stdout + equal.stderr);
  output.size("unrelated.js", 200_001);
  const exceeded = output.run();
  assert.equal(exceeded.status, 1, exceeded.stdout + exceeded.stderr);
  assert.match(exceeded.stdout, /settings panes[^\n]*FAIL/);
});

test("a declared lazy source merged into any startup chunk fails independently of names", (t) => {
  const output = fixture(t);
  output.graph.files["assets/main.js"].modules.push("src/settings.js");
  const result = output.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /settings panes: declared source is on the startup/);
});

test("bootstrap-only source ownership rejects heavy packages before the gate", (t) => {
  const output = fixture(t);
  output.graph.files["assets/entry.js"].modules.push("npm:react/index.js");
  output.graph.files["assets/entry.js"].modulePackages = {
    "src/bootstrap.js": "fixture",
    "npm:react/index.js": "react",
  };
  const result = output.run();
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, /bootstrap-only graph contains react/);
});

async function builtFixture(
  t,
  {
    eagerCss = false,
    eagerHelper = false,
    heavyBootstrap = false,
    heavySdk = false,
    cssImport = null,
    cssQuery = null,
    lazyCssImport = false,
  } = {},
) {
  const root = mkdtempSync(join(tmpdir(), "flame artifact graph "));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const write = (name, source) => {
    const target = join(root, name);
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, source);
  };
  write("package.json", '{"type":"module","name":"artifact-fixture"}');
  write(
    "index.html",
    '<script src="/appearance.generated.js"></script><script src="/focus-modality.js"></script><script type="module" src="/src/bootstrap.js"></script>',
  );
  write("public/appearance.generated.js", 'globalThis.fixtureAppearance = "light";');
  write("public/focus-modality.js", 'globalThis.fixtureModality = "keyboard";');
  write("public/address-only.js", 'export const unused = "not requested";');
  write(
    "node_modules/react/package.json",
    '{"type":"module","name":"react","exports":"./index.js"}',
  );
  write("node_modules/react/index.js", 'export const shared = { identity: Symbol("shared") };');
  write(
    "node_modules/@flame/component-sdk/package.json",
    '{"type":"module","name":"@flame/component-sdk","exports":{".":"./index.js","./browser-policy":"./browser-policy.js"}}',
  );
  write(
    "node_modules/@flame/component-sdk/browser-policy.js",
    "export const supported = Object.freeze({ compatible: true });",
  );
  write(
    "node_modules/@flame/component-sdk/index.js",
    'export const sdkRoot = { identity: Symbol("sdk root") };',
  );
  write(
    "src/bootstrap.js",
    `import {supported} from "@flame/component-sdk/browser-policy"; globalThis.policy = supported; ${heavyBootstrap ? 'import {shared} from "react"; globalThis.early = shared;' : ""} ${heavySdk ? 'import {sdkRoot} from "@flame/component-sdk"; globalThis.earlySdk = sdkRoot;' : ""} globalThis.application = import(/* @flame-startup */ "./main.js");`,
  );
  write(
    "src/main.js",
    `import {shared} from "react"; ${eagerCss ? 'import "./math.css";' : ""} ${eagerHelper ? 'import {support} from "./support.js";globalThis.earlySupport=support;' : ""}
${cssImport ? 'import "./eager-containing.css";' : ""}
${cssQuery ? `import projection from "./eager-containing.css?${cssQuery}";globalThis.projectedCSS = projection;` : ""}
globalThis.shared = shared;
globalThis.features = [
()=>import(/* @flame-lazy:settings */ "./settings.js"),
()=>import(/* @flame-lazy:workspace */ "./workspace.js"),
()=>import(/* @flame-lazy:syntax-highlighting */ "./syntax-entry.js"),
()=>import(/* @flame-lazy:diagram-rendering */ "./diagram.js"),
()=>import("./math-owner.js")];`,
  );
  write(
    "src/settings.js",
    'import {shared} from "react"; import /* @flame-lazy:settings */ {support} from "./support.js";export const settings = shared + support;',
  );
  write("src/support.js", 'export const support = { identity: Symbol("captured helper") };');
  for (const name of ["workspace", "syntax", "diagram"])
    write(
      `src/${name}.js`,
      `import {shared} from "react"; ${name === "workspace" && lazyCssImport ? 'import "./lazy-containing.css";' : ""} export const ${name} = shared + ${JSON.stringify(name)};`,
    );
  write("src/syntax-entry.js", 'export {syntax} from "./syntax.js";');
  write(
    "src/math-owner.js",
    'import /* @flame-lazy:math-styles */ "./math.css"; import "./additional.css"; export const math = "retained";',
  );
  write("src/math.css", ".math-proof { color: #123456; }");
  write("src/additional.css", ".additional-proof { color: #654321; }");
  write(
    "src/eager-containing.css",
    `@import "./${cssImport === "nested" ? "intermediate.css" : "math.css"}"; .eager-proof { color: #345678; }`,
  );
  write("src/intermediate.css", '@import "./math.css"; .intermediate-proof { color: #456789; }');
  write("src/lazy-containing.css", '@import "./math.css"; .lazy-proof { color: #567890; }');
  await build({
    root,
    configFile: false,
    logLevel: "silent",
    plugins: [
      artifactGraph(),
      {
        name: "fixture-importmap",
        transformIndexHtml: {
          order: "post",
          handler: () => [
            {
              tag: "script",
              attrs: { type: "importmap" },
              children: '{"imports":{"address-only":"/address-only.js"}}',
              injectTo: "head-prepend",
            },
          ],
        },
      },
    ],
    build: { outDir: "dist", minify: false, target: "esnext" },
  });
  const directory = join(root, "dist");
  const graph = readArtifactGraph(directory);
  return { directory, graph, result: analyzeArtifactGraph(graph) };
}

test("real Vite output maps static lazy CSS through its owner and keeps shared React legal", async (t) => {
  const { directory, graph, result } = await builtFixture(t);
  assert.deepEqual(result.errors, []);
  assert(
    result.bootstrap.js.some((path) => graph.files[path].modules?.includes(graph.browserPolicy)),
  );
  assert(!JSON.stringify(graph).includes(dirname(directory)));
  const declaration = graph.declarations.find((entry) => entry.group === "math-styles");
  assert(graph.cssImporters[declaration.source].includes(declaration.importer));
  const owner = Object.values(graph.files).find((file) =>
    file.modules?.includes(declaration.importer),
  );
  const math = result.features.find((feature) => feature.group === "math-styles");
  assert(owner.css.length > 0);
  assert.deepEqual(math.roots.toSorted(), owner.css.toSorted());
  assert.equal(
    math.used,
    owner.css.reduce((sum, path) => sum + graph.files[path].bytes, 0),
  );
  const stylesheet = owner.css
    .map((path) => readFileSync(join(directory, path), "utf8"))
    .join("\n");
  assert.match(stylesheet, /math-proof/);
  assert.match(stylesheet, /additional-proof/);
  assert(!result.startup.js.includes(graph.document.imports["address-only"]));
  assert(result.features.find((feature) => feature.group === "settings").sharedBytes > 0);
});

test("real Vite eager CSS sharing cannot erase the lazy stylesheet guard", async (t) => {
  const { graph, result } = await builtFixture(t, { eagerCss: true });
  const declaration = graph.declarations.find((entry) => entry.group === "math-styles");
  assert(graph.cssImporters[declaration.source].includes("src/main.js"));
  const owner = Object.values(graph.files).find((file) =>
    file.modules?.includes(declaration.importer),
  );
  assert(owner.css.length > 0);
  assert(
    result.errors.some(
      (error) =>
        error.includes("math stylesheet:") &&
        (error.includes("startup") || error.includes("no emitted CSS mapping")),
    ),
    result.errors.join("\n"),
  );
});

for (const cssImport of ["direct", "nested"]) {
  test(`real Vite ${cssImport} CSS @import containment rejects an eager owner`, async (t) => {
    const { graph, result } = await builtFixture(t, { cssImport });
    const declaration = graph.declarations.find((entry) => entry.group === "math-styles");
    assert(graph.cssContains["src/eager-containing.css"].includes(declaration.source));
    assert(graph.cssImporters["src/eager-containing.css"].includes("src/main.js"));
    assert(
      result.errors.some(
        (error) =>
          error.includes("math stylesheet:") && error.includes("startup importer src/main.js"),
      ),
      result.errors.join("\n"),
    );
  });
}

test("real Vite CSS @import containment stays valid under another lazy owner", async (t) => {
  const { graph, result } = await builtFixture(t, { lazyCssImport: true });
  const declaration = graph.declarations.find((entry) => entry.group === "math-styles");
  assert(graph.cssContains["src/lazy-containing.css"].includes(declaration.source));
  assert(graph.cssImporters["src/lazy-containing.css"].includes("src/workspace.js"));
  assert.deepEqual(result.errors, []);
});

for (const cssQuery of ["raw", "url", "inline"]) {
  test(`real Vite CSS ?${cssQuery} is a JS projection rather than an applied stylesheet owner`, async (t) => {
    const { graph, result } = await builtFixture(t, { cssQuery });
    assert(
      !Object.keys(graph.cssContains).some((source) =>
        source.startsWith("src/eager-containing.css"),
      ),
    );
    assert.deepEqual(result.errors, []);
  });
}

test("stylesheet request classification excludes Vite's non-style query projections", () => {
  assert(isStylesheetRequest("/src/style.css"));
  assert(isStylesheetRequest("/src/style.css?used"));
  for (const query of [
    "raw",
    "url",
    "inline",
    "transform-only",
    "worker",
    "sharedworker",
    "commonjs-proxy",
  ])
    assert(!isStylesheetRequest(`/src/style.css?${query}`));
});

test("a source-declared static helper cannot become eager behind a still-lazy entry", async (t) => {
  const { result } = await builtFixture(t, { eagerHelper: true });
  assert(
    result.errors.some(
      (error) => error.includes("settings panes:") && error.includes("src/support.js"),
    ),
    result.errors.join("\n"),
  );
});

test("real chunk membership catches heavy bootstrap sharing", async (t) => {
  const { result } = await builtFixture(t, { heavyBootstrap: true });
  assert(
    result.errors.some((error) => error.includes("bootstrap-only graph contains react")),
    result.errors.join("\n"),
  );
});

test("the pure public browser policy does not exempt other SDK sources", async (t) => {
  const { result } = await builtFixture(t, { heavySdk: true });
  assert(
    result.errors.some(
      (error) =>
        error.includes("bootstrap-only graph contains @flame/component-sdk") &&
        error.includes("index.js"),
    ),
    result.errors.join("\n"),
  );
});
