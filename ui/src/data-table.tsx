import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithRef, ReactNode } from "react";
import { color, leading, space, surface, type, weight } from "./tokens.stylex";

const styles = stylex.create({
  region: {
    minWidth: 0,
    maxWidth: "100%",
    overflow: "auto",
    overscrollBehavior: "contain",
    paddingBlock: space.s1,
  },
  table: {
    width: "100%",
    borderCollapse: "collapse",
    textAlign: "start",
    lineHeight: leading.body,
  },
  caption: { textAlign: "start", color: color.fgMuted, paddingBlock: space.s2 },
  cell: {
    fontWeight: weight.regular,
    paddingInline: space.s3,
    paddingBlock: space.s2,
    verticalAlign: "top",
    overflowWrap: "anywhere",
    borderBottomWidth: "var(--hairline-width)",
    borderBottomStyle: "solid",
    borderBottomColor: surface.divider,
  },
  numeric: {
    textAlign: "end",
    whiteSpace: "nowrap",
    fontVariantNumeric: "tabular-nums",
  },
  number: { fontFamily: "var(--font-mono)" },
  source: { minWidth: "12ch" },
  heading: {
    fontWeight: weight.medium,
    color: color.fgMuted,
    backgroundColor: surface.sunken,
  },
});
export function ScrollRegion({
  label,
  children,
  ...props
}: ComponentPropsWithRef<"div"> & { label: string }) {
  return (
    <div
      {...props}
      role="region"
      aria-label={label}
      tabIndex={0}
      {...stylex.props(styles.region)}
    >
      {children}
    </div>
  );
}
export function DataTable({
  caption,
  columns,
  rows,
  regionLabel,
}: {
  caption: string;
  columns: readonly { label: string; numeric?: boolean }[];
  rows: readonly ReactNode[][];
  regionLabel?: string;
}) {
  return (
    <ScrollRegion label={regionLabel ?? `Scrollable ${caption.toLowerCase()}`}>
      <table {...stylex.props(styles.table, type.uiSm)}>
        <caption {...stylex.props(styles.caption)}>{caption}</caption>
        <thead>
          <tr>
            {columns.map((column) => (
              <th
                key={column.label}
                scope="col"
                {...stylex.props(
                  styles.cell,
                  styles.heading,
                  !column.numeric && styles.source,
                  column.numeric && styles.numeric,
                )}
              >
                {column.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={index}>
              {row.map((value, column) =>
                column === 0 ? (
                  <th
                    key={column}
                    scope="row"
                    {...stylex.props(styles.cell, styles.source)}
                  >
                    {value}
                  </th>
                ) : (
                  <td
                    key={column}
                    {...stylex.props(
                      styles.cell,
                      columns[column]?.numeric && styles.numeric,
                      columns[column]?.numeric && styles.number,
                    )}
                  >
                    {value}
                  </td>
                ),
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </ScrollRegion>
  );
}
