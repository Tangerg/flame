import { readFileSync } from "node:fs";
import { join } from "node:path";
import { colord } from "colord";
import { beforeEach, describe, expect, it } from "vitest";
import { densityCssVariables, DEFAULT_UI_DENSITY } from "@/lib/density";
import { iconScaleCssVariables } from "@/lib/iconScale";
import { normalizeUiFontSize, uiTypeLadderCssVariables } from "@/lib/typography";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey } from "@/plugins/sdk/selectors/extensions";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

// `globals.css` declares every `--density-*`, `--icon-*`, `--fs-*` and accent shade as a
// literal, and the appearance painter writes the same properties from TypeScript once a
// preference resolves. Two sources for one number: the CSS is what a reader sees before
// hydration, so a TS change alone leaves the first paint at the old measure with nothing to
// say so.

const CSS = readFileSync(join(process.cwd(), "src/styles/globals.css"), "utf8");

function declaredInCss(property: string): string | undefined {
  return new RegExp(`${property.replace("--", "--")}:\\s*([^;]+);`).exec(CSS)?.[1]?.trim();
}

// The accent shades are declared once per scheme, so the whole-file lookup above would
// always answer with the light one. Read them out of the block that owns them.
function declaredInBlock(selector: string, property: string): string | undefined {
  const start = CSS.indexOf(`${selector} {`);
  if (start === -1) return undefined;
  const block = CSS.slice(start, CSS.indexOf("\n}", start));
  return new RegExp(`^\\s*${property}:\\s*([^;]+);`, "m").exec(block)?.[1]?.trim();
}

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

// The palette is the largest mirror of all: each built-in theme's spec carries the tokens
// the painter writes, and `globals.css` restates them per scheme. Three light values had
// gone stale while the dark block stayed correct — the signature of a palette change
// applied to the spec and to one block but not the other, which is what nothing comparing
// them buys you.
describe("the stylesheet's palette blocks and the theme specs they mirror", () => {
  beforeEach(async () => {
    await loadPluginsForTest(
      (await import("@/plugins/builtin/theme/themes/flame-light")).default,
      (await import("@/plugins/builtin/theme/themes/flame-dark")).default,
    );
  });

  it.each([
    [":root", "light"],
    ["html.theme-dark", "dark"],
  ])("agree on every token %s declares (%s)", (selector, themeId) => {
    const spec = lookupExtensionByKey(COLOR_THEME, themeId) as
      { tokens?: Record<string, string> } | undefined;
    const tokens = spec?.tokens ?? {};
    expect(Object.keys(tokens).length, `${themeId} contributed no tokens`).toBeGreaterThan(10);

    // Only what the block DECLARES: a token the dark block omits inherits the light one on
    // purpose, and demanding it here would be asking the stylesheet to repeat itself.
    const disagreed: string[] = [];
    let compared = 0;
    for (const [name, value] of Object.entries(tokens)) {
      const declared = declaredInBlock(selector, `--${name}`);
      if (declared === undefined) continue;
      compared++;
      if (declared !== value) disagreed.push(`--${name}: css=${declared} spec=${value}`);
    }
    expect(compared, `${selector} mirrors none of the spec`).toBeGreaterThan(10);
    expect(disagreed).toEqual([]);
  });
});
