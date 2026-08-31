import { colord } from "colord";
import { describe, expect, it } from "vitest";
import { densityCssVariables, DEFAULT_UI_DENSITY } from "@/lib/density";
import { iconScaleCssVariables } from "@/lib/iconScale";
import { normalizeUiFontSize, uiTypeLadderCssVariables } from "@/lib/typography";
import { declaredInBlock, declaredInCss } from "@/test/stylesheet";

// `globals.css` declares every `--density-*`, `--icon-*`, `--fs-*` and accent shade as a
// literal, and the appearance painter writes the same properties from TypeScript once a
// preference resolves. Two sources for one number: the CSS is what a reader sees before
// hydration, so a TS change alone leaves the first paint at the old measure with nothing to
// say so.
//
// The palette and the visual style mirror the same way, but their specs are the theme
// context's private business, so `theme/stylesheetMirror.test.ts` owns those.

function expectDefaults(written: Readonly<Record<string, string>>) {
  const disagreed: string[] = [];
  for (const [property, value] of Object.entries(written)) {
    const declared = declaredInCss(property);
    if (declared !== value) disagreed.push(`${property}: css=${declared ?? "absent"} ts=${value}`);
  }
  return disagreed;
}

describe("the stylesheet's defaults and the values TypeScript writes", () => {
  it("agree on every density property", () => {
    const written = densityCssVariables(DEFAULT_UI_DENSITY);
    expect(Object.keys(written).length).toBeGreaterThan(10);
    expect(expectDefaults(written)).toEqual([]);
  });

  it("agree on every icon size", () => {
    const written = iconScaleCssVariables(normalizeUiFontSize(undefined));
    expect(Object.keys(written).length).toBeGreaterThan(4);
    expect(expectDefaults(written)).toEqual([]);
  });

  it("agree on every type step", () => {
    const written = uiTypeLadderCssVariables(normalizeUiFontSize(undefined));
    expect(Object.keys(written).length).toBeGreaterThan(4);
    expect(expectDefaults(written)).toEqual([]);
  });

  // These two are DERIVED, not declared: the painter darkens whatever accent is live, so
  // the stylesheet's literals have to be that derivation applied to the accent each block
  // declares. They were not — the hand-picked dark shade sat two thirds of the way across
  // the blue it claimed to be a shade of, and nothing anywhere compared them.
  it.each([
    [":root", "the light default"],
    ["html.theme-dark", "the dark scheme"],
  ])("agree on the accent shades %s derives (%s)", (selector) => {
    const accent = declaredInBlock(selector, "--color-accent");
    expect(accent, `${selector} declares no accent to derive from`).toMatch(/^#[\da-f]{6}$/i);
    expect(declaredInBlock(selector, "--color-accent-border")).toBe(
      colord(accent!).darken(0.08).toHex(),
    );
    expect(declaredInBlock(selector, "--color-accent-press")).toBe(
      colord(accent!).darken(0.16).toHex(),
    );
  });

  it("is not vacuous — a property the stylesheet does not declare is reported", () => {
    expect(expectDefaults({ "--density-not-a-property": "1px" })).toEqual([
      "--density-not-a-property: css=absent ts=1px",
    ]);
  });
});
