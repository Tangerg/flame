import { colord } from "colord";
import { describe, expect, it } from "vitest";
import { densityCssVariables } from "./kit/density";
import { DEFAULT_UI_DENSITY } from "./kit/appearance";
import { iconScaleCssVariables } from "@/lib/iconScale";
import { normalizeUiFontSize } from "@/lib/typography";
import { uiTypeLadderCssVariables } from "./kit/typeLadder";
import { declaredInBlock, declaredInCss } from "@/test/stylesheet";

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

  it("draws image edges in pure ink, never the theme's tinted neutral", () => {
    expect(declaredInBlock(":root", "--color-media-edge")).toBe("rgb(0 0 0 / 0.1)");
    expect(declaredInBlock("html.theme-dark", "--color-media-edge")).toBe("rgb(255 255 255 / 0.1)");
  });

  it("keeps the scrim's own edge independent of the scheme", () => {
    expect(declaredInBlock(":root", "--color-media-preview")).toBe("rgb(0 0 0 / 0.9)");
    expect(declaredInBlock(":root", "--color-on-media")).toBe("#ffffff");
  });
  it("keeps one reading edge for the chat gutter, the dock rows and the tab strip", () => {
    expect(declaredInBlock(":root", "--density-column-gutter-wide")).toBe("20px");
    expect(densityCssVariables("compact")["--density-column-gutter-wide"]).toBe("17px");
    expect(densityCssVariables("spacious")["--density-column-gutter-wide"]).toBe("23px");
  });
});
