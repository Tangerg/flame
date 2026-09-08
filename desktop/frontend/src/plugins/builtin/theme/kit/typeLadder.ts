import {
  normalizeUiFontSize,
  UI_FONT_SIZE_MIN_PX,
  UI_TYPE_STEPS,
  type UiTypeStep,
} from "@/lib/typography";

export type UiTypeLadder = Readonly<Record<UiTypeStep, number>>;

// Floors matter at the small end: ratio alone sinks `ui-2xs` to 8px at base 11. `prose`
// floors at the base so it never dips under the chrome it sits above.
// `aboveProse` is the invariant a ratio cannot hold on its own: at base 11 the reading step
// and the h3 above it both round to 13px, because 1.14 and 1.214 are less than a pixel apart
// down there. A heading that meets its own body is the defect this round exists to remove, so
// the ladder guarantees it rather than leaving it to arithmetic that happens to work.
const STEPS: Readonly<
  Record<
    UiTypeStep,
    { readonly ratio: number; readonly floorPx: number; readonly aboveProse?: true }
  >
> = {
  "ui-2xs": { ratio: 0.76, floorPx: 9 },
  "ui-xs": { ratio: 0.84, floorPx: 10 },
  "ui-sm": { ratio: 0.92, floorPx: 10 },
  "ui-md": { ratio: 1, floorPx: UI_FONT_SIZE_MIN_PX },
  prose: { ratio: 1.14, floorPx: 0 },
  code: { ratio: 0.95, floorPx: 10 },
  // Ratios read off the sizes these steps already had at the default base, so the default
  // renders to the pixel it always did and only the ends of the range move.
  "markdown-h5": { ratio: 1.071, floorPx: 0 },
  "markdown-h3": { aboveProse: true, ratio: 1.214, floorPx: 0 },
  "display-sm": { aboveProse: true, ratio: 1.286, floorPx: 0 },
  "display-md": { aboveProse: true, ratio: 1.429, floorPx: 0 },
  "display-lg": { aboveProse: true, ratio: 1.714, floorPx: 0 },
};

export function uiTypeLadder(basePx: number | null | undefined): UiTypeLadder {
  const base = normalizeUiFontSize(basePx);
  const ladder = {} as Record<UiTypeStep, number>;
  for (const step of UI_TYPE_STEPS) {
    const { ratio, floorPx } = STEPS[step];
    // A step that overshoots the base may never fall under it; a step that undershoots keeps
    // its own floor so the small end stays legible. There is no ladder-wide ceiling: the base
    // is already clamped to [MIN, MAX], so every step is bounded by its own ratio.
    const { aboveProse } = STEPS[step];
    const floor = Math.max(
      floorPx,
      ratio > 1 ? base : 0,
      aboveProse === true ? ladder.prose + 1 : 0,
    );
    ladder[step] = Math.max(floor, Math.round(base * ratio));
  }
  return ladder;
}

/** Names spelled out so a grep for a token finds both its writer and its readers. */
export function uiTypeLadderCssVariables(
  basePx: number | null | undefined,
): Readonly<Record<string, string>> {
  const ladder = uiTypeLadder(basePx);
  return {
    "--fs-ui-2xs": `${ladder["ui-2xs"]}px`,
    "--fs-ui-xs": `${ladder["ui-xs"]}px`,
    "--fs-ui-sm": `${ladder["ui-sm"]}px`,
    "--fs-ui-md": `${ladder["ui-md"]}px`,
    "--fs-prose": `${ladder.prose}px`,
    "--fs-code": `${ladder.code}px`,
    "--fs-markdown-h5": `${ladder["markdown-h5"]}px`,
    "--fs-markdown-h3": `${ladder["markdown-h3"]}px`,
    "--fs-display-sm": `${ladder["display-sm"]}px`,
    "--fs-display-md": `${ladder["display-md"]}px`,
    "--fs-display-lg": `${ladder["display-lg"]}px`,
  };
}
