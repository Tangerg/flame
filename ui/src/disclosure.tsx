import * as stylex from "@stylexjs/stylex";
import {
  useContext,
  useLayoutEffect,
  useRef,
  type ComponentPropsWithRef,
  type ReactNode,
} from "react";
import { ChevronRight } from "@flame/icons/react";
import { CollapsiblePrimitive } from "./primitives/collapsible";
import { Button } from "./button";
import { Icon } from "./font-scale";
import { layout, space, surface, type } from "./tokens.stylex";
import { PanelLayoutContext } from "./panel-layout";

const styles = stylex.create({
  root: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    minWidth: 0,
    backgroundColor: {
      default: "transparent",
      ":is([data-focused=true])": surface.selected,
    },
  },
  header: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    columnGap: layout.contentGap,
    rowGap: layout.controlGap,
    minWidth: 0,
  },
  stickyHeader: {
    position: { default: null, ":is([data-sticky])": "sticky" },
    insetBlockStart: {
      default: null,
      ":is([data-sticky])": "var(--panel-controls-height, 0px)",
    },
    zIndex: {
      default: null,
      ":is([data-sticky])": "calc(var(--layer-card) - 1)",
    },
    backgroundColor: {
      default: null,
      ":is([data-sticky])": surface.canvas,
    },
  },
  stickyRowHeader: {
    backgroundColor: {
      default: null,
      ":is([data-sticky])": surface.card,
    },
  },
  row: {
    paddingInline: layout.cardPadding,
    paddingBlock: space.s3,
  },
  rowTrigger: {
    marginInline: `calc(-1 * ${space.s2})`,
    paddingInline: space.s2,
    maxWidth: "none",
    borderWidth: 0,
    lineHeight: "inherit",
  },
  trigger: {
    width: "auto",
    flexGrow: 1,
    flexShrink: 1,
    flexBasis: "auto",
    minWidth: 0,
    height: "auto",
    justifyContent: "flex-start",
    textAlign: "start",
    whiteSpace: "normal",
    overflowWrap: "anywhere",
    paddingBlock: space.s1,
    paddingInline: 0,
  },
  glyph: {
    flexShrink: 0,
    transform: {
      default: "rotate(0deg)",
      ":dir(rtl)": "rotate(180deg)",
      ":is([data-panel-open] > *)": "rotate(90deg)",
      ":is([data-panel-open] > *):dir(rtl)": "rotate(90deg)",
    },
  },
  title: { minWidth: 0, flex: 1, overflowWrap: "anywhere" },
  content: {
    display: "grid",
    gridTemplateColumns: "minmax(0,1fr)",
    gap: layout.controlGap,
    paddingBlock: space.s3,
  },
  rowContent: {
    paddingBlockStart: space.s4,
    marginBlockStart: space.s2,
    borderBlockStartWidth: "var(--control-edge-width)",
    borderBlockStartStyle: "solid",
    borderBlockStartColor: surface.divider,
  },
  actions: {
    display: "flex",
    flex: "0 1 auto",
    minWidth: 0,
    maxWidth: "100%",
    marginInlineStart: "auto",
    alignItems: "center",
    flexWrap: "wrap",
    gap: layout.controlGap,
    justifyContent: "flex-end",
  },
});
export function Disclosure({
  title,
  children,
  open,
  defaultOpen,
  onOpenChange,
  variant = "plain",
  actions,
  stickyHeader = false,
  ...props
}: Omit<ComponentPropsWithRef<"section">, "title" | "onChange"> & {
  title: ReactNode;
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  variant?: "plain" | "row";
  actions?: ReactNode;
  stickyHeader?: boolean;
}) {
  const headerRef = useRef<HTMLDivElement>(null);
  const panelLayout = useContext(PanelLayoutContext);
  useLayoutEffect(() => {
    const header = headerRef.current;
    const root = header?.parentElement;
    if (!stickyHeader || !header || !root || !panelLayout) return;
    let viewport = root.parentElement;
    while (
      viewport &&
      !["auto", "scroll", "hidden"].includes(
        getComputedStyle(viewport).overflowY,
      )
    ) {
      viewport = viewport.parentElement;
    }
    if (!viewport) return;
    const scrollViewport = viewport;
    const controls = panelLayout?.controlsRef?.current;
    let content: HTMLElement | null = null;
    const measure = () => {
      const nextContent = header.nextElementSibling as HTMLElement | null;
      if (content !== nextContent) {
        content?.style.removeProperty("--disclosure-header-height");
        content = nextContent;
      }
      const height = header.getBoundingClientRect().height;
      const contentStyle = content ? getComputedStyle(content) : null;
      const lineHeight = contentStyle
        ? Number.parseFloat(contentStyle.lineHeight)
        : 0;
      const bodyInset = contentStyle
        ? Number.parseFloat(contentStyle.marginBlockStart) +
          Number.parseFloat(contentStyle.paddingBlockStart)
        : 0;
      const available =
        scrollViewport.clientHeight -
        (controls?.getBoundingClientRect().height ?? 0);
      const fits =
        content !== null &&
        height > 0 &&
        lineHeight > 0 &&
        height + bodyInset + 2 * lineHeight <= available;
      const nextHeight = fits ? `${height}px` : "";
      const changed =
        (content?.style.getPropertyValue("--disclosure-header-height") ??
          "") !== nextHeight;
      header.toggleAttribute("data-sticky", fits);
      if (fits && content) {
        content.style.setProperty("--disclosure-header-height", nextHeight);
      } else {
        content?.style.removeProperty("--disclosure-header-height");
      }
      if (changed) {
        const focused = header.ownerDocument.activeElement;
        if (focused && content?.contains(focused)) {
          panelLayout?.keepBodyFocusVisible(focused);
        }
      }
    };
    const observer = new ResizeObserver(measure);
    observer.observe(header, { box: "border-box" });
    observer.observe(scrollViewport, { box: "border-box" });
    if (controls) observer.observe(controls, { box: "border-box" });
    // Uncontrolled expansion can mount the body without rerendering Disclosure.
    observer.observe(root, { box: "border-box" });
    measure();
    return () => {
      observer.disconnect();
      header.removeAttribute("data-sticky");
      content?.style.removeProperty("--disclosure-header-height");
    };
  }, [stickyHeader, panelLayout]);
  return (
    <CollapsiblePrimitive.Root
      open={open}
      defaultOpen={defaultOpen}
      onOpenChange={onOpenChange}
      render={
        <section
          {...props}
          {...stylex.props(styles.root, variant === "row" && styles.row)}
        />
      }
    >
      <div
        ref={headerRef}
        {...stylex.props(
          styles.header,
          stickyHeader && styles.stickyHeader,
          stickyHeader && variant === "row" && styles.stickyRowHeader,
        )}
      >
        <CollapsiblePrimitive.Trigger
          render={
            <Button
              variant="ghost"
              size="sm"
              press={variant === "row" ? "none" : undefined}
              styles={
                variant === "row"
                  ? [styles.trigger, styles.rowTrigger]
                  : styles.trigger
              }
            />
          }
        >
          {variant === "plain" && (
            <Icon
              glyph={ChevronRight}
              size="xs"
              {...stylex.props(styles.glyph)}
            />
          )}
          <span {...stylex.props(styles.title, type.uiSm)}>{title}</span>
          {variant === "row" && (
            <Icon
              glyph={ChevronRight}
              size="xs"
              {...stylex.props(styles.glyph)}
            />
          )}
        </CollapsiblePrimitive.Trigger>
        {actions && <div {...stylex.props(styles.actions)}>{actions}</div>}
      </div>
      <CollapsiblePrimitive.Panel
        {...stylex.props(
          styles.content,
          variant === "row" && styles.rowContent,
          type.uiSm,
        )}
      >
        {children}
      </CollapsiblePrimitive.Panel>
    </CollapsiblePrimitive.Root>
  );
}
