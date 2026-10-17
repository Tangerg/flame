import { createContext, useContext, useSyncExternalStore } from "react";
import type { ComponentType, ReactNode } from "react";
import type { IconProps } from "@flame/icons/react";
import { normalizeUiFontSize, UI_FONT_SIZE_DEFAULT_PX } from "./typography";

export type IconSize = "xs" | "sm" | "md" | "lg" | "xl";
export type IconGlyph = ComponentType<IconProps>;
const FontScale = createContext(UI_FONT_SIZE_DEFAULT_PX);
const ICON_SIZES: readonly IconSize[] = ["xs", "sm", "md", "lg", "xl"];
const OFFSETS = { xs: -2, sm: 0, md: 2, lg: 6 } as const;

export function iconSizePx(size: IconSize, fontSize: number): number {
  const base = normalizeUiFontSize(fontSize);
  return size === "xl" ? base * 2 : base + OFFSETS[size];
}

export function FontScaleProvider({
  fontSize,
  children,
}: {
  fontSize: number;
  children: ReactNode;
}) {
  return (
    <FontScale value={normalizeUiFontSize(fontSize)}>{children}</FontScale>
  );
}

export function Icon({
  glyph: Glyph,
  size = "md",
  ...props
}: Omit<IconProps, "size"> & {
  glyph: IconGlyph;
  size?: IconSize | number;
}) {
  const fontSize = useFontSize();
  return (
    <Glyph
      {...props}
      size={typeof size === "number" ? size : iconSizePx(size, fontSize)}
    />
  );
}

export function useFontSize(): number {
  return useContext(FontScale);
}

function subscribe(callback: () => void) {
  window.addEventListener("flame.ui.theme", callback);
  return () => window.removeEventListener("flame.ui.theme", callback);
}

function projectedFontSize() {
  return normalizeUiFontSize(
    Number(document.documentElement.dataset.flameUiFontSize),
  );
}

export function useProjectedFontSize(): number {
  return useSyncExternalStore(
    subscribe,
    projectedFontSize,
    () => UI_FONT_SIZE_DEFAULT_PX,
  );
}

export function iconScaleCssVariables(
  fontSize: number | null | undefined,
): Readonly<Record<string, string>> {
  const base = normalizeUiFontSize(fontSize);
  return Object.fromEntries(
    ICON_SIZES.map((size) => [`--icon-${size}`, `${iconSizePx(size, base)}px`]),
  );
}
