import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { color, space, type, weight } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  row: {
    display: "flex",
    alignItems: "center",
    gap: space.s1_5,
    marginBlock: space.s2,
    fontWeight: weight.medium,
  },
  neutral: { color: color.fgFaint },
  accent: { color: color.accentText },
  label: { minWidth: 0 },
});

// A settled decision or a date reads as one quiet line on the reading edge, not as a rule.
export function TranscriptMark({
  icon,
  intent = "neutral",
  children,
}: {
  icon?: ReactNode;
  intent?: "neutral" | "accent";
  children: ReactNode;
}) {
  return (
    <div {...stylex.props(styles.row, type.uiSm, styles[intent])}>
      {icon}
      <span {...stylex.props(styles.label)}>{children}</span>
    </div>
  );
}
