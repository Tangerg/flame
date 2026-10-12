import * as stylex from "@stylexjs/stylex";
import { Fragment, type ComponentPropsWithRef, type ReactNode } from "react";
import {
  color,
  leading,
  radius,
  space,
  surface,
  type,
  weight,
} from "./tokens.stylex";
import { WELL_SURFACE } from "./well-surface";

const styles = stylex.create({
  group: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: space.s3,
  },
  title: {
    minWidth: 0,
    flexGrow: 1,
    fontWeight: weight.semibold,
    overflowWrap: "anywhere",
    textWrap: "balance",
  },
  body: {
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    lineHeight: leading.body,
    maxWidth: "75ch",
  },
  facts: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr) minmax(0,2fr)",
    columnGap: space.s3,
    rowGap: space.s1_5,
    paddingBlock: space.s2,
  },
  label: { color: color.fgMuted },
  value: {
    minWidth: 0,
    overflowWrap: "anywhere",
    fontVariantNumeric: "tabular-nums",
    fontFamily: "var(--font-mono)",
  },
  stats: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,128px),1fr))",
    gap: space.s4,
    paddingBlock: space.s4,
  },
  stat: { display: "flex", flexDirection: "column", gap: space.s1 },
  count: {
    fontWeight: weight.semibold,
    fontVariantNumeric: "tabular-nums",
    fontFamily: "var(--font-mono)",
  },
  code: {
    minWidth: 0,
    maxWidth: "100%",
    maxHeight: "min(28rem,60vh)",
    overflow: "auto",
    overscrollBehavior: "contain",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
  },
  image: {
    maxWidth: "100%",
    maxHeight: "20rem",
    objectFit: "contain",
    outline: "1px solid var(--color-media-edge)",
    outlineOffset: "-1px",
  },
  grid: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fill,minmax(min(100%,144px),1fr))",
    gap: space.s4,
  },
  tile: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: space.s2,
    paddingBlock: space.s4,
    paddingInline: space.s3,
    backgroundColor: surface.sunken,
    borderRadius: radius.sm,
    overflowWrap: "anywhere",
    textAlign: "center",
  },
  visual: { display: "grid", placeItems: "center" },
});

export type Fact = readonly [label: string, value: ReactNode];
export function RecordGroup({
  children,
  ...props
}: ComponentPropsWithRef<"section">) {
  return (
    <section {...props} {...stylex.props(styles.group)}>
      {children}
    </section>
  );
}
export function RecordTitle({ children }: { children: ReactNode }) {
  return <h2 {...stylex.props(styles.title, type.uiMd)}>{children}</h2>;
}
export function RecordBody({ children }: { children: ReactNode }) {
  return <p {...stylex.props(styles.body)}>{children}</p>;
}
export function Facts({ values }: { values: readonly Fact[] }) {
  return (
    <dl {...stylex.props(styles.facts, type.uiSm)}>
      {values
        .filter(([, value]) => value !== undefined)
        .map(([label, value]) => (
          <Fragment key={label}>
            <dt {...stylex.props(styles.label)}>{label}</dt>
            <dd {...stylex.props(styles.value)}>
              <bdi>{value}</bdi>
            </dd>
          </Fragment>
        ))}
    </dl>
  );
}
export function Stats({
  values,
  ...props
}: { values: readonly Fact[] } & ComponentPropsWithRef<"dl">) {
  return (
    <dl {...props} {...stylex.props(styles.stats)}>
      {values.map(([label, value]) => (
        <div key={label} {...stylex.props(styles.stat)}>
          <dt {...stylex.props(styles.label, type.uiSm)}>{label}</dt>
          <dd {...stylex.props(styles.count, type.displaySm)}>{value}</dd>
        </div>
      ))}
    </dl>
  );
}
export function CodeBlock({
  children,
  label,
}: {
  children: ReactNode;
  label: string;
}) {
  return (
    <pre
      role="region"
      aria-label={label}
      tabIndex={0}
      {...stylex.props(WELL_SURFACE.face, styles.code, type.code)}
    >
      <code>{children}</code>
    </pre>
  );
}
export function EvidenceImage({ src, alt }: { src: string; alt: string }) {
  return <img src={src} alt={alt} {...stylex.props(styles.image)} />;
}
export function TileGrid({
  children,
  ...props
}: ComponentPropsWithRef<"section">) {
  return (
    <section {...props} {...stylex.props(styles.grid)}>
      {children}
    </section>
  );
}
export function Tile({
  visual,
  children,
  ...props
}: ComponentPropsWithRef<"article"> & { visual: ReactNode }) {
  return (
    <article {...props} {...stylex.props(styles.tile, type.uiSm)}>
      <div {...stylex.props(styles.visual)}>{visual}</div>
      {children}
    </article>
  );
}
