import * as stylex from "@stylexjs/stylex";
import type { ComponentPropsWithRef, ReactNode } from "react";
import { color, leading, space, surface, type, weight } from "./tokens.stylex";

const styles = stylex.create({
  panel: {
    color: color.fg,
    backgroundColor: surface.canvas,
    fontFamily: "var(--font-sans)",
    lineHeight: leading.body,
    paddingInline: space.s3,
    paddingBlock: space.s3,
    minWidth: 0,
  },
  toolbar: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s2,
    paddingBlock: space.s2,
    minWidth: 0,
  },
  title: { margin: 0, fontWeight: weight.semibold, flexGrow: 1 },
  row: {
    paddingBlock: space.s3,
    minWidth: 0,
    overflowWrap: "anywhere",
    whiteSpace: "pre-wrap",
  },
  meta: { color: color.fgMuted, minWidth: 0, overflowWrap: "anywhere" },
});

export function Panel({
  children,
  label,
}: {
  children: ReactNode;
  label: string;
}) {
  return (
    <section aria-label={label} {...stylex.props(styles.panel, type.uiMd)}>
      {children}
    </section>
  );
}
export function Toolbar({
  children,
  label,
}: {
  children: ReactNode;
  label: string;
}) {
  return (
    <div role="group" aria-label={label} {...stylex.props(styles.toolbar)}>
      {children}
    </div>
  );
}
export function PanelTitle({ children }: { children: ReactNode }) {
  return <h1 {...stylex.props(styles.title, type.uiMd)}>{children}</h1>;
}
export function ListRow({
  children,
  ...props
}: ComponentPropsWithRef<"article">) {
  return (
    <article {...props} {...stylex.props(styles.row)}>
      {children}
    </article>
  );
}
export function Metadata({ children }: { children: ReactNode }) {
  return <div {...stylex.props(styles.meta, type.uiSm)}>{children}</div>;
}
