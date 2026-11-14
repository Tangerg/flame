import {
  draggable,
  dropTargetForElements,
} from "@atlaskit/pragmatic-drag-and-drop/adapter/element-adapter";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  useCallback,
  useMemo,
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
import { isImeKey, vocab, RemovalFocus, useContentVisible, FocusWitness } from "@flame/ui";
import { reveal } from "@/ui/atoms/reveal";
import { ResizeHandle, type ResizeHandleProps } from "@/ui/atoms/resize-handle";
import { Tabs as TabsPrimitive } from "@base-ui/react/tabs";
import { Tooltip } from "@flame/ui/tooltip";
type DockTabDrag = {
  kind: "flame-dock-tab";
  strip: HTMLElement;
  registration: object;
  id: string;
};

export interface AgentDockTab {
  id: string;
  title: string;
  icon?: IconName;
  badge?: ReactNode;
  active?: boolean;
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
    onMove: (id: string, toIndex: number) => string | null;
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
  labelClosable: { paddingLeft: "var(--dock-tab-inset)", paddingRight: space.s1 },
  glyph: { flexShrink: 0, opacity: "var(--glyph-step)" },
  tabClose: { marginRight: space.s0_5 },
  badge: { flexShrink: 0, fontFamily: "var(--font-mono)", lineHeight: 1, color: color.fgFaint },
  contents: { display: "contents" },
});

export function AgentContextDock({
  children,
  className,
  value,
  onValueChange,
  ...props
}: ComponentProps<"aside"> & {
  value: string | null;
  onValueChange: (value: string | null) => void;
}) {
  return (
    <TabsPrimitive.Root
      value={value}
      onValueChange={onValueChange}
      render={<aside {...props} className={cn("agent-context-dock", className)} />}
    >
      {children}
    </TabsPrimitive.Root>
  );
}

export function AgentDockPanel({
  value,
  presentation,
  ...props
}: Omit<ComponentProps<"div">, "role" | "hidden" | "inert"> & {
  value: string;
  presentation: "dock" | "main";
}) {
  return (
    <TabsPrimitive.Panel
      {...props}
      value={value}
      keepMounted
      // The host retains inactive layout so WebKit preserves native scroll positions.
      hidden={false}
      role={presentation === "main" ? "region" : "tabpanel"}
    />
  );
}

function DockTab({
  tab,
  onClose,
  dragging,
}: {
  tab: AgentDockTab;
  onClose: () => void;
  dragging: boolean;
}) {
  const label = useRef<HTMLSpanElement>(null);
  const [truncated, setTruncated] = useState(false);
  useLayoutEffect(() => {
    const node = label.current;
    if (!node) return;
    const measure = () => setTruncated(node.scrollWidth > node.clientWidth);
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    node.ownerDocument.fonts?.addEventListener("loadingdone", measure);
    measure();
    return () => {
      observer.disconnect();
      node.ownerDocument.fonts?.removeEventListener("loadingdone", measure);
    };
  }, [tab.title]);
  return (
    <Tooltip label={tab.title} disabled={!truncated || dragging}>
      <ContextMenu.Trigger
        render={
          <TabsPrimitive.Tab
            value={tab.id}
            data-dock-tab-id={tab.id}
            data-focus-inset=""
            onKeyDown={(event) => {
              if (event.key !== "Delete" && event.key !== "Backspace") return;
              if (event.defaultPrevented || isImeKey(event.nativeEvent)) return;
              event.preventDefault();
              onClose();
            }}
            {...stylex.props(styles.label, type.uiMd, styles.labelClosable)}
          >
            {tab.icon && <Icon name={tab.icon} size="sm" {...stylex.props(styles.glyph)} />}
            <span ref={label} {...stylex.props(vocab.truncate)}>
              {tab.title}
            </span>
            {tab.badge != null && (
              <span {...stylex.props(styles.badge, type.ui2xs)}>{tab.badge}</span>
            )}
          </TabsPrimitive.Tab>
        }
      />
    </Tooltip>
  );
}

export const DOCK_MEASURE_PROPERTY = "--dock-measure";

export function AgentDockRow({
  open,
  available,
  width,
  presentation,
  ref,
  children,
}: {
  open: boolean;
  available: boolean;
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
      data-dock-available={available ? "" : undefined}
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
  const [dragPresentation, setDragPresentation] = useState<{
    id: string;
    registration: object;
  } | null>(null);
  const visible = useContentVisible();
  const binding = useRef({ tabs, reorder, visible });
  const removal = useRef<{
    witness: RemovalFocus;
    target: () => HTMLElement | null | undefined;
  } | null>(null);
  const menuReturn = useRef<{
    id: string;
    target: () => HTMLElement | null | undefined;
  } | null>(null);
  const menuFocus = useRef<{ id: string; witness: FocusWitness } | null>(null);
  const openingFocus = useRef<{ id: string; target: HTMLElement | null } | null>(null);
  const lifetime = useRef({ active: true });
  const [announcement, setAnnouncement] = useState("");
  useLayoutEffect(() => {
    binding.current = { tabs, reorder, visible };
  });
  const requestMove = useCallback((id: string, toIndex: number) => {
    const current = binding.current;
    const from = current.tabs.findIndex((tab) => tab.id === id);
    if (!current.reorder || from < 0 || from === toIndex) return;
    const message = current.reorder.onMove(id, toIndex);
    if (message) setAnnouncement(message);
  }, []);
  const tabIds = JSON.stringify(tabs.map((tab) => tab.id));
  const canReorder = visible && reorder !== undefined && tabs.length > 1;
  const registration = useMemo(() => ({ tabIds, canReorder }), [tabIds, canReorder]);
  const draggingId = dragPresentation?.registration === registration ? dragPresentation.id : null;
  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root || !canReorder) return;
    let active = true;
    const draggedId = (data: Record<string | symbol, unknown>) => {
      if (
        !active ||
        !binding.current.visible ||
        data.kind !== "flame-dock-tab" ||
        data.strip !== root ||
        data.registration !== registration ||
        typeof data.id !== "string" ||
        !binding.current.tabs.some((tab) => tab.id === data.id)
      )
        return null;
      return data.id;
    };
    const releases = Array.from(root.querySelectorAll<HTMLElement>("[data-dock-tab-row]")).flatMap(
      (element) => {
        const id = element.dataset.dockTabRow!;
        return [
          draggable({
            element,
            canDrag: () =>
              active && binding.current.visible && binding.current.reorder !== undefined,
            getInitialData: () =>
              ({ kind: "flame-dock-tab", strip: root, registration, id }) satisfies DockTabDrag,
            onDragStart: () => {
              if (active) setDragPresentation({ id, registration });
            },
            onDrop: () => {
              if (active) setDragPresentation(null);
            },
          }),
          dropTargetForElements({
            element,
            canDrop: ({ source }) => {
              const moved = draggedId(source.data);
              return moved !== null && moved !== id && binding.current.reorder !== undefined;
            },
            onDrop: ({ source, location }) => {
              if (location.current.dropTargets[0]?.element !== element) return;
              const moved = draggedId(source.data);
              const index = binding.current.tabs.findIndex((tab) => tab.id === id);
              if (moved !== null && moved !== id && index >= 0) requestMove(moved, index);
            },
          }),
        ];
      },
    );
    return () => {
      active = false;
      releases.forEach((release) => release());
    };
  }, [tabIds, canReorder, requestMove, registration]);
  const activeIndex = tabs.findIndex((tab) => tab.active);
  const activeId = tabs[activeIndex]?.id;
  const closeTarget = () => {
    const header = rootRef.current?.parentElement;
    const reopen = Array.from(
      header?.querySelectorAll<HTMLElement>(
        'button:not(:disabled), [tabindex]:not([tabindex="-1"])',
      ) ?? [],
    ).find((node) => !node.closest('[role="tablist"]'));
    return () =>
      rootRef.current?.querySelector<HTMLElement>('[role="tab"][data-active]') ??
      rootRef.current?.querySelector<HTMLElement>('[role="tab"]') ??
      reopen;
  };
  const closeAndFocus = (id: string, action: () => void) => {
    const root = rootRef.current;
    const row = Array.from(root?.querySelectorAll<HTMLElement>("[data-dock-tab-row]") ?? []).find(
      (node) => node.dataset.dockTabRow === id,
    );
    const witness =
      row &&
      RemovalFocus.capture(
        row,
        () => binding.current.visible && (rootRef.current === root || rootRef.current === null),
      );
    if (witness) {
      removal.current?.witness.retire();
      removal.current = { witness, target: closeTarget() };
    }
    action();
  };
  const fromMenu = (id: string, action: () => void, target = closeTarget()) => {
    if (!lifetime.current.active || !binding.current.visible) return;
    menuReturn.current = { id, target };
    action();
  };
  useLayoutEffect(() => {
    const captured = removal.current;
    removal.current = null;
    captured?.witness.restore(captured.target);
  });
  useLayoutEffect(() => {
    const current = lifetime.current;
    current.active = true;
    return () => {
      current.active = false;
      removal.current?.witness.retire();
      removal.current = null;
      menuFocus.current?.witness.retire();
      menuFocus.current = null;
      menuReturn.current = null;
      openingFocus.current = null;
    };
  }, [visible]);
  useLayoutEffect(() => {
    const root = rootRef.current;
    root
      ?.querySelector<HTMLElement>('[role="tab"][data-active]')
      ?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    if (root) {
      keepActiveDockTabInsideFade(root);
      reflectDockTabOverflow(root);
    }
  }, [activeId, activeIndex]);
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
    <div ref={rootRef} className="agent-dock-tabs">
      <span role="status" aria-live="polite" className="sr-only">
        {announcement}
      </span>
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
            const fallback = closeTarget();
            const trigger = Array.from(
              rootRef.current?.querySelectorAll<HTMLElement>('[role="tab"]') ?? [],
            ).find((node) => node.dataset.dockTabId === tab.id);
            fromMenu(
              tab.id,
              () => requestMove(tab.id, toIndex),
              () => (trigger?.isConnected ? trigger : fallback()),
            );
          };
          const row = (
            <div
              data-active={tab.active ? "" : undefined}
              data-dragging={draggingId === tab.id ? "" : undefined}
              data-dock-tab-row={tab.id}
              onAuxClick={(event) => {
                if (event.button !== 1) return;
                event.preventDefault();
                close();
              }}
              {...stylex.props(reveal.host, styles.tab)}
            >
              <DockTab tab={tab} onClose={close} dragging={draggingId !== null} />
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
            <ContextMenu.Root
              key={tab.id}
              onOpenChange={(open) => {
                if (!open) return;
                menuReturn.current = null;
                menuFocus.current?.witness.retire();
                menuFocus.current = null;
                const document = rootRef.current?.ownerDocument;
                openingFocus.current = {
                  id: tab.id,
                  target:
                    document?.activeElement !== document?.body
                      ? (document?.activeElement as HTMLElement | null)
                      : null,
                };
              }}
            >
              {row}
              <ContextMenu.Content
                data-dock-tab-menu={tab.id}
                aria-label={tab.title}
                onFocus={(event) => {
                  menuFocus.current?.witness.retire();
                  const witness = FocusWitness.capture(
                    event.currentTarget,
                    () => lifetime.current.active && binding.current.visible,
                  );
                  menuFocus.current = witness ? { id: tab.id, witness } : null;
                }}
                finalFocus={() => {
                  const focus = menuFocus.current;
                  if (focus?.id !== tab.id) return false;
                  menuFocus.current = null;
                  const intent = menuReturn.current;
                  if (intent?.id === tab.id) menuReturn.current = null;
                  // Settle in the public closing callback so later focus cannot lose to a queued return.
                  focus.witness.restore(
                    intent?.id === tab.id
                      ? intent.target
                      : () =>
                          openingFocus.current?.id === tab.id ? openingFocus.current.target : null,
                  );
                  return false;
                }}
              >
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
                <ContextMenu.IconItem icon="x" onSelect={() => fromMenu(tab.id, tab.onClose)}>
                  {tab.closeLabel}
                </ContextMenu.IconItem>
                {tab.onCloseOthers && (
                  <ContextMenu.IconItem
                    icon="close-others"
                    disabled={tabs.length < 2}
                    onSelect={() => fromMenu(tab.id, tab.onCloseOthers!)}
                  >
                    {tab.closeOthersLabel}
                  </ContextMenu.IconItem>
                )}
                {tab.onCloseAll && (
                  <ContextMenu.IconItem
                    icon="close-all"
                    onSelect={() => fromMenu(tab.id, tab.onCloseAll!)}
                  >
                    {tab.closeAllLabel}
                  </ContextMenu.IconItem>
                )}
              </ContextMenu.Content>
            </ContextMenu.Root>
          );
        })}
      </TabsPrimitive.List>
    </div>
  );
}
