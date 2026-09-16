#!/usr/bin/env node

import { readFileSync } from "node:fs";
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

// Three shapes an atom already owns, read off the STYLE OBJECTS people write.
//
// These were regexes over Tailwind class strings, and they had been dead since the codebase
// left Tailwind: zero authored class lists matched them, so the check printed OK while
// `ToolOutputPanel` restated the whole Well — corner, fill, inset, mono face and leading —
// in StyleX, one ring below the atom that owns it. A guard that reads a syntax nobody writes
// any more is worse than no guard, because it reports the rule as enforced.
//
// Each shape below carries its own floor, for the same reason the file already floors the
// files read and the agent classes derived: the shapes are stated in the atoms themselves, so
// an atom that stops matching its own shape means this vocabulary has drifted and the rule has
// silently stopped matching everywhere else too.
const TOKENS = readFileSync(new URL("../src/styles/tokens.stylex.ts", import.meta.url), "utf8");

// Derived, never listed: a tone whose fill is renamed must break the floor rather than quietly
// leave the rule matching nothing.
const TONE_FILLS = new Set(
  [...TOKENS.matchAll(/^ {2}(\w+(?:Wash|Badge)):/gm)].map((match) => match[1]),
);
const TONE_INKS = new Set(
  [...new Set([...TONE_FILLS].map((fill) => fill.replace(/(?:Wash|Badge)$/, "")))].filter((tone) =>
    new RegExp(String.raw`^ {2}${tone}: "var\(--color-${tone}\)"`, "m").test(TOKENS),
  ),
);

const REBUILT = [
  {
    // A recessed block of verbatim machine text is `Well`. Nine call sites drew it in six
    // spellings before it had an atom; the mono is what separates it from a plain sunken panel.
    id: "well",
    owner: "ui/atoms/well.tsx",
    message: 'hand-rolls a Well — use <Well>, or <TextArea variant="well"> to edit one',
    test: (style) =>
      style.get("backgroundColor") === "surface.sunken" &&
      (style.get("borderRadius") ?? "").startsWith("radius.") &&
      /--font-mono/.test(style.get("fontFamily") ?? ""),
  },
  {
    // A small raised token — a raised fill, a tag-scale corner and a hair of horizontal padding
    // — is `Tag` (a literal the reader may need to copy) or `Badge` (a state named in the
    // reader's language). Fourteen call sites had hand-rolled it in nine spellings.
    id: "tag",
    owner: "ui/atoms/tag.tsx",
    message: "hand-rolls a Tag/Badge — use <Tag> for a literal, <Badge> for a state",
    test: (style) =>
      style.get("backgroundColor") === "surface.surface2" &&
      /^radius\.(?:step2xs|xs|sm)$/.test(style.get("borderRadius") ?? "") &&
      (style.get("paddingInline") ?? "").startsWith("space."),
  },
  {
    // The same defect wearing a colour. The application layer emits what a state MEANS and the
    // Badge picks the fill and the ink, so a tinted fill sitting beside a coloured ink is a call
    // site painting a second palette. Badge itself never does it — its tones all take
    // `color.fgSoft` — which is why this one is floored on the token vocabulary instead.
    id: "tone",
    vocabulary: () => TONE_FILLS.size >= 5 && TONE_INKS.size >= 5,
    message: "paints a tone itself — emit a `Tone` and let <Badge> pick fill and ink",
    test: (style) =>
      TONE_FILLS.has(/^surface\.(\w+)$/.exec(style.get("backgroundColor") ?? "")?.[1] ?? "") &&
      TONE_INKS.has(/^color\.(\w+)$/.exec(style.get("color") ?? "")?.[1] ?? ""),
  },
];

// An `agent-*` class is ui/agent's private vocabulary, and a class name is NOT an export: the
// layer guard reads imports, so a consumer that spells one couples upward through a string no
// tool can see. That is how `ui/atoms` came to draw the shell's pane seam, one ring below the
// owner, invisibly. A boundary mechanism genuinely shared across rings drops the prefix
// (`pane-split`, `panel-scroll`) instead of lying about who owns it.
//
// Derived from the stylesheet, never listed here: the list is what would drift, and deriving it
// is also what keeps the many plugin ids that merely begin with `agent-` (`agent-memory`,
// `agent-fold`, `agent-session`) out of a check about CSS.
const AGENT_RING = "ui/agent/";
const GLOBALS = new URL("../src/styles/globals.css", import.meta.url);
const AGENT_CLASSES = new Map(
  [
    ...new Set(
      [...readFileSync(GLOBALS, "utf8").matchAll(/\.(agent-[a-z0-9-]+)/g)].map((m) => m[1]),
    ),
  ]
    .sort()
    .map((cls) => [cls, new RegExp(String.raw`(?<![\w-])${cls}(?![\w-])`)]),
);

function isTestFile(path) {
  return /\.(?:spec|test)\.[jt]sx?$/.test(path) || path.includes("/__tests__/");
}

function lineOf(sourceFile, node) {
  return sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile)).line + 1;
}

// Every `stylex.create` entry in one file, as a property -> source-text map. The value is kept
// verbatim (`surface.sunken`, `"var(--font-mono)"`) because that is what the shapes above are
// written against, and because a token reference is the only spelling this codebase allows.
function styleObjects(sourceFile) {
  const found = [];
  function visit(node) {
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      node.expression.name.text === "create" &&
      node.expression.expression.getText(sourceFile) === "stylex" &&
      node.arguments.length === 1 &&
      ts.isObjectLiteralExpression(node.arguments[0])
    ) {
      for (const entry of node.arguments[0].properties) {
        if (!ts.isPropertyAssignment(entry) || !ts.isObjectLiteralExpression(entry.initializer)) {
          continue;
        }
        const style = new Map();
        for (const property of entry.initializer.properties) {
          if (!ts.isPropertyAssignment(property)) continue;
          const key =
            ts.isIdentifier(property.name) || ts.isStringLiteral(property.name)
              ? property.name.text
              : undefined;
          if (key) style.set(key, property.initializer.getText(sourceFile));
        }
        found.push({
          name: ts.isIdentifier(entry.name) ? entry.name.text : "style",
          line: lineOf(sourceFile, entry),
          style,
        });
      }
    }
    node.forEachChild(visit);
  }
  visit(sourceFile);
  return found;
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
const shapesSeenAtOwner = new Set();
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

  const lines = sourceFile.getFullText().split("\n");

  if (!rel.startsWith(AGENT_RING)) {
    for (const [index, line] of lines.entries()) {
      for (const [cls, named] of AGENT_CLASSES) {
        if (!named.test(line)) continue;
        violations.push(
          `${rel}:${index + 1} names \`${cls}\` — ui/agent owns that class; take a component or a prop from @/ui/agent`,
        );
      }
    }
  }

  for (const { name, line, style } of styleObjects(sourceFile)) {
    for (const shape of REBUILT) {
      if (!shape.test(style)) continue;
      if (shape.owner === rel) shapesSeenAtOwner.add(shape.id);
      if (!insideDesignSystem) violations.push(`${rel}:${line} \`${name}\` ${shape.message}`);
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

// A shape whose own atom no longer matches it has drifted, and a drifted shape matches nothing
// anywhere else either — which is exactly how the Tailwind spellings these replaced went quiet.
for (const shape of REBUILT) {
  const alive = shape.owner ? shapesSeenAtOwner.has(shape.id) : shape.vocabulary();
  if (alive) continue;
  console.error(
    shape.owner
      ? `check-design-system-boundaries: \`${shape.id}\` no longer matches ${shape.owner}, which owns it — the shape has drifted and now matches nothing.`
      : `check-design-system-boundaries: \`${shape.id}\` derived ${TONE_FILLS.size} fill(s) and ${TONE_INKS.size} ink(s) from tokens.stylex.ts — the tone vocabulary is not being read.`,
  );
  process.exit(2);
}

// The same floor for the derived half: an empty set reads as a clean pass.
const MIN_AGENT_CLASSES = 20;
if (AGENT_CLASSES.size < MIN_AGENT_CLASSES) {
  console.error(
    `check-design-system-boundaries: derived only ${AGENT_CLASSES.size} agent-* classes (floor ${MIN_AGENT_CLASSES}) — globals.css is not being read.`,
  );
  process.exit(2);
}

console.log(
  `check-design-system-boundaries: ${examined} files read; native interaction and Base UI stay behind design-system rings, and ${AGENT_CLASSES.size} agent-* classes stay inside ui/agent`,
);
