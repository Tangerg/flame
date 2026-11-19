import { DragDropProvider, KeyboardSensor, PointerSensor } from "@dnd-kit/react";
import { Separator } from "react-resizable-panels";
import { isSortableOperation, useSortable } from "@dnd-kit/react/sortable";
import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import {
  useLayoutEffect,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { cn } from "@/lib/classNames";
import { color, motion, space, surface, type, weight } from "@flame/ui/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons/index";
import { ContextMenu } from "@/ui/atoms/menu";
import { IconButton } from "@flame/ui/icon-button";
import { handoffFocus, isImeKey, vocab, useContentVisible } from "@flame/ui";
import { reveal } from "@/ui/atoms/reveal";
import { Tabs as TabsPrimitive } from "@base-ui/react/tabs";
import { Tooltip } from "@flame/ui/tooltip";

const dockSortSensors = [
  PointerSensor.configure({ activatorElements: ({ element }) => [element] }),
  KeyboardSensor,
];

function SortableTabRow({
  tab,
  index,
  disabled,
  dragging,
  reorderLabel,
  onClose,
}: {
  tab: AgentDockTab;
  index: number;
  disabled: boolean;
  dragging: boolean;
  reorderLabel: string;
  onClose(source: HTMLElement): void;
}) {
  const { ref, handleRef, isDragging } = useSortable({ id: tab.id, index, disabled });
  const showHandle = tab.active && !disabled;
  return (
    <div
      ref={ref}
      role="presentation"
      data-active={tab.active ? "" : undefined}
      data-dock-tab-row={tab.id}
      data-dragging={isDragging ? "" : undefined}
      onMouseDownCapture={(event) => {
        if (event.button === 1) event.preventDefault();
      }}
      onAuxClick={(event) => {
        if (event.button === 1) {
          event.preventDefault();
          onClose(event.currentTarget);
        }
      }}
      {...stylex.props(reveal.host, styles.tab)}
    >
      <DockTab tab={tab} dragging={dragging} onClose={onClose} />
      <IconButton
        ref={handleRef}
        data-dock-sort-handle=""
        icon={iconGlyph("grip-horizontal")}
        size="xs"
        quiet
        aria-label={reorderLabel}
        tabIndex={showHandle ? 0 : -1}
        styles={showHandle ? styles.sortHandle : styles.hiddenSortHandle}
      />
      <IconButton
        data-reveal="hover"
        icon={iconGlyph("x")}
        size="xs"
        quiet
        aria-label={tab.closeLabel}
        tabIndex={-1}
        onClick={(event) => onClose(event.currentTarget)}
        className={stylex.props(styles.tabClose, reveal.pointerAffordance).className}
      />
    </div>
  );
}

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
  fallbackFocus?: RefObject<HTMLElement | null>;
  reorder?: {
    onMove: (id: string, toIndex: number) => string | null;
    handleLabel: (title: string) => string;
    moveLeftLabel: string;
    moveRightLabel: string;
  };
}

export function AgentDockSeparator({ label }: { label: string }) {
  return <Separator aria-label={label} className="agent-pane-resizer" />;
}

const styles = stylex.create({
  row: { display: "flex", minHeight: 0, flex: 1 },
  tab: {
    display: "flex",
    height: "var(--dock-tab-height)",
    minWidth: 0,
    // The full row must fit between the strip's two overflow fades.
    maxInlineSize: "calc(100cqi - 2 * var(--dock-tab-edge))",
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
  sortHandle: { flexShrink: 0, cursor: "grab" },
  hiddenSortHandle: { display: "none" },
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
  onClose(source: HTMLElement): void;
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
              onClose(event.currentTarget);
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

export function AgentDockRow({
  open,
  available,
  presentation,
  ref,
  children,
}: {
  open: boolean;
  available: boolean;
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
    >
      {children}
    </div>
  );
}

function reflectDockTabOverflow(element: HTMLElement): void {
  const maxScrollLeft = Math.max(0, element.scrollWidth - element.clientWidth);
  element.toggleAttribute("data-overflow-start", element.scrollLeft > 1);
  element.toggleAttribute("data-overflow-end", maxScrollLeft - element.scrollLeft > 1);
}

function keepActiveDockTabInsideFade(element: HTMLElement): void {
  if (element.clientWidth === 0) return;
  const activeRow = element.querySelector<HTMLElement>("[data-dock-tab-row][data-active]");
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

function dockMenuFocusOwner(menu: HTMLElement): HTMLElement | null {
  const document = menu.ownerDocument;
  const focused = document.activeElement;
  if (focused === document.body) return document.body;
  if (focused === document.documentElement) return document.documentElement;
  return menu.contains(focused) ? menu : null;
}

export function AgentDockTabs({ tabs, ariaLabel, fallbackFocus, reorder }: AgentDockTabsProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const cancelMenuReturn = useRef<(() => void) | null>(null);
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const visible = useContentVisible();
  const active = useRef(false);
  const tabStructure = JSON.stringify(
    tabs.map(({ id, title, active: selected }) => [id, title, selected]),
  );
  useLayoutEffect(() => {
    active.current = visible;
    return () => {
      active.current = false;
      cancelMenuReturn.current?.();
      cancelMenuReturn.current = null;
    };
  }, [visible]);
  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const reflect = () => {
      keepActiveDockTabInsideFade(root);
      reflectDockTabOverflow(root);
    };
    const observer = new ResizeObserver(reflect);
    observer.observe(root);
    const scrolled = () => reflectDockTabOverflow(root);
    root.addEventListener("scroll", scrolled, { passive: true });
    reflect();
    return () => {
      observer.disconnect();
      root.removeEventListener("scroll", scrolled);
    };
  }, [tabStructure]);
  const move = (id: string, index: number) => {
    if (!active.current || !reorder || !tabs.some((tab) => tab.id === id)) return;
    const message = reorder.onMove(id, index);
    if (message) setAnnouncement(message);
  };
  const close = (tab: AgentDockTab, source: HTMLElement) => {
    const index = tabs.findIndex((item) => item.id === tab.id);
    const nextId = (tabs[index + 1] ?? tabs[index - 1])?.id;
    const target =
      [...(rootRef.current?.querySelectorAll<HTMLElement>("[role=tab]") ?? [])].find(
        (element) => element.dataset.dockTabId === nextId,
      ) ??
      fallbackFocus?.current ??
      rootRef.current;
    handoffFocus(source, target);
    tab.onClose();
  };
  const closeFromMenu = (source: HTMLElement, action: () => void) => {
    const menu = source.closest<HTMLElement>("[data-dock-tab-menu]");
    const root = rootRef.current;
    const view = menu?.ownerDocument.defaultView;
    cancelMenuReturn.current?.();
    cancelMenuReturn.current = null;
    if (active.current && root && menu && view && dockMenuFocusOwner(menu)) {
      // The modal hides surviving controls until Base UI releases its accessibility boundary.
      const frame = view.requestAnimationFrame(() => {
        cancelMenuReturn.current = null;
        if (rootRef.current !== root || (menu.isConnected && menu.hasAttribute("data-open")))
          return;
        const owner = dockMenuFocusOwner(menu);
        if (owner)
          handoffFocus(
            owner,
            root.querySelector<HTMLElement>('[data-dock-tab-row][data-active] [role="tab"]') ??
              fallbackFocus?.current ??
              root,
          );
      });
      cancelMenuReturn.current = () => view.cancelAnimationFrame(frame);
    }
    action();
  };
  return (
    <div ref={rootRef} tabIndex={-1} className="agent-dock-tabs">
      <span role="status" aria-live="polite" className="sr-only">
        {announcement}
      </span>
      <DragDropProvider
        key={visible ? "active" : "hidden"}
        sensors={dockSortSensors}
        onDragStart={({ operation }) =>
          setDraggingId(operation.source ? String(operation.source.id) : null)
        }
        onDragEnd={(event) => {
          setDraggingId(null);
          if (event.canceled || !active.current || !isSortableOperation(event.operation)) return;
          const source = event.operation.source;
          if (source && event.operation.target && tabs.some((tab) => tab.id === source.id))
            move(String(source.id), source.index);
        }}
      >
        <TabsPrimitive.List
          aria-label={ariaLabel}
          {...stylex.props(styles.contents)}
          activateOnFocus
          onKeyDown={(event) => {
            if (
              event.target instanceof Element &&
              event.target.hasAttribute("data-dock-sort-handle")
            )
              event.preventBaseUIHandler();
          }}
        >
          {tabs.map((tab, index) => (
            <ContextMenu.Root key={tab.id}>
              <SortableTabRow
                tab={tab}
                index={index}
                disabled={!visible || !reorder || tabs.length < 2}
                dragging={draggingId !== null}
                reorderLabel={reorder?.handleLabel(tab.title) ?? tab.title}
                onClose={(source) => close(tab, source)}
              />
              <ContextMenu.Content aria-label={tab.title} data-dock-tab-menu={tab.id}>
                {reorder && (
                  <>
                    <ContextMenu.IconItem
                      icon="arrow-left"
                      disabled={index === 0}
                      onSelect={() => move(tab.id, index - 1)}
                    >
                      {reorder.moveLeftLabel}
                    </ContextMenu.IconItem>
                    <ContextMenu.IconItem
                      icon="arrow-right"
                      disabled={index === tabs.length - 1}
                      onSelect={() => move(tab.id, index + 1)}
                    >
                      {reorder.moveRightLabel}
                    </ContextMenu.IconItem>
                    <ContextMenu.Separator />
                  </>
                )}
                <ContextMenu.IconItem
                  icon="x"
                  onSelect={(event) => closeFromMenu(event.currentTarget, tab.onClose)}
                >
                  {tab.closeLabel}
                </ContextMenu.IconItem>
                {tab.onCloseOthers && (
                  <ContextMenu.IconItem
                    icon="close-others"
                    disabled={tabs.length < 2}
                    onSelect={(event) => closeFromMenu(event.currentTarget, tab.onCloseOthers!)}
                  >
                    {tab.closeOthersLabel}
                  </ContextMenu.IconItem>
                )}
                {tab.onCloseAll && (
                  <ContextMenu.IconItem
                    icon="close-all"
                    onSelect={(event) => closeFromMenu(event.currentTarget, tab.onCloseAll!)}
                  >
                    {tab.closeAllLabel}
                  </ContextMenu.IconItem>
                )}
              </ContextMenu.Content>
            </ContextMenu.Root>
          ))}
        </TabsPrimitive.List>
      </DragDropProvider>
    </div>
  );
}
