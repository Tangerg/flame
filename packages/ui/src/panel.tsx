import * as stylex from "@stylexjs/stylex";
import {
  useId,
  createContext,
  useContext,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  type AriaAttributes,
  type ComponentPropsWithRef,
  type ReactNode,
} from "react";
import { CARD_SURFACE } from "./control-surface";
import { PanelLayoutContext } from "./panel-layout";
import { useContentVisible } from "./content-visibility";
import { readingUnitFor } from "./reading-unit";
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
  header: {
    display: "grid",
    gap: layout.controlGap,
    minWidth: 0,
  },
  controls: {
    position: "sticky",
    insetBlockStart: 0,
    zIndex: "var(--layer-card)",
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
  toolbarActions: { marginInlineStart: "auto" },
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
  filterLead: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    flex: "1 1 20rem",
    minWidth: 0,
  },
  filterSearch: { flex: "1 1 12rem", minWidth: 0 },
  filterActions: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: layout.controlGap,
    flexShrink: 0,
    maxWidth: "100%",
    marginInlineStart: "auto",
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
    color: color.fgMuted,
    fontWeight: weight.semibold,
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
    flexWrap: "wrap",
    gap: layout.contentGap,
    minWidth: 0,
  },
  formActionsUnit: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.sectionGap,
    minWidth: 0,
  },
  formActionGroup: { maxWidth: "100%" },
  formMainActions: { marginInlineStart: "auto", justifyContent: "flex-end" },
});

type PanelDensity = "compact" | "page";

const PanelHostContext = createContext<PanelDensity | null>(null);

interface PanelFocusFrame {
  top: number;
  bottom: number;
  width: number;
  contentHeight: number;
  unit: Element;
}

function panelViewport(panel: HTMLElement) {
  const bounds = panel.getBoundingClientRect();
  let width = bounds.width;
  let top = 0;
  let bottom = panel.ownerDocument.documentElement.clientHeight;
  for (
    let parent = panel.parentElement;
    parent;
    parent = parent.parentElement
  ) {
    if (getComputedStyle(parent).overflowY !== "visible") {
      const clipped = parent.getBoundingClientRect();
      top = Math.max(top, clipped.top);
      bottom = Math.min(bottom, clipped.bottom);
      width = Math.min(width, clipped.width);
    }
  }
  return { top, bottom, width, contentHeight: bounds.height };
}

function withinPanelFrame(bounds: DOMRect, frame: PanelFocusFrame) {
  // Native page zoom reports fractional CSS pixels at a rounded scroll offset.
  return (
    frame.bottom > frame.top &&
    bounds.top >= frame.top - 0.5 &&
    (bounds.height > frame.bottom - frame.top
      ? bounds.top < frame.bottom
      : bounds.bottom <= frame.bottom + 0.5)
  );
}

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
  ref,
  className,
  ...props
}: Omit<ComponentPropsWithRef<"section">, "children"> & {
  children: ReactNode;
  label: string;
  header?: ReactNode;
  controls?: ReactNode;
}) {
  const styled = stylex.props(styles.panel, type.uiMd);
  const compact = useContext(PanelHostContext) === "compact";
  const panelRef = useRef<HTMLElement>(null);
  const controlsRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const pressedTarget = useRef<Element | null>(null);
  const viewportPositions = useRef(new Map<HTMLElement, number>());
  const readingFocus = useRef<{
    target: Element;
    frame: PanelFocusFrame;
  } | null>(null);
  const visible = useContentVisible();
  const hasControls = Boolean(controls);
  const bodyFocusFrame = useCallback(
    (target: Element | null) => {
      const panel = panelRef.current;
      const controls = controlsRef.current;
      const body = bodyRef.current;
      if (!visible || !panel || !target || !body?.contains(target)) return null;
      const viewport = panelViewport(panel);
      const stickyHeaderHeight =
        Number.parseFloat(
          getComputedStyle(target).getPropertyValue(
            "--disclosure-header-height",
          ),
        ) || 0;
      const focusTop =
        Math.max(
          viewport.top,
          controls?.getBoundingClientRect().bottom ?? viewport.top,
        ) + stickyHeaderHeight;
      const focusUnit = readingUnitFor(target, body, (unit) => {
        const scrollMargin =
          Number.parseFloat(getComputedStyle(unit).scrollMarginBlockStart) || 0;
        const height = unit.getBoundingClientRect().height;
        return (
          height > 0 &&
          height <=
            viewport.bottom - Math.max(focusTop, viewport.top + scrollMargin)
        );
      });
      return {
        ...viewport,
        top: focusTop,
        unit: focusUnit,
      };
    },
    [visible],
  );
  const keepBodyFocusVisible = useCallback(
    (target: Element | null) => {
      const frame = bodyFocusFrame(target);
      if (!frame || !target) return null;
      const pressed = pressedTarget.current;
      const pointerFocus =
        pressed &&
        (target.contains(pressed) ||
          pressed.closest("label, [data-focus-proxy]")?.contains(target));
      // Moving a pressed control before pointerup can swallow its activation.
      if (
        !pointerFocus &&
        frame.bottom > frame.top &&
        !withinPanelFrame(frame.unit.getBoundingClientRect(), frame)
      ) {
        frame.unit.scrollIntoView({ block: "start", inline: "nearest" });
        // Its queued scroll event must not retire this owner's reflow witness.
        for (const viewport of viewportPositions.current.keys())
          viewportPositions.current.set(viewport, viewport.scrollTop);
      }
      return frame;
    },
    [bodyFocusFrame],
  );
  const panelLayout = useMemo(
    () => ({
      controlsRef: hasControls ? controlsRef : null,
      keepBodyFocusVisible,
    }),
    [hasControls, keepBodyFocusVisible],
  );
  useLayoutEffect(() => {
    const panel = panelRef.current;
    if (!panel) return;
    const controls = controlsRef.current;
    const measure = () => {
      panel.style.setProperty(
        "--panel-controls-height",
        `${controls?.getBoundingClientRect().height ?? 0}px`,
      );
    };
    measure();
    if (!controls) return;
    const observer = new ResizeObserver(measure);
    observer.observe(controls, { box: "border-box" });
    return () => {
      observer.disconnect();
      panel.style.setProperty("--panel-controls-height", "0px");
    };
  }, [hasControls]);
  useLayoutEffect(() => {
    const panel = panelRef.current;
    const body = bodyRef.current;
    if (!visible || !panel || !body) return;
    const measure = () => {
      const viewport = panelViewport(panel);
      panel.style.setProperty(
        "--reading-frame-height",
        `${Math.max(0, viewport.bottom - viewport.top)}px`,
      );
    };
    measure();
    const document = panel.ownerDocument;
    const initial = document.activeElement;
    const initialFrame = keepBodyFocusVisible(initial);
    readingFocus.current =
      initial && initialFrame ? { target: initial, frame: initialFrame } : null;
    const scrollPositions = viewportPositions.current;
    for (
      let parent = panel.parentElement;
      parent;
      parent = parent.parentElement
    ) {
      if (getComputedStyle(parent).overflowY !== "visible")
        scrollPositions.set(parent, parent.scrollTop);
    }
    const reveal = () => {
      measure();
      const reading = readingFocus.current;
      if (!reading) return;
      if (
        reading.target !== document.activeElement ||
        !body.contains(reading.target)
      ) {
        readingFocus.current = null;
        return;
      }
      const frame = keepBodyFocusVisible(reading.target);
      if (frame) reading.frame = frame;
    };
    const retire = () => {
      readingFocus.current = null;
    };
    const clearPress = () => {
      pressedTarget.current = null;
    };
    const onScroll = (event: Event) => {
      const viewport = event.currentTarget as HTMLElement;
      const moved = viewport.scrollTop !== scrollPositions.get(viewport);
      scrollPositions.set(viewport, viewport.scrollTop);
      const reading = readingFocus.current;
      if (!moved || !reading) return;
      const frame = bodyFocusFrame(reading.target);
      if (!frame) return;
      // Reflow and scroll anchoring can notify before ResizeObserver delivery.
      if (
        frame.top !== reading.frame.top ||
        frame.bottom !== reading.frame.bottom ||
        frame.width !== reading.frame.width ||
        frame.contentHeight !== reading.frame.contentHeight
      )
        return;
      if (!withinPanelFrame(reading.target.getBoundingClientRect(), frame))
        retire();
    };
    const observer = new ResizeObserver(reveal);
    observer.observe(panel, { box: "border-box" });
    if (controlsRef.current)
      observer.observe(controlsRef.current, { box: "border-box" });
    for (const viewport of scrollPositions.keys()) {
      observer.observe(viewport, { box: "border-box" });
      viewport.addEventListener("scroll", onScroll, { passive: true });
    }
    const wheelScope = scrollPositions.keys().next().value ?? panel;
    wheelScope.addEventListener("wheel", retire, {
      passive: true,
      capture: true,
    });
    document.addEventListener("pointerup", clearPress, true);
    document.addEventListener("pointercancel", clearPress, true);
    document.defaultView?.addEventListener("resize", reveal);
    return () => {
      observer.disconnect();
      for (const viewport of scrollPositions.keys())
        viewport.removeEventListener("scroll", onScroll);
      wheelScope.removeEventListener("wheel", retire, true);
      document.removeEventListener("pointerup", clearPress, true);
      document.removeEventListener("pointercancel", clearPress, true);
      document.defaultView?.removeEventListener("resize", reveal);
      panel.style.removeProperty("--reading-frame-height");
      scrollPositions.clear();
      retire();
      clearPress();
    };
  }, [visible, hasControls, bodyFocusFrame, keepBodyFocusVisible]);
  return (
    <section
      {...props}
      ref={(node) => {
        panelRef.current = node;
        if (typeof ref === "function") return ref(node);
        if (ref) ref.current = node;
      }}
      data-slot="panel"
      aria-label={label}
      {...styled}
      className={clsx(styled.className, className)}
    >
      <div {...stylex.props(styles.content, compact && styles.compactContent)}>
        {header}
        <HeadingLevel>
          {hasControls && (
            <div
              ref={controlsRef}
              data-slot="panel-controls"
              {...stylex.props(styles.controls)}
            >
              {controls}
            </div>
          )}
          <div
            ref={bodyRef}
            data-slot="panel-body"
            onPointerDownCapture={(event) => {
              pressedTarget.current =
                event.target instanceof Element ? event.target : null;
            }}
            onFocus={(event) => {
              const frame = keepBodyFocusVisible(event.target);
              readingFocus.current = frame
                ? { target: event.target, frame }
                : null;
            }}
            onBlurCapture={(event) => {
              const next = event.relatedTarget;
              if (!(
                next instanceof Node && event.currentTarget.contains(next)
              )) {
                pressedTarget.current = null;
                readingFocus.current = null;
              }
            }}
            {...stylex.props(styles.body)}
          >
            <PanelLayoutContext value={panelLayout}>
              {children}
            </PanelLayoutContext>
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
      {children}
      {actions && (
        <div {...stylex.props(styles.actions, styles.toolbarActions)}>
          {actions}
        </div>
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
      <legend {...stylex.props(styles.legend, type.uiSm)}>{title}</legend>
      <div {...stylex.props(styles.formFields)}>
        {description && <Metadata>{description}</Metadata>}
        {children}
      </div>
    </fieldset>
  );
}

export function FormActions({
  label,
  auxiliaryAction,
  feedback,
  children,
}: {
  label: string;
  auxiliaryAction?: ReactNode;
  feedback?: ReactNode;
  children: ReactNode;
}) {
  const groupRef = useRef<HTMLDivElement>(null);
  const panelLayout = useContext(PanelLayoutContext);
  useEffect(() => {
    const group = groupRef.current;
    if (!feedback || !panelLayout || !group) return;
    // Asynchronous results arrive without a focus event, so the retained
    // action focus is the only signal that the reader is still here.
    const focused = group.ownerDocument.activeElement;
    if (group.contains(focused)) panelLayout.keepBodyFocusVisible(focused);
  }, [feedback, panelLayout]);
  return (
    <div
      ref={groupRef}
      role="group"
      aria-label={label}
      data-focus-unit=""
      {...stylex.props(styles.formActionsUnit)}
    >
      <div {...stylex.props(styles.formActions)}>
        {auxiliaryAction && (
          <div {...stylex.props(styles.actions, styles.formActionGroup)}>
            {auxiliaryAction}
          </div>
        )}
        <div
          {...stylex.props(
            styles.actions,
            styles.formActionGroup,
            styles.formMainActions,
          )}
        >
          {children}
        </div>
      </div>
      {feedback}
    </div>
  );
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

export function Form({
  children,
  className,
  ...props
}: ComponentPropsWithRef<"form">) {
  const styled = stylex.props(styles.form);
  return (
    <form {...props} {...styled} className={clsx(styled.className, className)}>
      {children}
    </form>
  );
}

export function RecordForm({
  children,
  opener,
  ref,
  className,
  ...props
}: ComponentPropsWithRef<"section"> & {
  opener?: HTMLElement | (() => HTMLElement | null);
}) {
  const element = useRef<HTMLElement>(null);
  const ownedFocus = useRef(false);
  useLayoutEffect(() => {
    const form = element.current;
    if (!form || !opener) return;
    ownedFocus.current = false;
    return () => {
      ownedFocus.current = form.contains(form.ownerDocument.activeElement);
    };
  }, [opener]);
  useEffect(() => {
    const form = element.current;
    if (!form || !opener) return;
    return () => {
      // Passive cleanup runs after the opener's disabled state commits.
      if (
        !form.isConnected &&
        ownedFocus.current &&
        form.ownerDocument.activeElement === form.ownerDocument.body
      ) {
        const target = typeof opener === "function" ? opener() : opener;
        if (target?.isConnected) target.focus();
      }
    };
  }, [opener]);
  const styled = stylex.props(
    CARD_SURFACE.face,
    styles.form,
    styles.recordForm,
  );
  return (
    <section
      {...props}
      ref={(node) => {
        element.current = node;
        if (typeof ref === "function") return ref(node);
        if (ref) ref.current = node;
      }}
      {...styled}
      className={clsx(styled.className, className)}
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
  ...props
}: {
  label: string;
  children: ReactNode;
} & Pick<AriaAttributes, "aria-invalid" | "aria-describedby">) {
  const labelId = useId();
  return (
    <div
      {...props}
      role="group"
      aria-labelledby={labelId}
      data-focus-unit=""
      {...stylex.props(styles.field)}
    >
      <span id={labelId} {...stylex.props(styles.fieldLabel, type.uiSm)}>
        {label}
      </span>
      {children}
    </div>
  );
}
