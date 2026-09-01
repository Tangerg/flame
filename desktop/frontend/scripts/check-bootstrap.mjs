#!/usr/bin/env node
// Boot-contract guard — the pre-module bootstrap must agree with the app it boots.
//
// `index.html` runs two inline scripts before any module loads, because both jobs
// have to happen before the first paint: pick the scheme class + canvas colour,
// and mark the input modality. Neither can import anything — that's the point of
// being inline — so each one restates something another file owns:
//
//   - the localStorage key the preference store persists under,
//   - the class names the theme painter writes,
//   - the canvas colour per scheme, which lives in globals.css as a token,
//   - the attribute the focus-ring rule gates on,
//   - and, across the language boundary, the native window colour in main.go,
//     which shows for the frame before the WebView paints.
//
// Contracts in three languages, held by nothing. One had already drifted: the dark
// canvas was #121212 against a #101010 token, so a dark cold boot painted one grey and
// repainted another a moment later. This pins each of them.
//
// The same shape recurs one layer in, so the file grew past the bootstrap itself: any
// value a running app derives and globals.css also states as a literal, plus the motion
// ladder, which three files state independently. No count here on purpose — it was
// written as "five", and was wrong by the time anyone read it.
//
// Every one of them fails silently when it drifts — the wrong scheme for a frame,
// a ring that never shows, a flash of the wrong grey — which is exactly the class
// of defect nobody files a bug for.

import { readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(path, import.meta.url).pathname, "utf8");

const html = read("../index.html");
const css = read("../src/styles/globals.css");
const store = read("../src/plugins/builtin/theme/adapters/appearanceStore.ts");
const painter = read("../src/plugins/builtin/theme/adapters/documentAppearance.ts");
const shell = read("../../main.go");

const failures = [];

function expect(condition, message) {
  if (!condition) failures.push(message);
}

// ── 1. The persisted-preference key ───────────────────────────────────────────
// Read from the NAMED constant, not `name:`'s literal: a regex for the inline literal
// resolves to `undefined` the moment it stops being inline, which is a guard comparing a
// string to nothing and calling it agreement.
const htmlStorageKey = html.match(/localStorage\.getItem\("([^"]+)"\)/)?.[1];
const storeName = store.match(/APPEARANCE_STORAGE_KEY\s*=\s*"([^"]+)"/)?.[1];
expect(
  storeName !== undefined,
  "the appearance store no longer declares APPEARANCE_STORAGE_KEY — this check reads nothing",
);
expect(
  htmlStorageKey !== undefined && htmlStorageKey === storeName,
  `index.html reads localStorage "${htmlStorageKey}" but the store persists under "${storeName}"`,
);

// ── 2. The scheme class names ─────────────────────────────────────────────────
const htmlSchemeClass = html.match(/classList\.add\("([a-z]+)-"\s*\+\s*scheme\)/)?.[1];
expect(
  htmlSchemeClass !== undefined &&
    painter.includes(`\`${htmlSchemeClass}-\${scheme}\``) &&
    css.includes(`.${htmlSchemeClass}-dark`),
  `index.html writes "${htmlSchemeClass}-{scheme}", which the painter or globals.css does not use`,
);

// ── 3. The canvas colour per scheme ───────────────────────────────────────────
// globals.css declares --color-bg twice: light scheme first, then dark.
const canvasTokens = [...css.matchAll(/--color-bg:\s*(#[0-9a-fA-F]{3,8})/g)].map((m) =>
  m[1].toLowerCase(),
);
const htmlCanvas = html
  .match(
    /backgroundColor\s*=\s*scheme === "light" \? "(#[0-9a-fA-F]{3,8})" : "(#[0-9a-fA-F]{3,8})"/,
  )
  ?.slice(1, 3)
  .map((hex) => hex.toLowerCase());
expect(
  canvasTokens.length === 2,
  `globals.css declares --color-bg ${canvasTokens.length} time(s); expected one per scheme`,
);
expect(
  htmlCanvas !== undefined &&
    canvasTokens.length === 2 &&
    htmlCanvas.join() === canvasTokens.join(),
  `index.html paints ${htmlCanvas?.join(" / ")} where --color-bg is ${canvasTokens.join(" / ")}`,
);

// ── 4. The input-modality attribute ───────────────────────────────────────────
const modalityAttr = html.match(/setAttribute\("(data-[a-z-]+)",\s*""\)/)?.[1];
expect(
  modalityAttr !== undefined && css.includes(`html:not([${modalityAttr}])`),
  `index.html marks [${modalityAttr}] but no globals.css rule gates on it`,
);

// ── 5. The native window colour ───────────────────────────────────────────────
// The window opens before the WebView paints, so its colour must be the LIGHT
// canvas: the shell can't read a preference the WebView owns, and light is the
// app's default scheme. A dark-theme launch shows this frame briefly — the cost
// of not giving Go a second copy of the theme to read.
// v3 spells this `application.NewRGB(r, g, b)` on the WINDOW's options, where v2 had a
// `&options.RGBA{...}` literal on the application's. Matching the constructor rather than
// a struct literal is also why this reads as a call: there is no field to name.
const rgba = shell.match(
  /BackgroundColour:\s*application\.NewRGB\(\s*(\d+),\s*(\d+),\s*(\d+)\s*\)/,
);
const shellHex =
  rgba &&
  `#${[rgba[1], rgba[2], rgba[3]].map((v) => Number(v).toString(16).padStart(2, "0")).join("")}`;
expect(
  shellHex !== null && canvasTokens.length === 2 && shellHex === canvasTokens[0],
  `main.go opens the window ${shellHex} where the light --color-bg is ${canvasTokens[0]}`,
);

// The runtime-derived mirrors — the type ladder, the depth step, the motion ladder and the
// palette — used to be checked here too, by regex over the modules that own them. They are
// now checked by importing those modules (`theme/stylesheetMirror.test.ts` and
// `styles/tokenDefaults.test.ts`), which covers the whole of each mirror rather than the
// handful of values a pattern could reach, and cannot be defeated by moving a function to a
// better home — which is exactly what silently broke the regex that read `depthStep` out of
// the painter's source.
//
// What stays here is what no test can import: the pre-paint `<script>` in `index.html` and
// the Go shell's window colour.

if (failures.length > 0) {
  console.error(`[check-bootstrap] ${failures.length} boot-contract drift(s):\n`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log("[check-bootstrap] OK — the pre-paint bootstrap agrees with the app");
