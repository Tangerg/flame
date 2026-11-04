import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import type { Tone } from "./tone";
import { face, space, type, weight } from "./tokens.stylex";
import { toneInk } from "./tone-ink";

const styles = stylex.create({
  base: {
    display: "inline-flex",
    justifySelf: "start",
    width: "fit-content",
    maxWidth: "100%",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1,
    fontWeight: weight.medium,
  },
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
    toneInk[tone],
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
