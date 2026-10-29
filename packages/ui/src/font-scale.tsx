import { createContext, useContext } from "react";
import type { ComponentType, ReactNode } from "react";
import type { IconProps } from "@flame/icons/react";
import { normalizeUiFontSize, UI_FONT_SIZE_DEFAULT_PX } from "./typography";
import { iconSizePx, type IconSize } from "./icon-scale";

export type IconGlyph = ComponentType<IconProps>;
const FontScale = createContext(UI_FONT_SIZE_DEFAULT_PX);

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
