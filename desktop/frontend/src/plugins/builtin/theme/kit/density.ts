import { DEFAULT_UI_DENSITY, UI_DENSITY_MODES, type UiDensity } from "./appearance";

const SCALE: Readonly<Record<UiDensity, number>> = {
  compact: 0.85,
  comfortable: 1,
  spacious: 1.15,
};

const BASE_PX = {
  rowHeight: 30,
  rowGap: 8,
  navigationGutter: 12,
  navigationSectionGap: 18,
  navigationGroupGap: 10,
} as const;

function isUiDensity(value: unknown): value is UiDensity {
  return typeof value === "string" && (UI_DENSITY_MODES as readonly string[]).includes(value);
}

function normalizeUiDensity(value: unknown): UiDensity {
  return isUiDensity(value) ? value : DEFAULT_UI_DENSITY;
}

export function densityCssVariables(mode: unknown): Readonly<Record<string, string>> {
  const scale = SCALE[normalizeUiDensity(mode)];
  const px = (base: number) => `${Math.round(base * scale)}px`;
  return {
    "--density-row-height": px(BASE_PX.rowHeight),
    "--density-row-gap": px(BASE_PX.rowGap),
    "--density-navigation-gutter": px(BASE_PX.navigationGutter),
    "--density-navigation-section-gap": px(BASE_PX.navigationSectionGap),
    "--density-navigation-group-gap": px(BASE_PX.navigationGroupGap),
  };
}
