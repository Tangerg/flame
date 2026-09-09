import * as stylex from "@stylexjs/stylex";
import {
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
  type Ref,
} from "react";
import { cn } from "@/lib/classNames";
import { color, motion, space, surface, type, weight } from "@/styles/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";
import { ContextMenu } from "@/ui/atoms/menu";
import { IconButton } from "@/ui/atoms/icon-button";
import { reveal } from "@/ui/atoms/reveal";
import { ResizeHandle, type ResizeHandleProps } from "@/ui/atoms/resize-handle";
import { TabsPrimitive } from "@/ui/primitives";

export interface AgentDockTab {
  id: string;
  title: ReactNode;
  icon?: IconName;
  badge?: ReactNode;
  active?: boolean;
  onSelect?: () => void;
  onClose?: () => void;
  closeLabel?: string;
  onCloseOthers?: () => void;
  closeOthersLabel?: string;
  onCloseAll?: () => void;
  closeAllLabel?: string;
}

export interface AgentDockTabsProps {
  tabs: AgentDockTab[];
  ariaLabel: string;
  onReorder?: (id: string, toIndex: number) => void;
}

const styles = stylex.create({
  // The strip's own row: it holds the tabs and the resizer side by side.
  row: { display: "flex", minHeight: 0, flex: 1 },
  tab: {
    display: "flex",
    height: "var(--dock-tab-height)",
    minWidth: 0,
    flexShrink: 0,
    alignItems: "center",
    borderRadius: "var(--dock-tab-radius)",
    color: {
      default: color.fgMuted,
      ":hover": color.fg,
      ":focus-within": color.fg,
      ":is([data-active])": color.fg,
    },
    backgroundColor: {
      default: null,
      ":hover": surface.hover,
      ":is([data-active])": "var(--dock-tab-active-surface)",
      ":is([data-active]):hover": "var(--dock-tab-active-hover-surface)",
    },
    // A tab being dragged steps back so the gap it will leave is legible.
    opacity: { default: null, ":is([data-dragging])": 0.5 },
    transitionProperty: "background-color, color, opacity",
    transitionDuration: motion.color,
    transitionTimingFunction: "var(--ease-out)",
  },
  // The label is capped so one long title cannot take the strip; the corner is inherited
  // because the tab and its label are one shape.
  label: {
    display: "inline-flex",
    height: "100%",
    minWidth: 0,
    maxWidth: "calc(var(--spacing) * 40)",
    alignItems: "center",
    gap: space.s1_5,
    borderRadius: "inherit",
    borderWidth: 0,
    backgroundColor: "transparent",
    paddingBlock: 0,
    fontWeight: weight.regular,
    color: "inherit",
  },
  labelClosable: { paddingLeft: space.s2, paddingRight: space.s1 },
  labelPlain: { paddingInline: space.s2 },
  glyph: { flexShrink: 0, opacity: "var(--glyph-step)" },
  /** The close control sits inside the tab's own inset rather than against its edge. */
  tabClose: { marginRight: space.s0_5 },
  title: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  badge: { flexShrink: 0, fontFamily: "var(--font-mono)", lineHeight: 1, color: color.fgFaint },
  // The list adds no box: the strip already is one, and a second would put the tabs a nesting
  // step away from the padding that positions them.
  contents: { display: "contents" },
});

export function AgentContextDock({ children }: { children: ReactNode }) {
  return <aside className="agent-context-dock pane-split">{children}</aside>;
}

/** Lays the transcript, the resizer and the dock side by side. The collapsed state lives here
 *  rather than on the dock because descendant rules — the dock's own slide and the header's
 *  end padding — both read it. */
export function AgentDockRow({
  open,
  ref,
  style,
  children,
}: {
  open: boolean;
  ref?: Ref<HTMLDivElement>;
  style?: CSSProperties;
  children: ReactNode;
}) {
  return (
    <div
      ref={ref}
      className={cn("agent-dock-row", stylex.props(styles.row).className)}
      data-dock={open ? "open" : "collapsed"}
      style={style}
    >
      {children}
    </div>
  );
}

/** Sits on the dock's inner edge, so the edge and the placement belong to the dock; the caller
 *  brings only the geometry it stores. */
export function AgentDockResizer(props: Omit<ResizeHandleProps, "edge" | "className">) {
  return <ResizeHandle {...props} edge="start" className="agent-pane-resizer" />;
}

/** The dock element inside a row. Exported so the class stays this file's alone: a consumer
 *  keeping its own copy of the selector goes silently blind when the class moves. */
export function agentDockElement(row: HTMLElement): HTMLElement | null {
  return row.querySelector<HTMLElement>(".agent-context-dock");
}

function reflectDockTabOverflow(element: HTMLElement): void {
  const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
  element.toggleAttribute("data-overflow-start", element.scrollLeft > 1);
  element.toggleAttribute("data-overflow-end", maxScrollLeft - element.scrollLeft > 1);
}

function keepActiveDockTabInsideFade(element: HTMLElement): void {
  if (element.clientWidth === 0) return;
  const active = element.querySelector<HTMLElement>('[role="tab"][data-active]');
  if (!active) return;
  const stripBox = element.getBoundingClientRect();
  const activeBox = active.getBoundingClientRect();
  const edgeHint = 16;
  if (activeBox.left < stripBox.left + edgeHint) {
    element.scrollLeft -= stripBox.left + edgeHint - activeBox.left;
  } else if (activeBox.right > stripBox.right - edgeHint) {
    element.scrollLeft += activeBox.right - (stripBox.right - edgeHint);
  }
}

export function AgentDockTabs({ tabs, ariaLabel, onReorder }: AgentDockTabsProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const activeId = tabs.find((tab) => tab.active)?.id ?? tabs[0]?.id;
  useLayoutEffect(() => {
    const root = rootRef.current;
    root
      ?.querySelector<HTMLElement>('[role="tab"][data-active]')
      ?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    if (root) {
      keepActiveDockTabInsideFade(root);
      reflectDockTabOverflow(root);
    }
  }, [activeId]);
  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const reflect = () => reflectDockTabOverflow(root);
    const reconcileGeometry = () => {
      keepActiveDockTabInsideFade(root);
      reflect();
    };
    root.addEventListener("scroll", reflect, { passive: true });
    const resizeObserver = new ResizeObserver(reconcileGeometry);
    resizeObserver.observe(root);
    root.querySelectorAll<HTMLElement>('[role="tablist"] > *').forEach((tab) => {
      resizeObserver.observe(tab);
    });
    reconcileGeometry();
    return () => {
      root.removeEventListener("scroll", reflect);
      resizeObserver.disconnect();
    };
  }, [tabs.length]);
  if (tabs.length === 0) return null;
  return (
    <TabsPrimitive.Root
      ref={rootRef}
      value={activeId}
      onValueChange={(id) => tabs.find((tab) => tab.id === id)?.onSelect?.()}
      className="agent-dock-tabs"
    >
      <TabsPrimitive.List aria-label={ariaLabel} {...stylex.props(styles.contents)} activateOnFocus>
        {tabs.map((tab, index) => {
          const close = () => {
            // Where focus goes has to be decided BEFORE the tab carrying it is removed. Moving
            // to the newly active tab is the ARIA answer while one remains — but closing the
            // LAST panel leaves no tab, and `?.focus()` on nothing is silent, so focus fell to
            // `<body>` and the next Tab restarted at the top of the document. The header
            // survives an empty dock and holds the control that opens a panel again, which is
            // both adjacent in the order and the thing a person wants next.
            const header = rootRef.current?.parentElement ?? null;
            const reopen =
              header
                ?.querySelectorAll<HTMLElement>('button, [tabindex]:not([tabindex="-1"])')
                .values()
                .find((candidate) => !candidate.closest('[role="tablist"]')) ?? null;
            tab.onClose?.();
            requestAnimationFrame(() => {
              const remaining =
                rootRef.current?.querySelector<HTMLElement>('[role="tab"][data-active]') ??
                rootRef.current?.querySelector<HTMLElement>('[role="tab"]') ??
                null;
              (remaining ?? reopen)?.focus({ preventScroll: true });
            });
          };
          const row = (
            <div
              data-active={tab.active ? "" : undefined}
              data-dragging={draggingId === tab.id ? "" : undefined}
              draggable={onReorder !== undefined && tabs.length > 1}
              onDragStart={(event) => {
                event.dataTransfer.effectAllowed = "move";
                event.dataTransfer.setData("text/plain", tab.id);
                setDraggingId(tab.id);
              }}
              onDragEnd={() => setDraggingId(null)}
              onDragOver={(event) => {
                if (draggingId === null || draggingId === tab.id) return;
                event.preventDefault();
                event.dataTransfer.dropEffect = "move";
              }}
              onDrop={(event) => {
                event.preventDefault();
                const moved = event.dataTransfer.getData("text/plain") || draggingId;
                setDraggingId(null);
                if (moved && moved !== tab.id) onReorder?.(moved, index);
              }}
              onAuxClick={(event) => {
                if (event.button !== 1 || !tab.onClose) return;
                event.preventDefault();
                close();
              }}
              {...stylex.props(reveal.host, styles.tab)}
            >
              <TabsPrimitive.Tab
                value={tab.id}
                // NOT `data-chrome-focus`: the row state that would stand in for the ring is
                // `focus-within:text-fg`, which the ACTIVE tab already has — so keyboard focus
                // landing on it showed nothing at all. The ring is drawn inward because the
                // strip scrolls and clips.
                data-focus-inset=""
                // The × is a pointer affordance: a focusable sibling inside a `tablist` is
                // an unallowed child (axe `aria-required-children`, critical), and hiding it
                // from the keyboard the way `visibility: hidden` does leaves closing with no
                // key at all — seventy Tab presses never reached one. Delete/Backspace on the
                // focused tab is the ARIA practice for a closable tab and needs no extra stop
                // in the tab order.
                onKeyDown={(event) => {
                  if (!tab.onClose) return;
                  if (event.key !== "Delete" && event.key !== "Backspace") return;
                  event.preventDefault();
                  close();
                }}
                {...stylex.props(
                  styles.label,
                  type.uiSm,
                  tab.onClose ? styles.labelClosable : styles.labelPlain,
                )}
              >
                {tab.icon && <Icon name={tab.icon} size="sm" {...stylex.props(styles.glyph)} />}
                <span {...stylex.props(styles.title)}>{tab.title}</span>
                {tab.badge != null && (
                  <span {...stylex.props(styles.badge, type.ui2xs)}>{tab.badge}</span>
                )}
              </TabsPrimitive.Tab>
              {tab.onClose && (
                <IconButton
                  data-reveal="hover"
                  icon="x"
                  size="xs"
                  quiet
                  title={tab.closeLabel}
                  onClick={close}
                  className={stylex.props(styles.tabClose, reveal.pointerAffordance).className}
                />
              )}
            </div>
          );
          if (!tab.onClose && !tab.onCloseOthers && !tab.onCloseAll) {
            return <div key={tab.id}>{row}</div>;
          }
          return (
            <ContextMenu.Root key={tab.id}>
              <ContextMenu.Trigger render={row} />
              <ContextMenu.Content>
                {tab.onClose && (
                  <ContextMenu.IconItem icon="x" onSelect={close}>
                    {tab.closeLabel}
                  </ContextMenu.IconItem>
                )}
                {tab.onCloseOthers && (
                  <ContextMenu.IconItem
                    icon="minimize"
                    disabled={tabs.length < 2}
                    onSelect={tab.onCloseOthers}
                  >
                    {tab.closeOthersLabel}
                  </ContextMenu.IconItem>
                )}
                {tab.onCloseAll && (
                  <ContextMenu.IconItem icon="trash" onSelect={tab.onCloseAll}>
                    {tab.closeAllLabel}
                  </ContextMenu.IconItem>
                )}
              </ContextMenu.Content>
            </ContextMenu.Root>
          );
        })}
      </TabsPrimitive.List>
    </TabsPrimitive.Root>
  );
}
