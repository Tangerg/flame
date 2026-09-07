import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import type { DotTone } from "@/lib/tone";
import { color, corner, space, surface, type, weight } from "@/styles/tokens.stylex";
import { StatusDot } from "@/ui/atoms/status-dot";

const styles = stylex.create({
  pill: {
    display: "inline-flex",
    height: "22px",
    alignItems: "center",
    gap: space.s1_5,
    backgroundColor: surface.surface2,
    paddingInline: space.s2_5,
    fontFamily: "var(--font-sans)",
    fontWeight: weight.medium,
    lineHeight: 1,
    color: color.fgMuted,
  },
});

/**
 * A run's state, said in a word beside the mark that means it.
 *
 * The mark is `StatusDot`: this used to draw its own, with its own four-tone ladder, of which
 * one tone was ever passed. A pill saying "running" and a dot saying "running" have to look the
 * same, so there is one vocabulary — `DotTone` — and one thing that draws it.
 */
export function AgentStatusPill({
  children,
  tone = "idle",
}: {
  children: ReactNode;
  tone?: DotTone;
}) {
  return (
    <span {...stylex.props(styles.pill, corner.pill, type.uiSm)}>
      <StatusDot tone={tone} />
      {children}
    </span>
  );
}
