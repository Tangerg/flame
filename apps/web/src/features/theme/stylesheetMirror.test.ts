import { beforeEach, describe, expect, it } from "vitest";
import { DEFAULT_MOTION, type Scheme, type VisualStyleMotion } from "@/lib/appearance";
import { createTheme, type Theme } from "@/features/theme/adapters/theme";
let owner: Theme;
beforeEach(() => {
  owner = createTheme(document, window);
});
import { COLOR_THEME, VISUAL_STYLE } from "@/features/theme/public/extensionPoints";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { loadPluginsForTest } from "@/test/kernel";
import { declaredInBlock, driftAgainstBlock } from "@/test/stylesheet";
import { DEFAULT_UI_DENSITY } from "@/features/theme/kit/appearance";
import { densityCssVariables } from "@/features/theme/kit/density";
import { uiTypeLadderCssVariables } from "@/features/theme/kit/typeLadder";
import { depthStep } from "@/features/theme/kit/tokens";
import { visualStyleMotionTokens } from "@/features/theme/visualStyles/tokens";
describe("the palette blocks and the theme specs they mirror", () => {
  beforeEach(async () => {
    await loadPluginsForTest(
      (await import("@/features/theme/themes/flame-light")).default(owner.access.preference),
      (await import("@/features/theme/themes/flame-dark")).default(owner.access.preference),
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

    const { compared, disagreed } = driftAgainstBlock(selector, tokens);
    expect(compared, `${selector} mirrors none of the spec`).toBeGreaterThan(10);
    expect(disagreed).toEqual([]);
  });
});

describe("the stylesheet defaults and the visual style that replaces them", () => {
  beforeEach(async () => {
    const { builtinVisualStyles } = await import("@/features/theme/visualStyles/index");
    await loadPluginsForTest(...builtinVisualStyles);
  });

  it("agree on every shape, material and motion token the default style writes", () => {
    const spec = lookupExtensionByKey(VISUAL_STYLE, "flame") as
      { tokens?: Record<string, string>; motion: VisualStyleMotion } | undefined;
    expect(spec, "the default visual style did not register").toBeDefined();

    const written = { ...spec!.tokens, ...visualStyleMotionTokens(spec!.motion) };
    expect(Object.keys(written).length).toBeGreaterThan(50);

    const { compared, disagreed } = driftAgainstBlock(":root", written);
    expect(compared, ":root mirrors far less of the style than it did").toBeGreaterThan(67);
    expect(disagreed).toEqual([]);
  });

  it("agree with the fallback every consumer stands on until a style publishes", () => {
    const spec = lookupExtensionByKey(VISUAL_STYLE, "flame") as { motion: VisualStyleMotion };
    expect(DEFAULT_MOTION).toEqual(spec.motion);
  });
});

describe("the stylesheet defaults and the scalars the painter writes alone", () => {
  const percent = (value: string | undefined) => Number.parseFloat(value ?? "NaN");

  it.each([
    [":root", "light"],
    ["html.theme-dark", "dark"],
  ])("agree on the ink step %s opens at (%s)", (selector, scheme) => {
    const { contrast } = owner.store.getState();
    expect(percent(declaredInBlock(selector, "--depth-step"))).toBe(
      percent(depthStep(scheme as Scheme, contrast)),
    );
  });

  it("agree on the shape and motion scales", () => {
    const { radiusScale, motionScale } = owner.store.getState();
    expect(declaredInBlock(":root", "--radius-scale")).toBe(String(radiusScale));
    expect(declaredInBlock(":root", "--motion-scale")).toBe(String(motionScale));
  });
});

describe.each([
  ["the density ladder", () => densityCssVariables(DEFAULT_UI_DENSITY)],
  ["the type ladder", () => uiTypeLadderCssVariables(null)],
])("%s and the stylesheet fallbacks it overwrites", (_label, compute) => {
  it("agree on every variable it writes, at the default", () => {
    const written = Object.fromEntries(
      Object.entries(compute()).map(([name, value]) => [name.replace(/^--/, ""), value]),
    );
    expect(Object.keys(written).length).toBeGreaterThan(0);

    const { compared, disagreed } = driftAgainstBlock(":root", written);
    expect(compared, "a variable the stylesheet never declares").toBe(Object.keys(written).length);
    expect(disagreed).toEqual([]);
  });
});
