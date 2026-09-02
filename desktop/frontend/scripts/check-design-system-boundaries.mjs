#!/usr/bin/env node

import { relative, resolve } from "node:path";
import { API } from "typescript/unstable/sync";
import * as ts from "typescript/unstable/ast";

const SRC = new URL("../src/", import.meta.url).pathname;
const ROOT = new URL("../", import.meta.url).pathname;
const TSCONFIG = resolve(ROOT, "tsconfig.json");
const PRIMITIVES = "ui/primitives/";
const DESIGN_SYSTEM_RINGS = ["ui/primitives/", "ui/atoms/", "ui/agent/"];
const NATIVE_INTERACTIVE_TAGS = new Set([
  "a",
  "button",
  "details",
  "input",
  "select",
  "summary",
  "textarea",
]);
const NATIVE_INTERACTIVE_ROLES = new Set([
  "button",
  "checkbox",
  "menuitem",
  "option",
  "radio",
  "separator",
  "slider",
  "switch",
  "tab",
  "treeitem",
]);

// A small raised token — a raised fill, a tag-scale corner, a hair of horizontal padding and
// a UI type step — is `Tag` (a literal the reader may need to copy) or `Badge` (a state named
// in the reader's language). Fourteen call sites had hand-rolled it in nine spellings, three
// of which rendered the SAME field two different ways in two views, and one carried a second
// tone palette beside the one Badge owns.
//
// The bare `bg-surface-2` is what makes this a token rather than a state: `hover:bg-surface-2`
// and `data-[highlighted]:bg-surface-2` are row feedback and are left alone. Read per line, so
// a class list prettier split across several string literals can still slip through — every
// form that existed was one line, and a guard that reads what people write beats one that
// reads what they might.
const TAG_SHAPE = [
  /(?<![:\]-])\bbg-surface-2\b/,
  /\brounded-(?:2xs|xs|sm)\b/,
  /\bpx-[\d.]+\b/,
  /\btext-ui-(?:xs|sm|md)\b/,
];

// The same defect wearing a colour. `Tone`'s own header says the application layer emits
// what a state MEANS and the Badge picks the fill and the ink — so a tinted fill sitting
// beside its own coloured ink is a call site painting a second palette. Six did, in two
// conventions (`-wash` with coloured ink, `-badge` with coloured ink) against Badge's one,
// and two of them kept a scope-to-classes table where a scope-to-Tone table belonged.
//
// The fill has to be BARE and the ink has to match it: `hover:bg-warning-wash` is row
// feedback, and a tint with no coloured ink is a panel or a row state, not a badge. Both
// stay.
const TONE_FAMILY = "accent|negative|warning|success|info";
const TONED_BADGE = [
  new RegExp(String.raw`(?<![:\]-])\bbg-(${TONE_FAMILY})-(?:wash|badge)\b`),
  new RegExp(String.raw`(?<![:\]-])\btext-(${TONE_FAMILY})\b`),
];

// A recessed block of verbatim machine text is `Well`. Nine call sites drew it in six
// spellings before it had an atom; the mono is what separates it from a plain sunken panel.
const WELL_SHAPE = [/(?<![:\]-])\bbg-sunken\b/, /\brounded-\S+/, /\bfont-mono\b/];

function isTestFile(path) {
  return /\.(?:spec|test)\.[jt]sx?$/.test(path) || path.includes("/__tests__/");
}

function lineOf(sourceFile, node) {
  return sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile)).line + 1;
}

function stringAttribute(node, name) {
  for (const property of node.attributes.properties) {
    if (!ts.isJsxAttribute(property) || property.name.text !== name) continue;
    return property.initializer && ts.isStringLiteral(property.initializer)
      ? property.initializer.text
      : undefined;
  }
  return undefined;
}

const violations = [];
const compiler = new API({ cwd: ROOT });
let compilerClosed = false;
function closeCompiler() {
  if (compilerClosed) return;
  compilerClosed = true;
  compiler.close();
}
process.once("exit", closeCompiler);
const snapshot = compiler.updateSnapshot({ openProjects: [TSCONFIG] });
const project = snapshot.getProject(TSCONFIG);
if (!project) throw new Error("TypeScript did not load tsconfig.json");

let examined = 0;
for (const fileName of project.program.getSourceFileNames()) {
  const path = resolve(fileName);
  if (!path.startsWith(SRC) || isTestFile(path)) continue;

  const rel = relative(SRC, path);
  const sourceFile = project.program.getSourceFile(path);
  if (!sourceFile) continue;
  examined += 1;
  const insidePrimitives = rel.startsWith(PRIMITIVES);
  const insideDesignSystem = DESIGN_SYSTEM_RINGS.some((prefix) => rel.startsWith(prefix));

  function visit(node) {
    if (ts.isImportDeclaration(node) && ts.isStringLiteral(node.moduleSpecifier)) {
      const specifier = node.moduleSpecifier.text;
      if (specifier.startsWith("@base-ui/react") && !insidePrimitives) {
        violations.push(`${rel}:${lineOf(sourceFile, node)} imports Base UI outside ui/primitives`);
      }
      if (
        (specifier === "@/ui/primitives" || specifier.startsWith("@/ui/primitives/")) &&
        !insideDesignSystem
      ) {
        violations.push(
          `${rel}:${lineOf(sourceFile, node)} imports ui/primitives outside the design system`,
        );
      }
    }

    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      ts.isIdentifier(node.tagName) &&
      node.tagName.text === node.tagName.text.toLowerCase()
    ) {
      const tag = node.tagName.text;
      if (NATIVE_INTERACTIVE_TAGS.has(tag) && !insidePrimitives) {
        violations.push(
          `${rel}:${lineOf(sourceFile, node)} renders native <${tag}> outside ui/primitives`,
        );
      }

      const role = stringAttribute(node, "role");
      if (role && NATIVE_INTERACTIVE_ROLES.has(role) && !insidePrimitives) {
        violations.push(
          `${rel}:${lineOf(sourceFile, node)} implements role="${role}" outside ui/primitives`,
        );
      }
    }

    node.forEachChild(visit);
  }

  visit(sourceFile);

  if (!insideDesignSystem) {
    const rebuilt = [
      [TAG_SHAPE, "hand-rolls a Tag/Badge — use <Tag> for a literal, <Badge> for a state"],
      [TONED_BADGE, "paints a tone itself — emit a `Tone` and let <Badge> pick fill and ink"],
      [WELL_SHAPE, 'hand-rolls a Well — use <Well>, or <TextArea variant="well"> to edit one'],
    ];
    const lines = sourceFile.getFullText().split("\n");
    for (const [index, line] of lines.entries()) {
      for (const [shape, message] of rebuilt) {
        if (shape.every((fragment) => fragment.test(line))) {
          violations.push(`${rel}:${index + 1} ${message}`);
        }
      }
    }
  }
}

closeCompiler();

if (violations.length > 0) {
  console.error(`check-design-system-boundaries: ${violations.length} abstraction bypass(es)\n`);
  for (const violation of violations) console.error(`  ${violation}`);
  process.exit(1);
}

// Floor, not a target: a guard that read nothing prints the same OK as one that read everything.
const MIN_FILES_EXAMINED = 500;
if (examined < MIN_FILES_EXAMINED) {
  console.error(
    `check-design-system-boundaries: only read ${examined} files (floor ${MIN_FILES_EXAMINED}) — the program is not loading src.`,
  );
  process.exit(2);
}

console.log(
  `check-design-system-boundaries: ${examined} files read; native interaction and Base UI stay behind design-system rings`,
);
