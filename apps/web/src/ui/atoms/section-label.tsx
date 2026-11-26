import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { clsx } from "clsx";
import { vocab } from "@flame/ui";
import { color, font, leading, space, type, weight } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  row: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s2,
    fontFamily: font.sans,
    fontWeight: weight.medium,
    lineHeight: leading.tight,
    color: color.fgFaint,
  },
  label: { minWidth: 0 },
  trailing: {
    marginInlineStart: "auto",
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1_5,
    textTransform: "none",
    letterSpacing: "normal",
  },
});

export function SectionLabel({
  children,
  trailing,
  className,
}: {
  children: ReactNode;
  trailing?: ReactNode;
  className?: string;
}) {
  const row = stylex.props(styles.row, type.uiXs);
  return (
    <div {...row} className={clsx(row.className, className)}>
      <span {...stylex.props(styles.label, vocab.truncate)}>{children}</span>
      {trailing != null && <span {...stylex.props(styles.trailing)}>{trailing}</span>}
    </div>
  );
}
