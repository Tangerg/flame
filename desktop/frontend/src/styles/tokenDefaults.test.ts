import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { densityCssVariables, DEFAULT_UI_DENSITY } from "@/lib/density";
import { iconScaleCssVariables } from "@/lib/iconScale";
import { normalizeUiFontSize } from "@/lib/typography";

// `globals.css` declares every `--density-*` and `--icon-*` value as a literal, and the
// appearance painter writes the same properties from TypeScript once a preference resolves.
// Two sources for one number: the CSS is what a reader sees before hydration, so a TS change
// alone leaves the first paint at the old measure with nothing to say so.

const CSS = readFileSync(join(process.cwd(), "src/styles/globals.css"), "utf8");

function declaredInCss(property: string): string | undefined {
  return new RegExp(`${property.replace("--", "--")}:\\s*([^;]+);`).exec(CSS)?.[1]?.trim();
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

  it("is not vacuous — a property the stylesheet does not declare is reported", () => {
    expect(expectDefaults({ "--density-not-a-property": "1px" })).toEqual([
      "--density-not-a-property: css=absent ts=1px",
    ]);
  });
});
