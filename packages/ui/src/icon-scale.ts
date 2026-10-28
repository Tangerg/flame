import { normalizeUiFontSize } from "./typography";

const ICON_SIZES = ["xs", "sm", "md", "lg", "xl"] as const;
export type IconSize = (typeof ICON_SIZES)[number];
const OFFSETS = { xs: -2, sm: 0, md: 2, lg: 6 } as const;

export function iconSizePx(size: IconSize, fontSize: number): number {
  const base = normalizeUiFontSize(fontSize);
  return size === "xl" ? base * 2 : base + OFFSETS[size];
}

export function iconScaleCssVariables(
  fontSize: number | null | undefined,
): Readonly<Record<string, string>> {
  const base = normalizeUiFontSize(fontSize);
  return Object.fromEntries(
    ICON_SIZES.map((size) => [`--icon-${size}`, `${iconSizePx(size, base)}px`]),
  );
}
