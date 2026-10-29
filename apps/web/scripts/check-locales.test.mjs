import assert from "node:assert/strict";
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const guard = fileURLToPath(new URL("./check-locales.mjs", import.meta.url));
const dependencies = fileURLToPath(new URL("../../../node_modules", import.meta.url));
const baseKeys = [
  "tools.family.alpha",
  "tools.family.beta",
  "tool.doing.read",
  "tool.doing.write",
  "tool.doing.generic",
  "tool.done.read",
  "tool.done.write",
  "tool.done.generic",
  "rpcError.unknown",
  "rpcError.denied",
];

function fixture(t, prefix = "flame-locale-") {
  const root = mkdtempSync(join(tmpdir(), prefix));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const script = join(root, "scripts/check-locales.mjs");
  mkdirSync(dirname(script), { recursive: true });
  copyFileSync(guard, script);
  symlinkSync(
    dependencies,
    join(root, "node_modules"),
    process.platform === "win32" ? "junction" : "dir",
  );
  function source(path, content) {
    const destination = join(root, "src", path);
    mkdirSync(dirname(destination), { recursive: true });
    writeFileSync(destination, content);
  }
  function catalog(locale, keys = baseKeys, values = {}) {
    source(
      `lib/i18n/locales/${locale}.ts`,
      `export const catalog = {\n${keys.map((key) => `  "${key}": "${values[key] ?? "Value"}",`).join("\n")}\n};\n`,
    );
  }
  catalog("en");
  catalog("fr");
  source("consumer.ts", `export const named = ${JSON.stringify(baseKeys)};`);
  source(
    "lib/toolFamilies.ts",
    'export const TOOL_FAMILIES = [{id: "alpha", verbs: [{name: "read"}]}, {id: "beta", verbs: [{name: "write"}]}];',
  );
  source("lib/rpcErrors.ts", 'export const MAPPED_TYPES = ["unknown", "denied"];');
  source(
    "features/agent/presentation/toolPresentation.ts",
    'export const GENERIC_VERB_ID = "generic";',
  );
  source("features/workspace/views.ts", "export const views = [];");
  return {
    source,
    catalog,
    run() {
      const result = spawnSync(process.execPath, [script], { cwd: root, encoding: "utf8" });
      assert.ifError(result.error);
      return result;
    },
  };
}

function passes(result) {
  assert.equal(result.status, 0, result.stdout + result.stderr);
}
function fails(result, diagnostic) {
  assert.equal(result.status, 1, result.stdout + result.stderr);
  assert.match(result.stderr, diagnostic);
}

test("a real source tree and its named catalogs pass", (t) => passes(fixture(t).run()));

test("catalog declarations cannot prove a singular key has a consumer", (t) => {
  const tree = fixture(t);
  for (const locale of ["en", "fr"]) tree.catalog(locale, [...baseKeys, "fixture.unused"]);
  fails(tree.run(), /nothing names.*fixture\.unused/);
});

for (const path of [
  "ui/Lookup.tsx",
  "features/example/ui/Lookup.tsx",
  "features/example/detail/ui/Lookup.tsx",
]) {
  test(`rendered lookup copy in ${path} fails`, (t) => {
    const tree = fixture(t);
    tree.source(
      path,
      'const labels = {title: "Visible heading"}; export function Lookup() { return <h2>{labels.title}</h2>; }',
    );
    fails(tree.run(), /copy in a lookup table/);
  });
}

test("proper identities and StyleX declarations are not rendered lookup copy", (t) => {
  const tree = fixture(t);
  tree.source(
    "features/example/ui/Style.tsx",
    `
  import * as stylex from "@stylexjs/stylex";
  const styles = stylex.create({ root: { transitionProperty: "opacity, scale, translate" } });
  const names = new Map([
    ["google", { name: "Google" }],
    ["openai", { name: "OpenAI" }],
  ]);
  export function View() { return <div {...stylex.props(styles.root)}>{names.get("google").name}</div>; }
 `,
  );
  passes(tree.run());
});

test("the script resolves a source root containing spaces and percent signs", (t) => {
  passes(fixture(t, "flame locale % root ").run());
});

test("root and nested source consumers share one relative path convention", (t) => {
  const tree = fixture(t);
  for (const locale of ["en", "fr"])
    tree.catalog(locale, [...baseKeys, "fixture.root", "fixture.nested"]);
  tree.source("index.ts", 'export const root = "fixture.root";');
  tree.source("features/example/domain/consumer.ts", 'export const nested = "fixture.nested";');
  passes(tree.run());
});

test("plural families retain each language's own forms", (t) => {
  const tree = fixture(t);
  tree.catalog("en", [...baseKeys, "fixture.items_one", "fixture.items_other"]);
  tree.catalog("fr", [
    ...baseKeys,
    "fixture.items_one",
    "fixture.items_many",
    "fixture.items_other",
  ]);
  tree.source(
    "items.ts",
    'declare const t: (key: string, options: unknown) => string; export const items = t("fixture.items", {count: 2});',
  );
  passes(tree.run());
  tree.catalog("fr", [...baseKeys, "fixture.items_one", "fixture.items_other"]);
  fails(tree.run(), /fixture\.items.*needs/);
});

for (const [label, path, code, diagnostic] of [
  [
    "literal notifications",
    "features/example/ui/Notice.tsx",
    'notifyInfo("Action finished");',
    /literal copy in notifyInfo/,
  ],
  [
    "unknown translation keys",
    "unknown.ts",
    't("fixture.unknown");',
    /fixture\.unknown.*no catalog has/,
  ],
  [
    "domain copy",
    "features/example/domain/value.ts",
    'export const value = "Visible heading";',
    /prose in a copy-free ring/,
  ],
  [
    "application copy",
    "features/example/application/value.ts",
    'export const value = "Visible heading";',
    /a use case wrote copy/,
  ],
  [
    "registered translated values",
    "registration.ts",
    'commands.register({label: t("tools.family.alpha")});',
    /contribution resolves copy/,
  ],
  [
    "translated contribution factories",
    "OwnerContributions.ts",
    'export function specs() { return [{label: t("tools.family.alpha")}]; }',
    /spec factory resolves copy/,
  ],
  [
    "host locale formatting",
    "format.ts",
    "export const value = new Date().toLocaleString();",
    /does not format the way lib\/i18n does/,
  ],
  [
    "literal JSX text",
    "features/example/ui/Words.tsx",
    "export function Words() { return <div>Visible heading</div>; }",
    /literal copy in a component/,
  ],
  [
    "literal JSX props",
    "features/example/ui/Prop.tsx",
    'export function Prop() { return <input placeholder="Visible heading" />; }',
    /placeholder=.*is copy/,
  ],
  [
    "single quoted lookup values",
    "features/example/ui/Quoted.tsx",
    "const labels = {title: 'Visible heading'}; export function Quoted() { return <div>{labels.title}</div>; }",
    /copy in a lookup table/,
  ],
  [
    "asserted lookup values",
    "features/example/ui/Asserted.tsx",
    'const labels = {title: "Visible heading" as const}; export function Asserted() { return <div>{labels.title}</div>; }',
    /copy in a lookup table/,
  ],
]) {
  test(`${label} fails in an actual source file`, (t) => {
    const tree = fixture(t);
    tree.source(path, code);
    fails(tree.run(), diagnostic);
  });
}

test("catalog duplicates, missing entries and extras remain independent failures", (t) => {
  const tree = fixture(t);
  tree.catalog("fr", [...baseKeys, baseKeys[0]]);
  fails(tree.run(), /declared twice/);
  tree.catalog("fr", baseKeys.slice(1));
  fails(tree.run(), /key\(s\) missing/);
  tree.catalog("fr", [...baseKeys, "fixture.extra"]);
  fails(tree.run(), /key\(s\) absent from en/);
});

test("catalog value and placeholder rules remain enforced", (t) => {
  const tree = fixture(t);
  tree.catalog("en", baseKeys, { [baseKeys[0]]: "{{count}} values" });
  fails(tree.run(), /carries.*where en carries/);
  tree.catalog("en", baseKeys, { [baseKeys[0]]: "Working..." });
  fails(tree.run(), /writes.*use/);
  tree.catalog("en", baseKeys, { [baseKeys[0]]: "task(s)" });
  fails(tree.run(), /spells a plural with punctuation/);
});

test("English titles and the shared workspace identity retain their rules", (t) => {
  const tree = fixture(t);
  const keys = [...baseKeys, "demo.title", "workspace.view.title.demo"];
  for (const locale of ["en", "fr"])
    tree.catalog(locale, keys, { "demo.title": "Visible Heading" });
  tree.source("titles.ts", 'export const keys = ["demo.title", "workspace.view.title.demo"];');
  fails(tree.run(), /is Title Case/);
  for (const locale of ["en", "fr"])
    tree.catalog(locale, keys, { "demo.title": "Header", "workspace.view.title.demo": "Tab" });
  tree.source(
    "features/workspace/views.ts",
    'export const views = [{\n id: "demo",\n title: "workspace.view.title.demo",\n}];',
  );
  fails(tree.run(), /view "demo" is called two things/);
});

test("dynamic table families still reject missing and unproduced catalog keys", (t) => {
  const tree = fixture(t);
  tree.source(
    "lib/toolFamilies.ts",
    'export const TOOL_FAMILIES = [{id: "alpha", verbs: [{name: "read"}]}, {id: "beta", verbs: [{name: "write"}]}, {id: "gamma"}];',
  );
  fails(tree.run(), /table produces that no catalog carries.*tools\.family\.gamma/);
  tree.source(
    "lib/toolFamilies.ts",
    'export const TOOL_FAMILIES = [{id: "alpha", verbs: [{name: "read"}]}, {id: "beta", verbs: [{name: "write"}]}];',
  );
  for (const locale of ["en", "fr"]) tree.catalog(locale, [...baseKeys, "tools.family.retired"]);
  tree.source("retired.ts", 'export const retired = "tools.family.retired";');
  fails(tree.run(), /under this prefix that nothing produces.*tools\.family\.retired/);
});

test("aliased StyleX factories remain declarations and comments never become copy", (t) => {
  const tree = fixture(t);
  tree.source(
    "features/example/ui/Aliased.tsx",
    `
    import {create as makeStyles} from "@stylexjs/stylex";
    const styles = makeStyles({root: {transitionProperty: "opacity, scale, translate"}});
    // const labels = {title: "Visible heading"};
    export function Aliased() { return <div>{styles.root}</div>; }
  `,
  );
  passes(tree.run());
});
