import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { color, space, surface, weight } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "./settingStyles";

const styles = stylex.create({
  row: {
    display: "grid",
    gap: space.s6,
    borderTopWidth: { default: "var(--control-edge-width)", ":first-child": 0 },
    borderTopStyle: "solid",
    borderTopColor: surface.field,
    paddingInline: space.s4,
    paddingBlock: space.s3,
  },
  split: {
    gridTemplateColumns: {
      default: "minmax(0, 1fr) auto",
      "@container (max-width: 560px)": "minmax(0, 1fr)",
    },
    rowGap: space.s3,
  },
  stacked: { gap: space.s3 },
  top: { alignItems: "flex-start" },
  centre: { alignItems: "center" },
  name: { color: color.fg, fontWeight: weight.medium },
  control: {
    minWidth: 0,
    maxWidth: "100%",
    justifySelf: { default: "end", "@container (max-width: 560px)": "stretch" },
  },
});

export function SettingRow({
  label,
  labelId,
  sub,
  align = "center",
  children,
}: {
  label: string;
  labelId?: string;
  sub: string;
  align?: "start" | "center" | "stacked";
  children: ReactNode;
}) {
  const stacked = align === "stacked";
  return (
    <div
      {...stylex.props(
        styles.row,
        !stacked && styles.split,
        align === "center" && styles.centre,
        align === "start" && styles.top,
        stacked && styles.stacked,
      )}
    >
      <div>
        <div id={labelId} {...stylex.props(styles.name, typeStep.uiMd)}>
          {label}
        </div>
        <div {...stylex.props(ss.hintSpaced, typeStep.uiSm)}>{sub}</div>
      </div>
      <div {...stylex.props(!stacked && styles.control)}>{children}</div>
    </div>
  );
}
