import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import type { Tone } from "./tone";
import { color, radius, space, surface } from "./tokens.stylex";
import { toneFill } from "./tone-fill";

const styles = stylex.create({
  face: {
    display: "inline-grid",
    placeItems: "center",
    flexShrink: 0,
    borderRadius: radius.lg,
  },
  accent: { color: color.accentText, backgroundImage: surface.emphasis },
  small: { width: space.s7, height: space.s7 },
  medium: { width: space.s9, height: space.s9 },
  large: { width: space.s10, height: space.s10 },
});

const sizes = { sm: styles.small, md: styles.medium, lg: styles.large };

export function IconMark({
  children,
  size = "md",
  tone = "accent",
}: {
  children: ReactNode;
  size?: "sm" | "md" | "lg";
  tone?: Tone;
}) {
  return (
    <span
      aria-hidden="true"
      {...stylex.props(
        styles.face,
        tone === "accent" ? styles.accent : toneFill[tone],
        sizes[size],
      )}
    >
      {children}
    </span>
  );
}
