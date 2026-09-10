#!/usr/bin/env node
// Dead custom properties — a token declared and read by nothing.
//
// `check-dead-styles` answers the same question for CLASS names and has since the migration.
// Nothing asked it about custom properties, and three had accumulated: `--color-line`,
// `--color-surface-4` and `--color-media-canvas`. Two of them are the residue this file's
// subject keeps producing — globals.css's own header records twelve more that "were read by
// nothing at all: they were there so a utility could be generated from them".
//
// A token that nothing reads is worse than a missing one in the same way a discarded class is:
// it reads as a decision, it appears in every search for the thing it names, and editing it
// changes nothing.
//
// Two ways a naive version of this gets it WRONG, both of which it did before this comment
// existed and both of which would have deleted live tokens:
//
//   A NAME BUILT AT RUNTIME. `icon.tsx` reads `var(--icon-stroke-${size})`, so a search for
//   `var(--icon-stroke-md)` finds nothing while all five rungs are load-bearing. Deleting them
//   would have changed the stroke weight of every icon in the product. Any `var(--prefix-${…})`
//   in the source therefore marks `--prefix-*` live.
//
//   A CONSUMER IN A DEPENDENCY. `markdown.css` sets eleven Primer-named variables
//   (`--color-accent-fg`, `--color-danger-emphasis`, …) scoped to `.md .markdown-alert`, and the
//   only thing that reads them is `remark-github-blockquote-alert/alert.css`, which the markdown
//   renderer imports. Nothing in `src` references them. So the stylesheets this app IMPORTS
//   from dependencies are part of the reference set.

import { readFileSync, readdirSync, statSync, existsSync } from "node:fs";
import { extname, join, relative } from "node:path";

const ROOT = new URL("../", import.meta.url).pathname;
const SOURCE_DIRS = ["src", "visual"];

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) yield* walk(path);
    else yield path;
  }
}

const files = [];
for (const dir of SOURCE_DIRS) {
  const absolute = join(ROOT, dir);
  if (!existsSync(absolute)) continue;
  for (const path of walk(absolute)) {
    if ([".css", ".ts", ".tsx", ".mjs"].includes(extname(path))) files.push(path);
  }
}

const declared = new Map(); // name -> where it was declared
const referenced = new Set();
const livePrefixes = new Set();
const importedSheets = new Set();

for (const path of files) {
  const text = readFileSync(path, "utf8");
  const rel = relative(ROOT, path);

  if (extname(path) === ".css") {
    for (const match of text.matchAll(/^\s*(--[a-z0-9-]+)\s*:/gm)) {
      if (!declared.has(match[1])) declared.set(match[1], rel);
    }
  }
  // A token the appearance pipeline or a visual style writes is declared just as much as one
  // spelled in a stylesheet: `documentAppearance` sets `--${name}` for every key of the style
  // token map, which is where the whole `--app-*` surface comes from.
  for (const match of text.matchAll(/setProperty\(\s*"(--[a-z0-9-]+)"/g)) {
    if (!declared.has(match[1])) declared.set(match[1], rel);
  }

  for (const match of text.matchAll(/var\(\s*(--[a-z0-9-]+)/g)) referenced.add(match[1]);
  // `var(--icon-stroke-${size})` reads a rung whose name only exists at runtime, so the prefix
  // is live and every rung under it with it.
  //
  // Only a READ counts. `iconScale.ts` writes the same rungs through
  // `variables[`--icon-stroke-${size}`]`, and letting a write mark the prefix live made the
  // guard unable to notice the case it exists for: deleting the one consumer in `icon.tsx` left
  // all five rungs read by nothing and the guard reported them live, because the writer still
  // matched. A token nothing reads is dead however many places write it.
  //
  // The prefix must also be a NAMED one, ending in the hyphen the rung hangs off. Matching a
  // bare `--` marks the whole token surface live and the guard congratulates itself on 270
  // tokens while checking none of them — which is what it did, because `documentAppearance`
  // writes the visual style's map through `` `--${name}` ``. Those keys need no exemption: they
  // are read through `var()` like everything else, so they are already in the reference set.
  const RUNG = /var\(\s*(--[a-z0-9]+(?:-[a-z0-9]+)*-)\$\{/g;
  for (const match of text.matchAll(RUNG)) livePrefixes.add(match[1]);

  for (const match of text.matchAll(/import\s+"([^"]+\.css)"/g)) {
    if (!match[1].startsWith(".")) importedSheets.add(match[1]);
  }
}

// The dependency stylesheets this app actually pulls in, and nothing else under node_modules.
for (const specifier of importedSheets) {
  const path = join(ROOT, "node_modules", specifier);
  if (!existsSync(path)) continue;
  const text = readFileSync(path, "utf8");
  for (const match of text.matchAll(/var\(\s*(--[a-z0-9-]+)/g)) referenced.add(match[1]);
}

// Read by the native shell rather than by CSS: Wails' window-drag regions are a CSS custom
// property the Go host reads off the hit-tested element, so no `var()` will ever mention it.
const NATIVE = new Set(["--wails-draggable"]);

const dead = [...declared]
  .filter(([name]) => !referenced.has(name))
  .filter(([name]) => !NATIVE.has(name))
  // A rung, not a subtree. `var(--icon-${size})` interpolates ONE identifier, so it can only
  // ever name `--icon-<segment>` — and letting it match anything beginning `--icon-` made it
  // cover `--icon-stroke-*` as well, which is a second ladder with its own reader. With that
  // reader deleted the five stroke rungs were read by nothing and this still called them live.
  .filter(
    ([name]) =>
      ![...livePrefixes].some(
        (prefix) => name.startsWith(prefix) && !name.slice(prefix.length).includes("-"),
      ),
  );

if (dead.length > 0) {
  console.error(`check-dead-tokens: ${dead.length} custom propert(ies) nothing reads\n`);
  for (const [name, where] of dead) console.error(`  ${where}  ${name}`);
  console.error(
    `\nIf a consumer builds the name at runtime or lives in a dependency stylesheet, this` +
      ` guard has to learn about it — see its header.`,
  );
  process.exit(1);
}

// Floors, not targets: a walk that read nothing, or a reference set that collected nothing,
// prints the same confident line as one that read everything. Both sides get one, because the
// blindness that matters here is one side of the comparison emptying while the other looks fine.
if (declared.size < 200 || referenced.size < 200) {
  console.error(
    `check-dead-tokens: declared ${declared.size}, referenced ${referenced.size} — the walk is broken.`,
  );
  process.exit(2);
}
console.log(
  `check-dead-tokens: ${declared.size} custom properties declared, ${referenced.size} referenced` +
    ` (${importedSheets.size} dependency sheet(s), ${livePrefixes.size} runtime-built prefix(es)); every token is read`,
);
