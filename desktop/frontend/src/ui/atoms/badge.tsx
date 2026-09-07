import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import type { Tone } from "@/lib/tone";
import { cn } from "@/lib/classNames";
import { color, corner, face, space, surface, type, weight } from "@/styles/tokens.stylex";

/**
 * A small standing label: a status, a count, a scope, a name.
 *
 * `face` exists because eight of twenty-two call sites were writing `font-mono` — HTTP status
 * codes, tool names, provider ids, error codes. That utility swaps the family and nothing else,
 * so all eight were rendering mono glyphs at `--tracking-ui`, a negative tracking chosen for a
 * proportional face. A call site could not have fixed it: the tracking lives in the type step,
 * not in the font utility. `face.mono` carries both halves.
 */
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
  className?: string;
  title?: string;
};

export function Badge({
  tone = "neutral",
  size = "sm",
  face: textFace = "text",
  className,
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
  );
  return (
    <span title={title} {...styled} className={cn(styled.className, className)}>
      {children}
    </span>
  );
}
