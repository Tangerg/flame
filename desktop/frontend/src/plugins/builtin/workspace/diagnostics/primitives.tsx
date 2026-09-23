import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useT } from "@/lib/i18n";
import { EmptyState } from "@/ui";
import { color, motion, space, surface, type } from "@/styles/tokens.stylex";

const styles = stylex.create({
  row: {
    display: "flex",
    alignItems: "center",
    gap: space.s3,
    paddingInline: space.s1,
    fontFamily: "var(--font-mono)",
  },
  head: { color: color.fgFaint },
  body: {
    color: color.fg,
    transitionProperty: "background-color",
    transitionTimingFunction: motion.easeState,
    backgroundColor: { default: null, ":hover": surface.hover },
  },
  cell: { minWidth: 0 },
  frame: { display: "flex", flex: 1, minHeight: 0, flexDirection: "column" },
  scroller: { flex: 1, minHeight: 0, overflowY: "auto" },
  canvas: { position: "relative", width: "100%" },
  // The virtualiser positions rows by transform, so each one is taken out of the flow and
  // pinned to the same origin; only `translateY` distinguishes them.
  slot: { position: "absolute", left: 0, top: 0, width: "100%" },
});

export function Empty({ hint }: { hint: string }) {
  const t = useT();
  return <EmptyState icon="activity" title={t("diagnostics.empty.title")} sub={hint} />;
}

export function Row({
  children,
  head,
  styles: extra,
}: {
  children: ReactNode;
  head?: boolean;
  styles?: StyleXStyles;
}) {
  return (
    <div
      {...stylex.props(
        styles.row,
        head === true ? styles.head : styles.body,
        head === true ? type.uiXs : type.uiMd,
        extra,
      )}
    >
      {children}
    </div>
  );
}

/**
 * A column of the panel's own table.
 *
 * The width is the TABLE's decision, so it arrives as a style the panel declared beside its
 * header — not as a class the call site spells.
 */
export function Cell({ styles: extra, children }: { styles?: StyleXStyles; children?: ReactNode }) {
  return <div {...stylex.props(styles.cell, extra)}>{children}</div>;
}

export function VirtualList({
  count,
  rowHeight,
  header,
  renderRow,
}: {
  count: number;
  rowHeight: number;
  header: ReactNode;
  renderRow: (index: number) => ReactNode;
}) {
  const parentRef = useRef<HTMLDivElement>(null);
  const virt = useVirtualizer({
    count,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 12,
  });

  return (
    <div {...stylex.props(styles.frame)}>
      {header}
      <div ref={parentRef} {...stylex.props(styles.scroller)}>
        <div {...stylex.props(styles.canvas)} style={{ height: virt.getTotalSize() }}>
          {virt.getVirtualItems().map((vi) => (
            <div
              key={vi.key}
              data-index={vi.index}
              ref={virt.measureElement}
              {...stylex.props(styles.slot)}
              style={{ transform: `translateY(${vi.start}px)` }}
            >
              {renderRow(vi.index)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
