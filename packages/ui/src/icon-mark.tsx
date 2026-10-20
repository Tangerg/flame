import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { color, radius, space, surface } from "./tokens.stylex";

const styles = stylex.create({
  face: {
    display: "inline-grid",
    placeItems: "center",
    flexShrink: 0,
    color: color.accentText,
    backgroundImage: surface.emphasis,
    borderRadius: radius.lg,
  },
  small: { width: space.s7, height: space.s7 },
  medium: { width: space.s9, height: space.s9 },
  large: { width: space.s10, height: space.s10 },
});

const sizes = { sm: styles.small, md: styles.medium, lg: styles.large };

export function IconMark({
  children,
  size = "md",
}: {
  children: ReactNode;
  size?: "sm" | "md" | "lg";
}) {
  return (
    <span aria-hidden="true" {...stylex.props(styles.face, sizes[size])}>
      {children}
    </span>
  );
}
