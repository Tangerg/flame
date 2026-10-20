import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import type { Tone } from "./tone";
import {
  color,
  radius,
  face,
  space,
  surface,
  type,
  weight,
} from "./tokens.stylex";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    justifySelf: "start",
    width: "fit-content",
    maxWidth: "100%",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1,
    borderRadius: radius.sm,
    fontWeight: weight.medium,
  },
  neutral: { backgroundColor: surface.surface2, color: color.fgMuted },
  accent: {
    backgroundColor: surface.accentBadge,
    color: color.accentBadgeText,
  },
  success: {
    backgroundColor: surface.successBadge,
    color: color.successBadgeText,
  },
  warning: {
    backgroundColor: surface.warningBadge,
    color: color.warningBadgeText,
  },
  negative: {
    backgroundColor: surface.negativeBadge,
    color: color.negativeBadgeText,
  },
  info: { backgroundColor: surface.infoBadge, color: color.infoBadgeText },
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

export type { Tone } from "./tone";
