// A separate axis from the type ladder. Chrome-bar heights deliberately do NOT scale: the
// content header, drawer header and traffic-light gutter share one number across the seam.

import { DEFAULT_UI_DENSITY, UI_DENSITY_MODES, type UiDensity } from "./appearance";

const SCALE: Readonly<Record<UiDensity, number>> = {
  compact: 0.85,
  comfortable: 1,
  spacious: 1.15,
};

/** Comfortable-mode base values, in px. Every mode is these times its scale. */
const BASE_PX = {
  rowHeight: 34,
  rowGap: 8,
  navigationGutter: 12,
  navigationSectionGap: 18,
  navigationGroupGap: 10,
  columnGutter: 12,
  columnGutterWide: 20,
  composerEditorTop: 12,
  composerEditorBottom: 8,
  composerEditorStart: 12,
  composerEditorEnd: 14,
  composerFooter: 6,
  composerFooterEnd: 8,
} as const;

function isUiDensity(value: unknown): value is UiDensity {
  return typeof value === "string" && (UI_DENSITY_MODES as readonly string[]).includes(value);
}

function normalizeUiDensity(value: unknown): UiDensity {
  return isUiDensity(value) ? value : DEFAULT_UI_DENSITY;
}

/** Names spelled out so a grep for a token finds both its writer and its readers. */
export function densityCssVariables(mode: unknown): Readonly<Record<string, string>> {
  const scale = SCALE[normalizeUiDensity(mode)];
  const px = (base: number) => `${Math.round(base * scale)}px`;
  return {
    "--density-row-height": px(BASE_PX.rowHeight),
    "--density-row-gap": px(BASE_PX.rowGap),
    "--density-navigation-gutter": px(BASE_PX.navigationGutter),
    "--density-navigation-section-gap": px(BASE_PX.navigationSectionGap),
    "--density-navigation-group-gap": px(BASE_PX.navigationGroupGap),
    "--density-column-gutter": px(BASE_PX.columnGutter),
    "--density-column-gutter-wide": px(BASE_PX.columnGutterWide),
    "--density-composer-editor-top": px(BASE_PX.composerEditorTop),
    "--density-composer-editor-bottom": px(BASE_PX.composerEditorBottom),
    "--density-composer-editor-start": px(BASE_PX.composerEditorStart),
    "--density-composer-editor-end": px(BASE_PX.composerEditorEnd),
    "--density-composer-footer": px(BASE_PX.composerFooter),
    "--density-composer-footer-end": px(BASE_PX.composerFooterEnd),
  };
}
