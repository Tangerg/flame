import { beforeEach, describe, expect, it } from "vitest";
import { visualStyleMotion, type Scheme, type VisualStyleMotion } from "@/lib/appearance";
import { useUiStore } from "@/state/uiStore";
import { COLOR_THEME, VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey } from "@/plugins/sdk/selectors/extensions";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { declaredInBlock, driftAgainstBlock } from "@/test/stylesheet";
import { depthStep } from "./kit/tokens";
import { visualStyleMotionTokens } from "./visualStyles/tokens";

// `globals.css` states this context's palette and visual style as literals, and the painter
// overwrites the same properties from the registered specs the moment it installs. Both
// copies are needed — the stylesheet is what the first paint reads, and it cannot call a
// function — so what needs guarding is that they still say the same thing.
//
// They had stopped. Three light palette values were a palette revision behind, and twelve
// style values were a generation behind: every control height, four corner rungs, and the
// default easing curve, which the stylesheet still gave as a curve the style had replaced.
// A drift here never fails loudly; it is a frame of the old design on a cold start, which
// reads as the app settling.
//
// This lives in the theme context, not beside the stylesheet, because these specs are its
// private business — `check-layers` is what said so.

describe("the palette blocks and the theme specs they mirror", () => {
  beforeEach(async () => {
    await loadPluginsForTest(
      (await import("./themes/flame-light")).default,
      (await import("./themes/flame-dark")).default,
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
    const { builtinVisualStyles } = await import("./visualStyles");
    await loadPluginsForTest(...builtinVisualStyles);
  });

  it("agree on every shape, material and motion token the default style writes", () => {
    const spec = lookupExtensionByKey(VISUAL_STYLE, "flame") as
      { tokens?: Record<string, string>; motion: VisualStyleMotion } | undefined;
    expect(spec, "the default visual style did not register").toBeDefined();

    const written = { ...spec!.tokens, ...visualStyleMotionTokens(spec!.motion) };
    expect(Object.keys(written).length).toBeGreaterThan(50);

    // A floor just under what the sheet mirrors today, not a token gesture: the previous
    // one was low enough that a lookup reading only the FIRST `:root` block still cleared
    // it, so the whole motion ladder was skipped and the test passed anyway.
    const { compared, disagreed } = driftAgainstBlock(":root", written);
    expect(compared, ":root mirrors far less of the style than it did").toBeGreaterThan(70);
    expect(disagreed).toEqual([]);
  });

  // The THIRD copy: what `lib/appearance` hands every consumer until a style publishes.
  // Its own comment says every value must match the shipped style, and the reason it says
  // so is that `drawerMs` had drifted to 300 against a style shipping 240 — a drawer that
  // travelled on one clock cold and another warm.
  it("agree with the fallback every consumer stands on until a style publishes", () => {
    const spec = lookupExtensionByKey(VISUAL_STYLE, "flame") as { motion: VisualStyleMotion };
    expect(visualStyleMotion()).toEqual(spec.motion);
  });
});

// The last three mirrors, and the only ones the painter writes without a spec behind them.
// `--depth-step` is the ink rung every region, chip and row state is derived from, so a
// stylesheet that disagreed about it would open on a different separation everywhere at
// once. Compared as NUMBERS: the painter spells the step to one decimal and the sheet does
// not, and `4%` is not a different value from `4.0%`.
describe("the stylesheet defaults and the scalars the painter writes alone", () => {
  const percent = (value: string | undefined) => Number.parseFloat(value ?? "NaN");

  it.each([
    [":root", "light"],
    ["html.theme-dark", "dark"],
  ])("agree on the ink step %s opens at (%s)", (selector, scheme) => {
    const { contrast } = useUiStore.getState();
    expect(percent(declaredInBlock(selector, "--depth-step"))).toBe(
      percent(depthStep(scheme as Scheme, contrast)),
    );
  });

  it("agree on the shape and motion scales", () => {
    const { radiusScale, motionScale } = useUiStore.getState();
    expect(declaredInBlock(":root", "--radius-scale")).toBe(String(radiusScale));
    expect(declaredInBlock(":root", "--motion-scale")).toBe(String(motionScale));
  });
});
