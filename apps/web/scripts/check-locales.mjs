#!/usr/bin/env node
import { readFileSync, readdirSync } from "node:fs";
import { parseTree } from "jsonc-parser";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const LOCALES_DIR = fileURLToPath(new URL("../src/lib/i18n/locales/", import.meta.url));
const PLUGINS_DIR = fileURLToPath(new URL("../../../plugins/", import.meta.url));
const KEY_PATTERN = /^\s*"([^"]+)":/gm;
const PAIR_PATTERN = /"([^"]+)":\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')/g;
const PLACEHOLDER_PATTERN = /\{\{(\w+)\}\}/g;

const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/;

function pluralBase(key) {
  const match = PLURAL_SUFFIX.exec(key);
  return match ? key.slice(0, -match[0].length) : null;
}

/** The forms a locale's own plural rules require. `en` two, `fr` three, `zh` one. */
function pluralForms(locale) {
  return new Intl.PluralRules(locale).resolvedOptions().pluralCategories;
}

/** Every plural family in a catalog, as base → the forms it declares. */
function pluralFamilies(keys) {
  const families = new Map();
  for (const key of keys) {
    const base = pluralBase(key);
    if (base === null) continue;
    families.set(base, [...(families.get(base) ?? []), key.slice(base.length + 1)]);
  }
  return families;
}

const duplicates = [];

function keysOf(file) {
  const source = readFileSync(file, "utf8");
  const seen = new Set();
  let keys;
  if (file.endsWith(".json")) {
    const errors = [];
    const tree = parseTree(source, errors, { disallowComments: true, allowTrailingComma: false });
    if (
      errors.length ||
      tree?.type !== "object" ||
      !tree.children?.length ||
      tree.children.some((property) => property.children?.[1]?.type !== "string")
    ) {
      throw new Error(`${file}: locale must be a nonempty JSON object of strings`);
    }
    keys = tree.children.map((property) => property.children[0].value);
  } else {
    keys = [...source.matchAll(KEY_PATTERN)].map((match) => match[1]);
  }
  for (const key of keys) {
    if (seen.has(key)) duplicates.push(`${file}: "${key}" declared twice`);
    seen.add(key);
  }
  return seen;
}

function valuesOf(file) {
  const source = readFileSync(file, "utf8");
  if (file.endsWith(".json")) return new Map(Object.entries(JSON.parse(source)));
  const out = new Map();
  for (const match of source.matchAll(PAIR_PATTERN)) {
    out.set(match[1], match[2] ?? match[3] ?? "");
  }
  return out;
}

function placeholders(value) {
  return [...value.matchAll(PLACEHOLDER_PATTERN)].map((match) => match[1]).sort();
}

const catalogFiles = new Map([["en", join(LOCALES_DIR, "en.ts")]]);
for (const directory of readdirSync(PLUGINS_DIR, { withFileTypes: true })) {
  if (!directory.isDirectory()) continue;
  const root = join(PLUGINS_DIR, directory.name, "package");
  let manifest;
  try {
    manifest = JSON.parse(readFileSync(join(root, "plugin.json"), "utf8"));
  } catch (error) {
    if (error.code === "ENOENT") continue;
    throw error;
  }
  const reference = manifest.extensions?.["io.github.tangerg.flame"]?.declaration;
  if (!reference) continue;
  const declaration = JSON.parse(readFileSync(join(root, reference), "utf8"));
  for (const locale of declaration.contributes?.locales ?? []) {
    if (catalogFiles.has(locale.tag)) throw new Error(`duplicate first-party locale ${locale.tag}`);
    catalogFiles.set(locale.tag, join(root, locale.entry));
  }
}
const catalogs = new Map([...catalogFiles].map(([tag, file]) => [tag, keysOf(file)]));

const en = catalogs.get("en");
if (!en) {
  console.error("check-locales: en.ts is the reference catalog and is missing");
  process.exit(1);
}

const failures = [...duplicates];

function note(locale, keys, label) {
  if (keys.length === 0) return;
  const sample = keys.slice(0, 8).join(", ");
  const rest = keys.length > 8 ? `, … (+${keys.length - 8} more)` : "";
  failures.push(`${locale}: ${keys.length} ${label} — ${sample}${rest}`);
}

const enFamilies = pluralFamilies(en);
const enSingular = new Set([...en].filter((key) => pluralBase(key) === null));

for (const [locale, keys] of catalogs) {
  const families = pluralFamilies(keys);
  const singular = new Set([...keys].filter((key) => pluralBase(key) === null));
  if (locale !== "en") {
    // Whether a string inflects is a property of the LANGUAGE, so a locale may answer an
    // English singular with a family of its own: `{{count}} available` needs no form in
    // English and three in French, and i18next falls back from a missing form to the base.
    // What it may not do is answer with nothing, or with a family missing one of its forms.
    note(
      locale,
      [
        ...[...singular].filter((key) => !enSingular.has(key) && !enFamilies.has(key)),
        ...[...families.keys()].filter((base) => !enFamilies.has(base) && !enSingular.has(base)),
      ],
      "key(s) absent from en",
    );
    note(
      locale,
      [
        ...[...enSingular].filter((key) => !singular.has(key) && !families.has(key)),
        ...[...enFamilies.keys()].filter((base) => !families.has(base)),
      ],
      "key(s) missing",
    );
  }
  // Each family carries exactly the forms this language inflects for — no fewer, so
  // `t()` cannot fall through and render its own key at a count nobody tried, and no more,
  // so a catalog does not carry a form its readers never see.
  const wanted = pluralForms(locale).slice().sort().join(",");
  for (const [base, forms] of families) {
    const have = forms.slice().sort().join(",");
    if (have !== wanted) {
      failures.push(
        `${locale}: "${base}" declares ${have || "no"} plural form(s), needs ${wanted}`,
      );
    }
  }
}

const enValues = valuesOf(catalogFiles.get("en"));
for (const [locale] of catalogs) {
  if (locale === "en") continue;
  const values = valuesOf(catalogFiles.get(locale));
  for (const [key, english] of enValues) {
    const translated = values.get(key);
    if (translated === undefined) continue; // rule 3 already reports the gap
    const want = placeholders(english).join(",");
    const got = placeholders(translated).join(",");
    if (want !== got) {
      failures.push(
        `${locale}: "${key}" carries {{${got || "none"}}} where en carries {{${want || "none"}}}`,
      );
    }
  }
}

if (failures.length > 0) {
  console.error(`check-locales: ${failures.length} problem(s)\n`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log(`check-locales: ${en.size} keys, complete across ${catalogs.size} locales`);
