import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ComponentProps,
  type ReactNode,
  type Ref,
} from "react";
import { cn } from "@/lib/classNames";
import { color, motion, space, surface, type, weight } from "@flame/ui/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons/index";
import { ContextMenu } from "@/ui/atoms/menu";
import { IconButton } from "@flame/ui/icon-button";
import { isImeKey, vocab } from "@flame/ui";
import { reveal } from "@/ui/atoms/reveal";
import { ResizeHandle, type ResizeHandleProps } from "@/ui/atoms/resize-handle";
import { TabsPrimitive } from "@/ui/primitives/index";
export interface AgentDockTab {
  id: string;
  title: ReactNode;
  icon?: IconName;
  badge?: ReactNode;
  active?: boolean;
  onSelect?: () => void;
  onClose: () => void;
  closeLabel: string;
  onCloseOthers?: () => void;
  closeOthersLabel?: string;
  onCloseAll?: () => void;
  closeAllLabel?: string;
}

export interface AgentDockTabsProps {
  tabs: AgentDockTab[];
  ariaLabel: string;
  reorder?: {
    onMove: (id: string, toIndex: number) => void;
    moveLeftLabel: string;
    moveRightLabel: string;
  };
}

const styles = stylex.create({
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
    opacity: { default: null, ":is([data-dragging])": 0.5 },
    transitionProperty: "background-color, color, opacity",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
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
    fontWeight: weight.medium,
    color: "inherit",
  },
  labelClosable: { paddingLeft: space.s2, paddingRight: space.s1 },
  glyph: { flexShrink: 0, opacity: "var(--glyph-step)" },
  tabClose: { marginRight: space.s0_5 },
  badge: { flexShrink: 0, fontFamily: "var(--font-mono)", lineHeight: 1, color: color.fgFaint },
  contents: { display: "contents" },
});

export function AgentContextDock({ children, className, ...props }: ComponentProps<"aside">) {
  return (
    <aside {...props} className={cn("agent-context-dock", className)}>
      {children}
    </aside>
  );
}

export const DOCK_MEASURE_PROPERTY = "--dock-measure";

export function AgentDockRow({
  open,
  width,
  presentation,
  ref,
  children,
}: {
  open: boolean;
  width: number;
  presentation: "dock" | "main";
  ref?: Ref<HTMLDivElement>;
  children: ReactNode;
}) {
  return (
    <div
      ref={ref}
      className={cn("agent-dock-row", stylex.props(styles.row).className)}
      data-dock={open ? "open" : "collapsed"}
      data-presentation={presentation}
      style={{ [DOCK_MEASURE_PROPERTY]: `${width}px` } as CSSProperties}
    >
      {children}
    </div>
  );
}

export function AgentDockResizer(props: Omit<ResizeHandleProps, "edge" | "className">) {
  return <ResizeHandle {...props} edge="start" className="agent-pane-resizer" />;
}

function reflectDockTabOverflow(element: HTMLElement): void {
  const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
  element.toggleAttribute("data-overflow-start", element.scrollLeft > 1);
  element.toggleAttribute("data-overflow-end", maxScrollLeft - element.scrollLeft > 1);
}

function keepActiveDockTabInsideFade(element: HTMLElement): void {
  if (element.clientWidth === 0) return;
  const activeRow = element.querySelector<HTMLElement>('[role="tab"][data-active]')?.parentElement;
  if (!activeRow) return;
  const stripBox = element.getBoundingClientRect();
  const activeBox = activeRow.getBoundingClientRect();
  const edgeHint = Number.parseFloat(getComputedStyle(element).scrollPaddingInlineStart);
  if (activeBox.left < stripBox.left + edgeHint) {
    element.scrollLeft -= stripBox.left + edgeHint - activeBox.left;
  } else if (activeBox.right > stripBox.right - edgeHint) {
    element.scrollLeft += activeBox.right - (stripBox.right - edgeHint);
  }
}

export function AgentDockTabs({ tabs, ariaLabel, reorder }: AgentDockTabsProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const activeId = tabs.find((tab) => tab.active)?.id ?? tabs[0]?.id;
  const actAndSettleFocus = (
    id: string,
    action: () => void,
    target: () => HTMLElement | null | undefined,
  ) => {
    const doc = rootRef.current?.ownerDocument;
    const origin = doc?.activeElement;
    const menu = Array.from(doc?.querySelectorAll<HTMLElement>("[data-dock-tab-menu]") ?? []).find(
      (element) => element.dataset.dockTabMenu === id,
    );
    const ownsFocus =
      origin != null &&
      (origin === doc?.body || rootRef.current?.contains(origin) || menu?.contains(origin));
    action();
    if (!doc || !ownsFocus) return;
    requestAnimationFrame(() => {
      const focused = doc.activeElement;
      if (focused !== origin && focused !== doc.body && !menu?.contains(focused)) return;
      const next = target();
      if (next?.isConnected) next.focus({ preventScroll: true });
    });
  };
  const closeAndFocus = (id: string, action: () => void) => {
    const header = rootRef.current?.parentElement;
    const reopen = Array.from(
      header?.querySelectorAll<HTMLElement>(
        'button:not(:disabled), [tabindex]:not([tabindex="-1"])',
      ) ?? [],
    ).find((candidate) => !candidate.closest('[role="tablist"]'));
    actAndSettleFocus(
      id,
      action,
      () =>
        rootRef.current?.querySelector<HTMLElement>('[role="tab"][data-active]') ??
        rootRef.current?.querySelector<HTMLElement>('[role="tab"]') ??
        reopen,
    );
  };
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
      <TabsPrimitive.List
        aria-label={ariaLabel}
        {...stylex.props(styles.contents)}
        activateOnFocus
        onMouseDown={(event) => {
          if (event.button === 1) event.preventDefault();
        }}
      >
        {tabs.map((tab, index) => {
          const close = () => {
            closeAndFocus(tab.id, tab.onClose);
          };
          const move = (toIndex: number) => {
            const trigger = Array.from(
              rootRef.current?.querySelectorAll<HTMLElement>('[role="tab"]') ?? [],
            ).find((element) => element.dataset.dockTabId === tab.id);
            actAndSettleFocus(
              tab.id,
              () => reorder?.onMove(tab.id, toIndex),
              () => trigger,
            );
          };
          const row = (
            <div
              data-active={tab.active ? "" : undefined}
              data-dragging={draggingId === tab.id ? "" : undefined}
              draggable={reorder !== undefined && tabs.length > 1}
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
                if (moved && moved !== tab.id) reorder?.onMove(moved, index);
              }}
              onAuxClick={(event) => {
                if (event.button !== 1) return;
                event.preventDefault();
                close();
              }}
              {...stylex.props(reveal.host, styles.tab)}
            >
              <TabsPrimitive.Tab
                value={tab.id}
                data-dock-tab-id={tab.id}
                data-focus-inset=""
                onKeyDown={(event) => {
                  if (event.key !== "Delete" && event.key !== "Backspace") return;
                  if (event.defaultPrevented || isImeKey(event.nativeEvent)) return;
                  event.preventDefault();
                  close();
                }}
                {...stylex.props(styles.label, type.uiMd, styles.labelClosable)}
              >
                {tab.icon && <Icon name={tab.icon} size="sm" {...stylex.props(styles.glyph)} />}
                <span {...stylex.props(vocab.truncate)}>{tab.title}</span>
                {tab.badge != null && (
                  <span {...stylex.props(styles.badge, type.ui2xs)}>{tab.badge}</span>
                )}
              </TabsPrimitive.Tab>
              <IconButton
                data-reveal="hover"
                icon={iconGlyph("x")}
                size="xs"
                quiet
                aria-label={tab.closeLabel}
                onClick={close}
                className={stylex.props(styles.tabClose, reveal.pointerAffordance).className}
              />
            </div>
          );
          return (
            <ContextMenu.Root key={tab.id}>
              <ContextMenu.Trigger render={row} />
              <ContextMenu.Content data-dock-tab-menu={tab.id}>
                {reorder && (
                  <>
                    <ContextMenu.IconItem
                      icon="arrow-left"
                      disabled={index === 0}
                      onSelect={() => move(index - 1)}
                    >
                      {reorder.moveLeftLabel}
                    </ContextMenu.IconItem>
                    <ContextMenu.IconItem
                      icon="arrow-right"
                      disabled={index === tabs.length - 1}
                      onSelect={() => move(index + 1)}
                    >
                      {reorder.moveRightLabel}
                    </ContextMenu.IconItem>
                    <ContextMenu.Separator />
                  </>
                )}
                <ContextMenu.IconItem icon="x" onSelect={close}>
                  {tab.closeLabel}
                </ContextMenu.IconItem>
                {tab.onCloseOthers && (
                  <ContextMenu.IconItem
                    icon="close-others"
                    disabled={tabs.length < 2}
                    onSelect={() => closeAndFocus(tab.id, tab.onCloseOthers!)}
                  >
                    {tab.closeOthersLabel}
                  </ContextMenu.IconItem>
                )}
                {tab.onCloseAll && (
                  <ContextMenu.IconItem
                    icon="close-all"
                    onSelect={() => closeAndFocus(tab.id, tab.onCloseAll!)}
                  >
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
