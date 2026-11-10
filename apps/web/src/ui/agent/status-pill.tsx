import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { type DotTone } from "@/lib/tone";
import { color, radius, space, surface, type, weight } from "@flame/ui/tokens.stylex";
import { StatusDot } from "@/ui/atoms/status-dot";
const styles = stylex.create({
  pill: {
    display: "inline-flex",
    minWidth: 0,
    maxWidth: "100%",
    minHeight: {
      default: "calc(var(--spacing) * 5.5)",
      "@container conversation (max-width: 640px)": "auto",
    },
    flexShrink: 1,
    borderRadius: radius.button,
    "corner-shape": "round",
    alignItems: "center",
    gap: space.s1_5,
    backgroundColor: {
      default: surface.sunken,
      "@container conversation (max-width: 640px)": "transparent",
    },
    paddingInline: { default: space.s2_5, "@container conversation (max-width: 640px)": 0 },
    paddingBlock: { default: space.s1, "@container conversation (max-width: 640px)": 0 },
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
