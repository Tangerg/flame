#!/usr/bin/env node
// Plugin catalog guard — every migrated plugin carries every key in every
// locale, with matching interpolation placeholders, and references only keys it
// declares. Mirrors apps/web/scripts/check-locales.mjs for the plugin tree,
// which that script never walks.
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const LOCALES = ["en", "zh", "zh-TW", "ja", "ko", "es", "fr", "de"];
const PAIR_PATTERN =
  /(?:"([^"]+)"|([A-Za-z_$][\w$]*))\s*:\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')/g;
const PLACEHOLDER_PATTERN = /\{\{(\w+)\}\}/g;
const LITERAL_KEY_PATTERN = /\bt\(\s*"([^"]+)"/g;
const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/;

const withoutComments = (s) =>
  s.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/[^\n]*/g, "");
const pluralBase = (key) => {
  const m = PLURAL_SUFFIX.exec(key);
  return m ? key.slice(0, -m[0].length) : null;
};
const pluralForms = (locale) =>
  new Intl.PluralRules(locale).resolvedOptions().pluralCategories;
const placeholders = (value) =>
  [...value.matchAll(PLACEHOLDER_PATTERN)]
    .map((m) => m[1])
    .sort()
    .join(",");

const failures = [];

function entries(file, label) {
  const map = new Map();
  for (const m of readFileSync(file, "utf8").matchAll(PAIR_PATTERN)) {
    const key = m[1] ?? m[2];
    if (map.has(key)) failures.push(`${label}: "${key}" declared twice`);
    map.set(key, m[3] ?? m[4] ?? "");
  }
  return map;
}

const plugins = readdirSync(root, { withFileTypes: true })
  .filter(
    (d) =>
      d.isDirectory() &&
      existsSync(join(root, d.name, "component", "locales", "en.ts")),
  )
  .map((d) => d.name)
  .sort();

for (const plugin of plugins) {
  const dir = join(root, plugin, "component", "locales");
  const present = readdirSync(dir)
    .filter((n) => n.endsWith(".ts"))
    .map((n) => n.replace(/\.ts$/, ""));
  for (const loc of LOCALES)
    if (!present.includes(loc))
      failures.push(`${plugin}: missing locale ${loc}.ts`);
  for (const loc of present)
    if (!LOCALES.includes(loc))
      failures.push(`${plugin}: unexpected locale ${loc}.ts`);

  const en = entries(join(dir, "en.ts"), `${plugin}/en`);
  const enKeys = new Set(en.keys());
  const enFamilies = new Set();
  for (const k of enKeys) {
    const base = pluralBase(k);
    if (base !== null) enFamilies.add(base);
  }

  for (const loc of LOCALES) {
    if (loc === "en" || !existsSync(join(dir, `${loc}.ts`))) continue;
    const cat = entries(join(dir, `${loc}.ts`), `${plugin}/${loc}`);
    const catKeys = new Set(cat.keys());
    const need = pluralForms(loc);

    for (const k of catKeys) {
      if (enKeys.has(k)) continue;
      const base = pluralBase(k);
      if (base !== null && enFamilies.has(base)) continue;
      failures.push(`${plugin}/${loc}: "${k}" is not in en`);
    }
    for (const k of enKeys) {
      if (pluralBase(k) !== null) continue;
      if (!catKeys.has(k)) failures.push(`${plugin}/${loc}: missing "${k}"`);
    }
    for (const base of enFamilies)
      for (const form of need)
        if (!catKeys.has(`${base}_${form}`))
          failures.push(`${plugin}/${loc}: missing plural "${base}_${form}"`);

    for (const [k, v] of cat) {
      const base = pluralBase(k);
      const enVal =
        en.get(k) ?? (base !== null ? en.get(`${base}_other`) : undefined);
      if (enVal === undefined) continue;
      if (placeholders(v) !== placeholders(enVal))
        failures.push(`${plugin}/${loc}: "${k}" placeholders differ from en`);
    }
  }

  const compDir = join(root, plugin, "component");
  for (const f of readdirSync(compDir).filter(
    (n) => /\.(ts|tsx)$/.test(n) && !/\.(generated|d)\./.test(n),
  )) {
    const code = withoutComments(readFileSync(join(compDir, f), "utf8"));
    for (const m of code.matchAll(LITERAL_KEY_PATTERN)) {
      if (enKeys.has(m[1]) || enFamilies.has(m[1])) continue;
      failures.push(`${plugin}/${f}: t("${m[1]}") has no key in en`);
    }
  }
}

if (failures.length) {
  console.error(
    "check-i18n: plugin catalog problems\n" +
      failures.map((f) => `  - ${f}`).join("\n"),
  );
  process.exit(1);
}
console.log(
  `check-i18n: ${plugins.length} plugin catalog(s) complete across ${LOCALES.length} locales`,
);
