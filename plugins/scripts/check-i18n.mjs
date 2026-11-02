#!/usr/bin/env node
// Plugin catalog guard — every migrated plugin carries every key in every
// locale, with matching interpolation placeholders, and references only keys it
// declares. Mirrors apps/web/scripts/check-locales.mjs for the plugin tree,
// which that script never walks.
import { parseSync } from "rolldown/utils";
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root =
  process.argv[2] ?? dirname(dirname(fileURLToPath(import.meta.url)));
const LOCALES = ["en", "zh", "zh-TW", "ja", "ko", "es", "fr", "de"];
const PLACEHOLDER_PATTERN = /\{\{(\w+)\}\}/g;
const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/;
const prose = /^[A-Z][a-z]+(?: [A-Za-z]+)+[.!?]?$/u;
const translatedAttributes = new Set([
  "title",
  "label",
  "placeholder",
  "sub",
  "description",
  "aria-label",
  "ariaLabel",
  "clearLabel",
  "closeLabel",
]);

function syntax(file) {
  const result = parseSync(file, readFileSync(file, "utf8"), {
    lang: file.endsWith(".tsx") ? "tsx" : "ts",
  });
  if (result.errors.length)
    throw new Error(`invalid component syntax: ${file}`, {
      cause: result.errors,
    });
  return result.program;
}

function walk(node, visit) {
  if (node === null || typeof node !== "object") return;
  if (node.type) visit(node);
  for (const child of Object.values(node))
    if (child !== null && typeof child === "object") walk(child, visit);
}

function* componentSources(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      if (
        !["locales", "node_modules", "dist", "__tests__", "fixtures"].includes(
          entry.name,
        )
      )
        yield* componentSources(path);
    } else if (
      /\.tsx?$/u.test(entry.name) &&
      !/\.(?:test|spec|generated|d)\./u.test(entry.name)
    )
      yield path;
  }
}
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
  walk(syntax(file), (node) => {
    if (
      node.type !== "Property" ||
      node.value.type !== "Literal" ||
      typeof node.value.value !== "string"
    )
      return;
    const key = node.key.name ?? node.key.value;
    if (map.has(key)) failures.push(`${label}: "${key}" declared twice`);
    map.set(key, node.value.value);
  });
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
  for (const file of componentSources(compDir)) {
    walk(syntax(file), (node) => {
      if (
        node.type === "CallExpression" &&
        ((node.callee.type === "Identifier" && node.callee.name === "t") ||
          (node.callee.type === "MemberExpression" &&
            !node.callee.computed &&
            node.callee.property.name === "t"))
      ) {
        const key = node.arguments[0]?.value;
        if (typeof key === "string" && !enKeys.has(key) && !enFamilies.has(key))
          failures.push(`${file}: t("${key}") has no key in en`);
      }
      if (node.type === "JSXText" && prose.test(node.value.trim()))
        failures.push(
          `${file}: hardcoded UI text "${node.value.trim()}" — route through t()`,
        );
      if (
        node.type === "JSXAttribute" &&
        translatedAttributes.has(node.name.name) &&
        node.value?.type === "Literal" &&
        prose.test(node.value.value)
      )
        failures.push(
          `${file}: hardcoded UI prop ${node.name.name}="${node.value.value}" — route through t()`,
        );
    });
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
