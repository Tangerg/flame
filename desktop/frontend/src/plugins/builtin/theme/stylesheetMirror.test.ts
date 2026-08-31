import { beforeEach, describe, expect, it } from "vitest";
import type { VisualStyleMotion } from "@/lib/appearance";
import { COLOR_THEME, VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey } from "@/plugins/sdk/selectors/extensions";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { driftAgainstBlock } from "@/test/stylesheet";
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

    const { compared, disagreed } = driftAgainstBlock(":root", written);
    expect(compared, ":root mirrors none of the style").toBeGreaterThan(20);
    expect(disagreed).toEqual([]);
  });
});
