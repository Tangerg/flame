import { iconSizePx, iconScaleCssVariables, type IconSize } from "@flame/ui/font-scale";
import { normalizeUiFontSize } from "@flame/ui/typography";
export type ProductIconSize = IconSize | "composer";

const COMPOSER_ICON_PX = 16;

export function productIconSizePx(
  size: ProductIconSize,
  basePx: number | null | undefined,
): number {
  return size === "composer" ? COMPOSER_ICON_PX : iconSizePx(size, normalizeUiFontSize(basePx));
}

export function productIconScaleCssVariables(
  basePx: number | null | undefined,
): Readonly<Record<string, string>> {
  return { ...iconScaleCssVariables(basePx), "--icon-composer": `${COMPOSER_ICON_PX}px` };
}
