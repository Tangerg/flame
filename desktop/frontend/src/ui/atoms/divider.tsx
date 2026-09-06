import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { color, radius, space, surface, type } from "@/styles/tokens.stylex";

const styles = stylex.create({
  row: {
    display: "flex",
    alignItems: "center",
    gap: space.s3,
    marginBlock: space.s2,
    color: color.fgFaint,
    fontWeight: 500,
  },
  rule: {
    height: "1px",
    flex: 1,
    backgroundColor: surface.divider,
  },
  badge: {
    display: "grid",
    placeItems: "center",
    height: space.s4_5,
    width: space.s4_5,
    borderRadius: radius.pill,
    backgroundColor: surface.surface2,
  },
  neutral: { color: color.fgFaint },
  accent: { color: color.accent },
  label: { minWidth: 0, flexShrink: 0 },
});

export function Divider({
  icon,
  intent = "neutral",
  align = "center",
  className,
  children,
}: {
  icon?: ReactNode;
  intent?: "neutral" | "accent";
  align?: "center" | "start";
  className?: string;
  children: ReactNode;
}) {
  const rule = <span aria-hidden {...stylex.props(styles.rule)} />;
  const row = stylex.props(styles.row, type.uiSm);
  return (
    <div {...row} className={cn(row.className, className)}>
      {align === "center" && rule}
      {icon && <div {...stylex.props(styles.badge, styles[intent])}>{icon}</div>}
      <span {...stylex.props(styles.label)}>{children}</span>
      {rule}
    </div>
  );
}
