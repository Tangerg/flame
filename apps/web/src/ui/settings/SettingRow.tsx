import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { color, layout, weight } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
const styles = stylex.create({
  row: {
    display: "grid",
    gap: layout.sectionGap,
    paddingInline: layout.cardPadding,
    paddingBlock: layout.rowPadding,
    minWidth: 0,
  },
  split: {
    gridTemplateColumns: {
      default: "minmax(0, 1fr) auto",
      "@container (max-width: 35rem)": "minmax(0, 1fr)",
    },
    rowGap: layout.contentGap,
  },
  stacked: { gap: layout.contentGap },
  top: { alignItems: "flex-start" },
  centre: { alignItems: "center" },
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
  align?: "start" | "center" | "stacked";
  children?: ReactNode;
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
      <div {...stylex.props(styles.copy)}>
        <div id={labelId} {...stylex.props(styles.name, typeStep.uiMd)}>
          {label}
        </div>
        {sub != null && <div {...stylex.props(ss.hintSpaced, typeStep.uiSm)}>{sub}</div>}
      </div>
      {children != null && <div {...stylex.props(!stacked && styles.control)}>{children}</div>}
    </div>
  );
}
