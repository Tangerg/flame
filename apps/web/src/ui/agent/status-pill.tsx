import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { type DotTone } from "@/lib/tone";
import { color, radius, space, surface, type, weight } from "@flame/ui/tokens.stylex";
import { StatusDot } from "@/ui/atoms/status-dot";
const styles = stylex.create({
  pill: {
    display: "inline-flex",
    minHeight: "calc(var(--spacing) * 5.5)",
    flexShrink: 0,
    borderRadius: radius.button,
    "corner-shape": "round",
    alignItems: "center",
    gap: space.s1_5,
    backgroundColor: surface.surface2,
    paddingInline: space.s2_5,
    paddingBlock: space.s1,
    fontFamily: "var(--font-sans)",
    fontWeight: weight.medium,
    lineHeight: 1,
    whiteSpace: "nowrap",
    color: color.fgMuted,
  },
});

export function AgentStatusPill({
  children,
  tone = "idle",
}: {
  children: ReactNode;
  tone?: DotTone;
}) {
  return (
    <span data-slot="agent-status" {...stylex.props(styles.pill, type.uiSm)}>
      <StatusDot tone={tone} />
      {children}
    </span>
  );
}
