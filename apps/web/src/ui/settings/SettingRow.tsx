import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { type as typeStep, rowInset } from "@flame/ui/tokens.stylex";
import { color, layout, weight } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
const styles = stylex.create({
  row: {
    display: "grid",
    gridTemplateColumns: {
      default: "minmax(0, 1fr) auto",
      "@container (max-width: 35rem)": "minmax(0, 1fr)",
    },
    columnGap: layout.sectionGap,
    rowGap: layout.contentGap,
    paddingInline: rowInset.inline,
    paddingBlock: layout.rowPadding,
    // Rows with a switch or slider keep the pitch of rows holding an md field.
    minHeight: `calc(2 * ${layout.rowPadding} + max(var(--control-height-md), var(--control-target-min)))`,
    minWidth: 0,
  },
  top: { alignItems: "flex-start" },
  centre: { alignContent: "center", alignItems: "center" },
  copy: { minWidth: 0, overflowWrap: "anywhere" },
  name: { color: color.fg, fontWeight: weight.medium },
  control: {
    minWidth: 0,
    maxWidth: "100%",
    justifySelf: { default: "end", "@container (max-width: 35rem)": "stretch" },
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
  sub?: ReactNode;
  align?: "start" | "center";
  children?: ReactNode;
}) {
  return (
    <div
      data-focus-unit=""
      {...stylex.props(
        styles.row,
        align === "center" && styles.centre,
        align === "start" && styles.top,
      )}
    >
      <div {...stylex.props(styles.copy)}>
        <div id={labelId} {...stylex.props(styles.name, typeStep.uiMd)}>
          {label}
        </div>
        {sub != null && <div {...stylex.props(ss.hintSpaced, typeStep.uiSm)}>{sub}</div>}
      </div>
      {children != null && <div {...stylex.props(styles.control)}>{children}</div>}
    </div>
  );
}
