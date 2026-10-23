import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import { type ComponentPropsWithRef, type ReactNode } from "react";
import { clsx } from "clsx";
import {
  color,
  leading,
  layout,
  space,
  surface,
  type,
  weight,
} from "./tokens.stylex";
import { ChevronRight } from "@flame/icons/react";
import { useHeadingTag } from "./heading-level";
import { Button, type ButtonProps, type ButtonVariants } from "./button";
import { CARD_SURFACE } from "./control-surface";
import { Icon, type IconGlyph } from "./font-scale";
import { IconMark } from "./icon-mark";

const styles = stylex.create({
  recordButton: {
    display: "flex",
    width: "100%",
    minWidth: 0,
    textAlign: "start",
    justifyContent: "start",
    alignItems: "start",
    gap: layout.contentGap,
    paddingInline: layout.cardPadding,
    paddingBlock: layout.rowPadding,
    borderWidth: 0,
    borderRadius: 0,
    color: color.fg,
  },
  recordChevron: { color: color.fgMuted, marginBlockStart: space.s0_5 },
  group: {
    // Callers hide a truly empty group from their read state; hiding on
    // :empty would collapse the frame while React replaces its children.
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.contentGap,
  },
  list: {
    gap: 0,
    overflow: "clip",
  },
  cards: {
    gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,20rem),1fr))",
  },
  heading: {
    display: "flex",
    alignItems: "flex-start",
    gap: layout.contentGap,
    flexWrap: "wrap",
    minWidth: 0,
  },
  headingCopy: {
    display: "grid",
    gap: space.s2,
    flex: "1 1 12rem",
    minWidth: 0,
  },
  headingLine: {
    display: "flex",
    alignItems: "center",
    flexWrap: "wrap",
    gap: space.s2,
  },
  actions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "space-between",
    columnGap: layout.contentGap,
    rowGap: space.s1,
    minWidth: 0,
  },
  meta: {
    display: "inline-flex",
    flexWrap: "wrap",
    alignItems: "center",
    columnGap: layout.contentGap,
    rowGap: space.s1,
    minWidth: 0,
    color: color.fgMuted,
    fontWeight: weight.regular,
    overflowWrap: "anywhere",
  },
  title: {
    margin: 0,
    minWidth: 0,
    fontWeight: weight.medium,
    overflowWrap: "anywhere",
    textWrap: "balance",
    lineHeight: leading.snug,
  },
  summary: {
    display: "flex",
    alignItems: "center",
    gap: layout.contentGap,
    width: "100%",
    minWidth: 0,
  },
  summaryCopy: { display: "grid", gap: space.s1, flex: 1, minWidth: 0 },
  summaryTitle: {
    color: color.fg,
    fontWeight: weight.medium,
    overflowWrap: "anywhere",
  },
  summaryDescription: {
    color: color.fgMuted,
    overflowWrap: "anywhere",
    fontWeight: weight.regular,
  },
  body: {
    margin: 0,
    userSelect: "text",
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    lineHeight: leading.body,
    maxWidth: "75ch",
    textWrapStyle: "pretty",
  },
  document: {
    minWidth: 0,
    width: "100%",
    maxWidth: "75ch",
  },
  facts: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,12rem),1fr))",
    minWidth: 0,
    gap: layout.fieldGap,
    paddingBlock: space.s2,
  },
  fact: {
    display: "grid",
    gap: space.s1,
    alignContent: "start",
    minWidth: 0,
  },
  label: { color: color.fgMuted, minWidth: 0, overflowWrap: "anywhere" },
  value: {
    minWidth: 0,
    overflowWrap: "anywhere",
    fontVariantNumeric: "tabular-nums",
  },
  stats: {
    display: "flex",
    flexWrap: "wrap",
    gap: layout.contentGap,
    paddingInline: layout.cardPadding,
    paddingBlock: layout.cardPadding,
  },
  stat: {
    flex: "1 1 max-content",
    display: "flex",
    flexDirection: "column",
    gap: layout.controlGap,
    minWidth: 0,
  },
  count: {
    minWidth: 0,
    overflowWrap: "anywhere",
    fontWeight: weight.medium,
    fontVariantNumeric: "tabular-nums",
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
    gridTemplateColumns:
      "repeat(auto-fill,minmax(min(100%,calc(10em + 2 * var(--layout-card-padding))),1fr))",
    gap: layout.fieldGap,
  },
  tile: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    minWidth: 0,
    gap: space.s2,
    paddingBlock: layout.cardPadding,
    paddingInline: layout.cardPadding,
    alignContent: "start",
    overflowWrap: "anywhere",
    textAlign: "center",
  },
  visual: {
    display: "grid",
    placeItems: "center",
    minHeight: space.s12,
    color: color.fg,
    marginBlockEnd: space.s2,
  },
  share: {
    display: "block",
    width: "100%",
    height: space.s1,
    appearance: "none",
    borderWidth: 0,
    borderRadius: "var(--shape-pill)",
    backgroundColor: surface.surface2,
    "::-webkit-meter-bar": {
      borderWidth: 0,
      borderRadius: "inherit",
      backgroundColor: surface.surface2,
      backgroundImage: "none",
      boxShadow: "none",
    },
    "::-webkit-meter-optimum-value": {
      borderRadius: "inherit",
      backgroundColor: color.accent,
      backgroundImage: "none",
    },
    "::-moz-meter-bar": {
      borderRadius: "inherit",
      backgroundColor: color.accent,
      backgroundImage: "none",
    },
  },
});

export type Fact = readonly [label: string, value: ReactNode];
export function RecordGroup({
  children,
  variant = "sections",
  styles: extra,
  className,
  ...props
}: ComponentPropsWithRef<"section"> & {
  variant?: "sections" | "list" | "cards";
  styles?: StyleXArray<StyleXStyles | null | false>;
}) {
  const styled = stylex.props(
    styles.group,
    variant === "list" && [CARD_SURFACE.face, styles.list],
    variant === "cards" && styles.cards,
    extra,
  );
  return (
    <section
      {...props}
      data-slot={variant === "list" ? "record-list" : undefined}
      {...styled}
      className={clsx(styled.className, className)}
    >
      {children}
    </section>
  );
}
export function RecordHeading({
  title,
  description,
  status,
  actions,
}: {
  title: ReactNode;
  description?: ReactNode;
  status?: ReactNode;
  actions?: ReactNode;
}) {
  const Tag = useHeadingTag();
  return (
    <div {...stylex.props(styles.heading)}>
      <div {...stylex.props(styles.headingCopy)}>
        <div {...stylex.props(styles.headingLine)}>
          <Tag {...stylex.props(styles.title, type.uiMd)}>{title}</Tag>
          {status}
        </div>
        {description && (
          <p {...stylex.props(styles.summaryDescription, type.uiSm)}>
            {description}
          </p>
        )}
      </div>
      {actions && <div {...stylex.props(styles.actions)}>{actions}</div>}
    </div>
  );
}
export function ShareBar({ value, total }: { value: number; total: number }) {
  return (
    <meter
      aria-hidden="true"
      min={0}
      max={total}
      value={value}
      {...stylex.props(styles.share)}
    />
  );
}
export function MetricValue({ children }: { children: ReactNode }) {
  return (
    <span {...stylex.props(styles.count, type.displayMd)}>{children}</span>
  );
}
export function RecordMeta({ children }: { children: ReactNode }) {
  return <span {...stylex.props(styles.meta, type.uiSm)}>{children}</span>;
}
export function RecordTitle({ children }: { children: ReactNode }) {
  const Tag = useHeadingTag();
  return <Tag {...stylex.props(styles.title, type.displaySm)}>{children}</Tag>;
}
export function RecordSummary({
  title,
  description,
  status,
  icon,
}: {
  title: ReactNode;
  description?: ReactNode;
  status?: ReactNode;
  icon?: IconGlyph;
}) {
  return (
    <span {...stylex.props(styles.summary)}>
      {icon && (
        <IconMark>
          <Icon glyph={icon} size="md" />
        </IconMark>
      )}
      <span {...stylex.props(styles.summaryCopy)}>
        <span {...stylex.props(styles.headingLine)}>
          <span {...stylex.props(styles.summaryTitle, type.uiMd)}>{title}</span>
          {status}
        </span>
        {description && (
          <span {...stylex.props(styles.summaryDescription, type.uiSm)}>
            {description}
          </span>
        )}
      </span>
    </span>
  );
}
export function RecordButton({
  children,
  ...props
}: Omit<ButtonProps, keyof ButtonVariants | "styles">) {
  const current = props["aria-current"];
  return (
    <Button
      {...props}
      variant="ghost"
      active={current !== undefined && current !== false && current !== "false"}
      styles={styles.recordButton}
    >
      {children}
      <Icon
        glyph={ChevronRight}
        size="xs"
        {...stylex.props(styles.recordChevron)}
      />
    </Button>
  );
}
export function RecordBody({ children }: { children: ReactNode }) {
  return <p {...stylex.props(styles.body)}>{children}</p>;
}
export function DocumentBody({ children }: { children: ReactNode }) {
  return (
    <article
      data-slot="document-body"
      data-prose=""
      {...stylex.props(styles.document, type.prose)}
    >
      {children}
    </article>
  );
}
export function Facts({ values }: { values: readonly Fact[] }) {
  return (
    <dl {...stylex.props(styles.facts, type.uiSm)}>
      {values
        .filter(([, value]) => value != null)
        .map(([label, value], index) => (
          <div key={index} {...stylex.props(styles.fact)}>
            <dt {...stylex.props(styles.label, type.uiXs)}>{label}</dt>
            <dd {...stylex.props(styles.value)}>
              <bdi>{value}</bdi>
            </dd>
          </div>
        ))}
    </dl>
  );
}
export function Stats({
  values,
  className,
  ...props
}: { values: readonly Fact[] } & ComponentPropsWithRef<"dl">) {
  const styled = stylex.props(CARD_SURFACE.face, styles.stats);
  return (
    <dl {...props} {...styled} className={clsx(styled.className, className)}>
      {values.map(([label, value], index) => (
        <div key={index} {...stylex.props(styles.stat)}>
          <dt {...stylex.props(styles.label, type.uiSm)}>{label}</dt>
          <dd {...stylex.props(styles.count, type.displayLg)}>{value}</dd>
        </div>
      ))}
    </dl>
  );
}
export function EvidenceImage({ src, alt }: { src: string; alt: string }) {
  return <img src={src} alt={alt} {...stylex.props(styles.image)} />;
}
export function TileGrid({
  children,
  className,
  ...props
}: ComponentPropsWithRef<"section">) {
  const styled = stylex.props(styles.grid, type.uiSm);
  return (
    <section
      {...props}
      {...styled}
      className={clsx(styled.className, className)}
    >
      {children}
    </section>
  );
}
export function Tile({
  visual,
  children,
  className,
  ...props
}: ComponentPropsWithRef<"article"> & { visual: ReactNode }) {
  const styled = stylex.props(CARD_SURFACE.face, styles.tile, type.uiSm);
  return (
    <article
      {...props}
      {...styled}
      className={clsx(styled.className, className)}
    >
      <div {...stylex.props(styles.visual)}>{visual}</div>
      {children}
    </article>
  );
}
