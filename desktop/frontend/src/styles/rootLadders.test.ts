import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { DEFAULT_UI_DENSITY, densityCssVariables } from "@/lib/density";
import { iconScaleCssVariables } from "@/lib/iconScale";
import { UI_FONT_SIZE_DEFAULT_PX, uiTypeLadderCssVariables } from "@/lib/typography";

// Three ladders — type, icon, density — are computed in TypeScript and applied to the
// document element by the theme adapter. `globals.css` ALSO states each of them, at the
// default input, in `:root`.
//
// Both copies have to exist. The CSS one is what the first paint reads, before any adapter
// has run; the TS one is the only thing that can answer a size the person picked. Neither
// can derive the other: the stylesheet cannot call a function, and the adapter runs too
// late to be the first paint's answer.
//
// So the duplication stays and the AGREEMENT gets the guard. Drift here has no loud
// failure — it is a flash of the wrong sizes on every cold start, which reads as the app
// settling rather than as two numbers that stopped matching.

const GLOBALS = readFileSync(join(process.cwd(), "src/styles/globals.css"), "utf8");

// The `:root` block only — a later override under a media query or a `[data-*]` selector is
// a deliberate variation, not the default this is about.
function rootBlock(): string {
  const start = GLOBALS.indexOf(":root {");
  expect(start, ":root block not found in globals.css").toBeGreaterThan(-1);
  const end = GLOBALS.indexOf("\n}", start);
  return GLOBALS.slice(start, end);
}

function declaredInRoot(property: string): string | undefined {
  const match = rootBlock().match(new RegExp(`^\\s*${property}:\\s*([^;]+);`, "m"));
  return match?.[1]?.trim();
}

function expectRootMatches(computed: Readonly<Record<string, string>>): void {
  // Guards the guard: a typo in a property name would otherwise make every lookup
  // `undefined` and every comparison vacuous.
  expect(Object.keys(computed).length).toBeGreaterThan(0);
  for (const [property, value] of Object.entries(computed)) {
    expect(declaredInRoot(property), `${property} is not declared in :root`).toBeDefined();
    expect(declaredInRoot(property), `${property} disagrees with its TypeScript owner`).toBe(value);
  }
}

describe("the :root ladders match what TypeScript computes for the same input", () => {
  it("states the type ladder at the default base size", () => {
    expectRootMatches(uiTypeLadderCssVariables(UI_FONT_SIZE_DEFAULT_PX));
  });

  it("states the icon ladder at the same base — glyphs ride the type size", () => {
    expectRootMatches(iconScaleCssVariables(UI_FONT_SIZE_DEFAULT_PX));
  });

  it("states the density ladder at the default mode", () => {
    expectRootMatches(densityCssVariables(DEFAULT_UI_DENSITY));
  });
});
