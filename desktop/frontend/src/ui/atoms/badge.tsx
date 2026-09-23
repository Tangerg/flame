import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import { color, corner, face, space, surface, type, weight } from "@/styles/tokens.stylex";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1,
    fontWeight: weight.medium,
  },
  neutral: { backgroundColor: surface.surface2, color: color.fgMuted },
  accent: { backgroundColor: surface.accentBadge, color: color.fgSoft },
  success: { backgroundColor: surface.successBadge, color: color.fgSoft },
  warning: { backgroundColor: surface.warningBadge, color: color.fgSoft },
  negative: { backgroundColor: surface.negativeBadge, color: color.fgSoft },
  info: { backgroundColor: surface.infoBadge, color: color.fgSoft },
  sm: { paddingInline: space.s2, paddingBlock: "1px" },
  md: { paddingInline: space.s2_5, paddingBlock: space.s0_5 },
});

const SIZE_TYPE = { sm: type.uiXs, md: type.uiSm } as const;

export type BadgeProps = {
  tone?: Tone;
  size?: keyof typeof SIZE_TYPE;
  face?: keyof typeof face;
  children: ReactNode;
  styles?: StyleXArray<StyleXStyles | null | false>;
  title?: string;
};

export function Badge({
  tone = "neutral",
  size = "sm",
  face: textFace = "text",
  styles: callerStyles,
  children,
  title,
}: BadgeProps) {
  const styled = stylex.props(
    styles.base,
    corner.pill,
    styles[tone],
    styles[size],
    SIZE_TYPE[size],
    face[textFace],
    callerStyles,
  );
  return (
    <span title={title} {...styled}>
      {children}
    </span>
  );
}
