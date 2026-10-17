import * as stylex from "@stylexjs/stylex";
import { useId, type ComponentPropsWithRef, type ReactNode } from "react";
import { CARD_SURFACE } from "./control-surface";
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

const styles = stylex.create({
  panel: {
    width: "100%",
    containerType: "inline-size",
    maxWidth: layout.pageWidth,
    marginInline: "auto",
    minHeight: "100%",
    color: color.fg,
    backgroundColor: surface.canvas,
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
  header: {
    display: "grid",
    gap: layout.controlGap,
    minWidth: 0,
  },
  headerLine: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    columnGap: space.s4,
    rowGap: layout.contentGap,
    minWidth: 0,
  },
  actions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  description: {
    margin: 0,
    color: color.fgMuted,
    maxWidth: "60ch",
    textWrap: "pretty",
  },
  toolbar: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    minWidth: 0,
  },
  title: {
    margin: 0,
    fontWeight: weight.semibold,
    flexGrow: 1,
    minWidth: 0,
    overflowWrap: "anywhere",
    textWrap: "balance",
  },
  row: {
    display: "grid",
    alignContent: "start",
    gap: layout.contentGap,
    paddingInline: layout.cardPadding,
    paddingBlock: layout.rowPadding,
    minWidth: 0,
    overflowWrap: "anywhere",
  },
  filters: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "stretch",
    gap: layout.controlGap,
    minWidth: 0,
  },
  filterSearch: { flex: "1 1 14rem", minWidth: 0 },
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
  form: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.sectionGap,
    minWidth: 0,
  },
  recordForm: {
    paddingInline: layout.cardPadding,
    paddingBlock: layout.cardPadding,
  },
  field: {
    display: "grid",
    alignContent: "start",
    gap: layout.labelGap,
    minWidth: 0,
  },
  fieldLabel: { color: color.fgSoft, fontWeight: weight.medium },
  context: { marginTop: space.s2, minWidth: 0 },
  section: { display: "grid", gap: layout.contentGap, minWidth: 0 },
  sectionHeading: { margin: 0, fontWeight: weight.medium, minWidth: 0 },
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
  formRow: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,12rem),1fr))",
    alignItems: "start",
    gap: layout.fieldGap,
    minWidth: 0,
  },
  formSection: { margin: 0, padding: 0, borderWidth: 0, minWidth: 0 },
  legend: {
    padding: 0,
    marginBlockEnd: layout.contentGap,
    color: color.fg,
    fontWeight: weight.medium,
  },
  formFields: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.fieldGap,
    minWidth: 0,
  },
  formActions: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-end",
    flexWrap: "wrap",
    gap: layout.controlGap,
    minWidth: 0,
    paddingBlockStart: layout.fieldGap,
    borderBlockStartWidth: "var(--control-edge-width)",
    borderBlockStartStyle: "solid",
    borderBlockStartColor: surface.field,
  },
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
      <div {...stylex.props(styles.content)}>{children}</div>
    </section>
  );
}
export function PanelHeader({
  title,
  description,
  context,
  actions,
}: {
  title: ReactNode;
  description?: ReactNode;
  context?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header {...stylex.props(styles.header)}>
      <div {...stylex.props(styles.headerLine)}>
        <PanelTitle size="lg">{title}</PanelTitle>
        {actions && <div {...stylex.props(styles.actions)}>{actions}</div>}
      </div>
      {description && (
        <p {...stylex.props(styles.description, type.uiMd)}>{description}</p>
      )}
      {context && <div {...stylex.props(styles.context)}>{context}</div>}
    </header>
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
export function FilterBar({
  label,
  search,
  children,
}: {
  label: string;
  search: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div role="group" aria-label={label} {...stylex.props(styles.filters)}>
      <div {...stylex.props(styles.filterSearch)}>{search}</div>
      {children && (
        <div {...stylex.props(styles.filterOptions)}>{children}</div>
      )}
    </div>
  );
}
export function PanelTitle({
  children,
  size = "sm",
  ...props
}: ComponentPropsWithRef<"h1"> & { size?: "sm" | "md" | "lg" }) {
  return (
    <h1
      {...props}
      {...stylex.props(
        styles.title,
        size === "lg"
          ? type.displayLg
          : size === "md"
            ? type.displayMd
            : type.displaySm,
      )}
    >
      {children}
    </h1>
  );
}

export function PanelSection({
  title,
  description,
  actions,
  children,
  ...props
}: Omit<ComponentPropsWithRef<"section">, "title"> & {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <section {...props} {...stylex.props(styles.section)}>
      <div {...stylex.props(styles.sectionHead)}>
        <div {...stylex.props(styles.sectionCopy)}>
          <h2 {...stylex.props(styles.sectionHeading, type.uiMd)}>{title}</h2>
          {description && <Metadata>{description}</Metadata>}
        </div>
        {actions && <div {...stylex.props(styles.actions)}>{actions}</div>}
      </div>
      {children}
    </section>
  );
}

export function PanelFooter({ children }: { children: ReactNode }) {
  return <footer {...stylex.props(styles.footer)}>{children}</footer>;
}

export function FormRow({ children }: { children: ReactNode }) {
  return <div {...stylex.props(styles.formRow)}>{children}</div>;
}

export function FormSection({
  title,
  description,
  children,
}: {
  title: string;
  description?: ReactNode;
  children: ReactNode;
}) {
  return (
    <fieldset {...stylex.props(styles.formSection)}>
      <legend {...stylex.props(styles.legend, type.uiMd)}>{title}</legend>
      <div {...stylex.props(styles.formFields)}>
        {description && <Metadata>{description}</Metadata>}
        {children}
      </div>
    </fieldset>
  );
}

export function FormActions({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div role="group" aria-label={label} {...stylex.props(styles.formActions)}>
      {children}
    </div>
  );
}
export function ListRow({
  children,
  variant = "row",
  ...props
}: ComponentPropsWithRef<"article"> & { variant?: "row" | "card" }) {
  return (
    <article
      {...props}
      {...stylex.props(styles.row, variant === "card" && CARD_SURFACE.face)}
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

export function Form({ children, ...props }: ComponentPropsWithRef<"form">) {
  return (
    <form {...props} {...stylex.props(styles.form)}>
      {children}
    </form>
  );
}

export function RecordForm({
  children,
  ...props
}: ComponentPropsWithRef<"section">) {
  return (
    <section
      {...props}
      {...stylex.props(CARD_SURFACE.face, styles.form, styles.recordForm)}
    >
      {children}
    </section>
  );
}

export function FormField({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <label {...stylex.props(styles.field)}>
      <span {...stylex.props(styles.fieldLabel, type.uiSm)}>{label}</span>
      {children}
    </label>
  );
}

export function FormGroup({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  const labelId = useId();
  return (
    <div role="group" aria-labelledby={labelId} {...stylex.props(styles.field)}>
      <span id={labelId} {...stylex.props(styles.fieldLabel, type.uiSm)}>
        {label}
      </span>
      {children}
    </div>
  );
}
