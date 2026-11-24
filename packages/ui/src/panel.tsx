import * as stylex from "@stylexjs/stylex";
import {
  createContext,
  useContext,
  type ComponentPropsWithRef,
  type ReactNode,
  type Ref,
} from "react";
import { CARD_SURFACE } from "./control-surface";
import { HeadingLevel, useHeadingTag } from "./heading-level";
import { clsx } from "clsx";
import {
  color,
  leading,
  layout,
  space,
  surface,
  type,
  weight,
  rowInset,
} from "./tokens.stylex";

const styles = stylex.create({
  panel: {
    width: "100%",
    containerType: "inline-size",
    maxWidth: layout.pageWidth,
    marginInline: "auto",
    minHeight: "100%",
    flexShrink: 0,
    color: color.fg,
    backgroundColor: surface.canvas,
    backgroundImage: {
      default: null,
      ":is([data-paper-texture] *)": surface.texture,
    },
    fontFamily: "var(--font-sans)",
    lineHeight: leading.body,
    minWidth: 0,
    overflowWrap: "anywhere",
  },
  content: {
    display: "flex",
    flexDirection: "column",
    gap: layout.sectionGap,
    width: "100%",
    minWidth: 0,
    paddingInline: {
      default: layout.pageGutter,
      "@container (max-width: 600px)": layout.compactGutter,
    },
    paddingBlock: layout.pageBlock,
  },
  compactContent: {
    gap: layout.contentGap,
    paddingBlock: layout.compactGutter,
  },
  viewportPanel: {
    display: "flex",
    flexDirection: "column",
    flexGrow: 1,
    flexShrink: 1,
    flexBasis: 0,
    minHeight: 0,
  },
  viewportContent: {
    flex: 1,
    minHeight: 0,
    gap: layout.controlGap,
  },
  header: {
    display: "grid",
    gap: layout.controlGap,
    minWidth: 0,
  },
  controls: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.contentGap,
    paddingBlock: layout.controlGap,
    backgroundColor: surface.canvas,
    backgroundImage: {
      default: null,
      ":is([data-paper-texture] *)": surface.texture,
    },
    minWidth: 0,
  },
  body: {
    display: "flex",
    flexDirection: "column",
    gap: layout.sectionGap,
    minWidth: 0,
  },
  bodyViewport: {
    flex: 1,
    minHeight: 0,
    overflowY: "auto",
    overscrollBehaviorY: "contain",
    // The scroller reaches into the gutter by the row bleed so bleeding fills
    // (list rows, hanging fields) are not clipped at the text edge.
    marginInline: `calc(-1 * ${space.s3})`,
    paddingInline: space.s3,
  },
  actions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
    "--ghost-hang": 0,
  },
  description: {
    margin: 0,
    color: color.fgMuted,
    maxWidth: "60ch",
    textWrap: "pretty",
  },
  toolbar: {
    // A toolbar starts its row: a leading borderless button keeps the text edge.
    "--ghost-hang": 1,
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  toolbarLead: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    flexGrow: 1,
    minWidth: 0,
  },
  title: {
    margin: 0,
    fontWeight: weight.semibold,
    color: color.accentStrong,
    flexGrow: 1,
    minWidth: 0,
    overflowWrap: "anywhere",
    textWrap: "balance",
  },
  row: {
    display: "grid",
    alignContent: "start",
    gap: layout.contentGap,
    paddingInline: rowInset.inline,
    paddingBlock: layout.rowPadding,
    minWidth: 0,
    overflowWrap: "anywhere",
  },
  filters: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "flex-start",
    gap: layout.controlGap,
    minWidth: 0,
  },
  filterLead: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    flex: "1 1 20rem",
    minWidth: 0,
  },
  // The search starts its column: its glyph keeps the column edge and its fill
  // bleeds by the same inset as list rows (see RecordGroup).
  filterSearch: {
    flex: "1 1 12rem",
    minWidth: 0,
    marginInlineStart: `calc(-1 * ${space.s3})`,
  },
  hang: {
    display: "flex",
    minWidth: 0,
    marginInlineStart: `calc(-1 * ${space.s3})`,
  },
  filterActions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    flexShrink: 0,
    maxWidth: "100%",
  },
  filterOptions: {
    display: "flex",
    flexWrap: "wrap",
    gap: layout.controlGap,
    alignItems: "center",
    minWidth: 0,
  },
  meta: {
    margin: 0,
    color: color.fgMuted,
    minWidth: 0,
    overflowWrap: "anywhere",
    textWrap: "pretty",
  },
  context: { marginTop: space.s2, minWidth: 0 },
  hostedContext: { marginTop: 0 },
  section: {
    display: "flex",
    flexDirection: "column",
    gap: layout.contentGap,
    minWidth: 0,
  },
  sectionHeading: {
    margin: 0,
    fontWeight: weight.semibold,
    color: color.fgMuted,
    minWidth: 0,
  },
  sectionHead: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "space-between",
    columnGap: space.s4,
    rowGap: space.s2,
    minWidth: 0,
  },
  sectionCopy: {
    display: "grid",
    gap: space.s1,
    minWidth: 0,
    flex: "1 1 12rem",
  },
  footer: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "space-between",
    gap: layout.contentGap,
    minWidth: 0,
    paddingBlockStart: space.s1,
  },
});

type PanelDensity = "compact" | "page";

const PanelHostContext = createContext<PanelDensity | null>(null);

export function PanelHost({
  density = "compact",
  children,
}: {
  density?: PanelDensity;
  children: ReactNode;
}) {
  return <PanelHostContext value={density}>{children}</PanelHostContext>;
}

export function Panel({
  children,
  label,
  header,
  controls,
  bodyViewportRef,
  bodyViewportLabelledBy,
  ref,
  className,
  ...props
}: Omit<ComponentPropsWithRef<"section">, "children"> & {
  children: ReactNode;
  label: string;
  header?: ReactNode;
  controls?: ReactNode;
  bodyViewportRef?: Ref<HTMLDivElement>;
  bodyViewportLabelledBy?: string;
}) {
  const bounded = bodyViewportRef != null;
  const namedBody = bounded && bodyViewportLabelledBy != null;
  const styled = stylex.props(
    styles.panel,
    type.uiMd,
    bounded && styles.viewportPanel,
  );
  const compact = useContext(PanelHostContext) === "compact";
  return (
    <section
      {...props}
      ref={ref}
      data-slot="panel"
      aria-label={label}
      {...styled}
      className={clsx(styled.className, className)}
    >
      <div
        {...stylex.props(
          styles.content,
          compact && styles.compactContent,
          bounded && styles.viewportContent,
        )}
      >
        {header}
        <HeadingLevel>
          {controls != null && (
            <div data-slot="panel-controls" {...stylex.props(styles.controls)}>
              {controls}
            </div>
          )}
          <div
            ref={bodyViewportRef}
            data-slot="panel-body"
            role={namedBody ? "region" : undefined}
            aria-labelledby={namedBody ? bodyViewportLabelledBy : undefined}
            tabIndex={bounded ? (namedBody ? 0 : -1) : undefined}
            {...stylex.props(styles.body, bounded && styles.bodyViewport)}
          >
            {children}
          </div>
        </HeadingLevel>
      </div>
    </section>
  );
}

type PanelTitleSize = "sm" | "md" | "lg";

export function PanelHeader({
  title,
  description,
  context,
  size = "lg",
}: {
  title: ReactNode;
  description?: ReactNode;
  context?: ReactNode;
  size?: PanelTitleSize;
}) {
  const density = useContext(PanelHostContext);
  const hosted = density !== null;
  const compact = density === "compact";
  const headingOnly = !description && !context;
  const header = stylex.props(styles.header);
  return (
    <header
      data-slot="panel-header"
      {...header}
      className={clsx(header.className, hosted && headingOnly && "sr-only")}
    >
      <PanelTitle
        size={size}
        className={hosted && !headingOnly ? "sr-only" : undefined}
      >
        {title}
      </PanelTitle>
      {description && (
        <p
          {...stylex.props(styles.description, compact ? type.uiSm : type.uiMd)}
        >
          {description}
        </p>
      )}
      {context && (
        <div {...stylex.props(styles.context, hosted && styles.hostedContext)}>
          {context}
        </div>
      )}
    </header>
  );
}
export function Toolbar({
  children,
  label,
  actions,
}: {
  children: ReactNode;
  label: string;
  actions?: ReactNode;
}) {
  return (
    <div role="group" aria-label={label} {...stylex.props(styles.toolbar)}>
      {actions ? (
        <>
          {/* Growing content, not an auto margin, carries actions to the
              trailing edge, so a wrapped action row keeps the leading edge. */}
          <div {...stylex.props(styles.toolbarLead)}>{children}</div>
          <div {...stylex.props(styles.actions)}>{actions}</div>
        </>
      ) : (
        children
      )}
    </div>
  );
}
export function FilterBar({
  label,
  search,
  actions,
  children,
}: {
  label: string;
  search: ReactNode;
  actions?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div role="group" aria-label={label} {...stylex.props(styles.filters)}>
      <div {...stylex.props(styles.filterLead)}>
        <div {...stylex.props(styles.filterSearch)}>{search}</div>
        {actions && (
          <div {...stylex.props(styles.filterActions)}>{actions}</div>
        )}
      </div>
      {children && (
        <div {...stylex.props(styles.filterOptions)}>{children}</div>
      )}
    </div>
  );
}
/**
 * Starts a column with a filled control (segmented, select) so its text, not its
 * fill, sits on the column edge; the fill bleeds by the list rows' inset.
 */
export function Hang({ children }: { children: ReactNode }) {
  return <div {...stylex.props(styles.hang)}>{children}</div>;
}
export function PanelTitle({
  children,
  size = "sm",
  className,
  ...props
}: ComponentPropsWithRef<"h1"> & { size?: PanelTitleSize }) {
  const Tag = useHeadingTag();
  const styled = stylex.props(
    styles.title,
    size === "lg"
      ? type.displayLg
      : size === "md"
        ? type.displayMd
        : type.displaySm,
  );
  return (
    <Tag {...props} {...styled} className={clsx(styled.className, className)}>
      {children}
    </Tag>
  );
}

export function PanelSection({
  title,
  description,
  actions,
  children,
  className,
  ...props
}: Omit<ComponentPropsWithRef<"section">, "title"> & {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  const Tag = useHeadingTag();
  const styled = stylex.props(styles.section);
  return (
    <section
      {...props}
      {...styled}
      className={clsx(styled.className, className)}
    >
      <div {...stylex.props(styles.sectionHead)}>
        <div {...stylex.props(styles.sectionCopy)}>
          <Tag {...stylex.props(styles.sectionHeading, type.uiSm)}>{title}</Tag>
          {description && <Metadata>{description}</Metadata>}
        </div>
        {actions && <div {...stylex.props(styles.actions)}>{actions}</div>}
      </div>
      <HeadingLevel>{children}</HeadingLevel>
    </section>
  );
}

export function PanelFooter({ children }: { children: ReactNode }) {
  return <footer {...stylex.props(styles.footer)}>{children}</footer>;
}

export function ListRow({
  children,
  variant = "row",
  className,
  ...props
}: ComponentPropsWithRef<"article"> & { variant?: "row" | "card" }) {
  const styled = stylex.props(
    styles.row,
    variant === "card" && CARD_SURFACE.face,
  );
  return (
    <article
      {...props}
      {...styled}
      className={clsx(styled.className, className)}
    >
      {children}
    </article>
  );
}
export function Metadata({
  children,
  className,
  ...props
}: ComponentPropsWithRef<"p">) {
  const styled = stylex.props(styles.meta, type.uiSm);
  return (
    <p {...props} {...styled} className={clsx(styled.className, className)}>
      {children}
    </p>
  );
}
