import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import type { ComponentPropsWithRef } from "react";
import { clsx } from "clsx";
import { color, space } from "./tokens.stylex";

const styles = stylex.create({
  row: {
    display: "inline-flex",
    alignItems: "baseline",
    flexShrink: 0,
    flexWrap: "wrap",
    maxWidth: "100%",
    columnGap: space.s1_5,
    rowGap: space.s1,
    fontFamily: "var(--font-mono)",
    fontVariantNumeric: "tabular-nums",
    color: color.fgMuted,
  },
  count: { whiteSpace: "nowrap" },
  added: { color: color.success },
  removed: { color: color.negative },
});

export function DiffStat({
  added,
  removed,
  styles: extra,
  className,
  ...props
}: Omit<ComponentPropsWithRef<"span">, "children"> & {
  added: number | undefined;
  removed: number | undefined;
  styles?: StyleXArray<StyleXStyles | null | false>;
}) {
  const styled = stylex.props(styles.row, extra);
  return (
    <span
      {...props}
      data-slot="diff-stat"
      {...styled}
      className={clsx(styled.className, className)}
    >
      <span
        {...stylex.props(
          styles.count,
          added !== undefined && added > 0 && styles.added,
        )}
      >
        {`+${added ?? "?"}`}
      </span>{" "}
      <span
        {...stylex.props(
          styles.count,
          removed !== undefined && removed > 0 && styles.removed,
        )}
      >
        {`−${removed ?? "?"}`}
      </span>
    </span>
  );
}
