import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { type as typeStep } from "@/styles/tokens.stylex";
import { color, space, surface } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "./settingStyles";

const styles = stylex.create({
  // The seam between two rows, drawn by the row BELOW so the group's own top edge stays clean.
  row: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto",
    gap: space.s6,
    borderTopWidth: { default: "var(--control-edge-width)", ":first-child": 0 },
    borderTopStyle: "solid",
    borderTopColor: surface.field,
    paddingInline: space.s4,
    paddingBlock: space.s3,
  },
  top: { alignItems: "flex-start" },
  centre: { alignItems: "center" },
  name: { color: color.fg },
  control: { justifySelf: "end" },
});

export function SettingRow({
  label,
  sub,
  align = "center",
  children,
}: {
  label: string;
  sub: string;
  align?: "start" | "center";
  children: ReactNode;
}) {
  return (
    <div {...stylex.props(styles.row, align === "start" ? styles.top : styles.centre)}>
      <div>
        <div {...stylex.props(styles.name, typeStep.uiMd)}>{label}</div>
        <div {...stylex.props(ss.hintSpaced, typeStep.uiMd)}>{sub}</div>
      </div>
      <div {...stylex.props(styles.control)}>{children}</div>
    </div>
  );
}
