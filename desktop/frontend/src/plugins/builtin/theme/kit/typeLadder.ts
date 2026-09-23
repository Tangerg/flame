import {
  normalizeUiFontSize,
  UI_FONT_SIZE_MIN_PX,
  UI_TYPE_STEPS,
  type UiTypeStep,
} from "@/lib/typography";

export type UiTypeLadder = Readonly<Record<UiTypeStep, number>>;

// Floors matter at the small end: ratio alone sinks `ui-2xs` to 8px at base 11. `prose` reads
// at the interface size, as Codex and zcode both set it. `aboveProse` is the invariant a ratio
// cannot hold on its own at the small end, where two steps can round to the same pixel.
const STEPS: Readonly<
  Record<
    UiTypeStep,
    { readonly ratio: number; readonly floorPx: number; readonly aboveProse?: true }
  >
> = {
  "ui-2xs": { ratio: 0.714, floorPx: 9 },
  "ui-xs": { ratio: 0.786, floorPx: 9 },
  "ui-sm": { ratio: 0.857, floorPx: 10 },
  "ui-md": { ratio: 1, floorPx: UI_FONT_SIZE_MIN_PX },
  prose: { ratio: 1, floorPx: 0 },
  code: { ratio: 0.95, floorPx: 10 },
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
    "--fs-display-sm": `${ladder["display-sm"]}px`,
    "--fs-display-md": `${ladder["display-md"]}px`,
    "--fs-display-lg": `${ladder["display-lg"]}px`,
  };
}
