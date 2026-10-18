import * as stylex from "@stylexjs/stylex";
import { ScrollRegion } from "./data-table";
import { color, space, surface, type } from "./tokens.stylex";

export type ComparisonRow =
  | { type: "hunk"; text: string }
  | {
      type: "context" | "added" | "deleted";
      code: string;
      leftLine?: number;
      rightLine?: number;
    };

export interface DiffTableLabels {
  region: string;
  beforeLine: string;
  beforeCode: string;
  afterLine: string;
  afterCode: string;
  changedCode: string;
}
const styles = stylex.create({
  table: {
    width: "100%",
    borderCollapse: "collapse",
    fontFamily: "var(--font-mono)",
  },
  cell: {
    paddingBlock: space.s1,
    paddingInline: space.s2,
    textAlign: "start",
    verticalAlign: "top",
    whiteSpace: "pre",
  },
  number: {
    textAlign: "end",
    fontVariantNumeric: "tabular-nums",
    color: color.fgMuted,
    userSelect: "none",
  },
  marker: {
    display: "inline-block",
    inlineSize: "2ch",
    textAlign: "start",
  },
  added: { backgroundColor: surface.successWash },
  deleted: { backgroundColor: surface.negativeWash },
  hunk: { backgroundColor: surface.sunken, color: color.fgMuted },
});
function CodeCell({
  code,
  kind,
}: {
  code?: string;
  kind?: "added" | "deleted";
}) {
  return (
    <td {...stylex.props(styles.cell, kind && styles[kind])}>
      <span aria-hidden="true" {...stylex.props(styles.number, styles.marker)}>
        {kind === "added" ? "+" : kind === "deleted" ? "−" : ""}
      </span>
      <code>{code}</code>
    </td>
  );
}
export function DiffTable({
  rows,
  layout,
  label,
  labels,
}: {
  rows: readonly ComparisonRow[];
  layout: "unified" | "split";
  label: string;
  labels: DiffTableLabels;
}) {
  const split = layout === "split";
  return (
    <ScrollRegion label={labels.region}>
      <table data-layout={layout} {...stylex.props(styles.table, type.code)}>
        <caption className="sr-only">{label}</caption>
        <thead className="sr-only">
          <tr>
            {(split
              ? [
                  labels.beforeLine,
                  labels.beforeCode,
                  labels.afterLine,
                  labels.afterCode,
                ]
              : [labels.beforeLine, labels.afterLine, labels.changedCode]
            ).map((title, index) => (
              <th key={index} scope="col">
                {title}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) =>
            row.type === "hunk" ? (
              <tr key={index}>
                <td
                  colSpan={split ? 4 : 3}
                  {...stylex.props(styles.cell, styles.hunk)}
                >
                  {row.text}
                </td>
              </tr>
            ) : split ? (
              <tr key={index}>
                <td
                  {...stylex.props(
                    styles.cell,
                    styles.number,
                    row.type === "deleted" && styles.deleted,
                  )}
                >
                  {row.leftLine}
                </td>
                <CodeCell
                  code={row.type === "added" ? "" : row.code}
                  kind={row.type === "deleted" ? "deleted" : undefined}
                />
                <td
                  {...stylex.props(
                    styles.cell,
                    styles.number,
                    row.type === "added" && styles.added,
                  )}
                >
                  {row.rightLine}
                </td>
                <CodeCell
                  code={row.type === "deleted" ? "" : row.code}
                  kind={row.type === "added" ? "added" : undefined}
                />
              </tr>
            ) : (
              <tr key={index}>
                <td {...stylex.props(styles.cell, styles.number)}>
                  {row.leftLine}
                </td>
                <td {...stylex.props(styles.cell, styles.number)}>
                  {row.rightLine}
                </td>
                <CodeCell
                  code={row.code}
                  kind={row.type === "context" ? undefined : row.type}
                />
              </tr>
            ),
          )}
        </tbody>
      </table>
    </ScrollRegion>
  );
}
