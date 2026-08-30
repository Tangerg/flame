// Every chrome text size derives from ONE base, so the user's size preference has a single
// place to act.
//
// Absolute px, NEVER rem: geometry (header height, row height, gutters) must hold still
// when the type base moves, and a rem ladder drags every padding and width with it.
//
// Nothing sits between the base and `prose` — a one-pixel intermediate cannot distinguish a
// title from a chrome label, and titles belong to a separate editorial ladder.

export const UI_FONT_SIZE_DEFAULT_PX = 14;
export const UI_FONT_SIZE_MIN_PX = 11;
export const UI_FONT_SIZE_MAX_PX = 18;

/**
 * `code` and `prose` are named by ROLE, not size: the first is the mono counterpart of
 * `ui-sm`, the second the one step for continuous reading. A runtime list and not only a
 * type, because `lib/classNames.ts` must name every step for Tailwind Merge and a hand-kept
 * copy there silently stops applying the day someone adds one here.
 */
export const UI_TYPE_STEPS = ["ui-2xs", "ui-xs", "ui-sm", "ui-md", "prose", "code"] as const;

export type UiTypeStep = (typeof UI_TYPE_STEPS)[number];

export type UiTypeLadder = Readonly<Record<UiTypeStep, number>>;

// The FLOORS matter at the small end of the base range: ratio alone sinks `ui-2xs` to 8px
// at base 11, below the size Geist stays legible at. `prose` floors at the base so it can
// never dip under the chrome it sits above.
const STEPS: Readonly<Record<UiTypeStep, { readonly ratio: number; readonly floorPx: number }>> = {
  "ui-2xs": { ratio: 0.76, floorPx: 9 },
  "ui-xs": { ratio: 0.84, floorPx: 10 },
  "ui-sm": { ratio: 0.92, floorPx: 10 },
  "ui-md": { ratio: 1, floorPx: UI_FONT_SIZE_MIN_PX },
  prose: { ratio: 1.14, floorPx: 0 },
  code: { ratio: 0.95, floorPx: 10 },
};

// `prose` overshoots the base by 14%, so the cap must sit ABOVE UI_FONT_SIZE_MAX_PX or the
// top of the ladder flattens.
const CEILING_PX = UI_FONT_SIZE_MAX_PX + 3;

/** Clamps a user-supplied base size into the supported range. `null` = default. */
export function normalizeUiFontSize(value: number | null | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return UI_FONT_SIZE_DEFAULT_PX;
  return Math.min(UI_FONT_SIZE_MAX_PX, Math.max(UI_FONT_SIZE_MIN_PX, Math.round(value)));
}

export function uiTypeLadder(basePx: number | null | undefined): UiTypeLadder {
  const base = normalizeUiFontSize(basePx);
  const ladder = {} as Record<UiTypeStep, number>;
  for (const step of UI_TYPE_STEPS) {
    const { ratio, floorPx } = STEPS[step];
    const floor = Math.max(floorPx, ratio > 1 ? base : 0);
    ladder[step] = Math.min(CEILING_PX, Math.max(floor, Math.round(base * ratio)));
  }
  return ladder;
}

/**
 * The ladder as the `--fs-*` custom properties `globals.css` maps into the
 * `text-ui-*` / `text-code` utilities. Names are spelled out so a grep for a
 * token finds both its writer and its readers.
 */
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
  };
}
