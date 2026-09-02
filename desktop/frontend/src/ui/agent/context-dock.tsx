import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
import { ContextMenu } from "@/ui/atoms/menu";
import { IconButton } from "@/ui/atoms/icon-button";
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

export function AgentContextDock({ children }: { children: ReactNode }) {
  return <aside className="agent-context-dock agent-pane-split">{children}</aside>;
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
      <TabsPrimitive.List aria-label={ariaLabel} className="contents" activateOnFocus>
        {tabs.map((tab, index) => {
          const restoreFocus = () => {
            requestAnimationFrame(() => {
              rootRef.current
                ?.querySelector<HTMLElement>('[role="tab"][data-active]')
                ?.focus({ preventScroll: true });
            });
          };
          const close = () => {
            tab.onClose?.();
            restoreFocus();
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
              className={cn(
                "group flex h-[var(--dock-tab-height)] min-w-0 shrink-0 items-center rounded-[var(--dock-tab-radius)]",
                "text-fg-muted transition-[background-color,color,opacity] duration-[var(--dur-color)] ease-out",
                "hover:bg-hover hover:text-fg focus-within:text-fg",
                "data-[active]:bg-[var(--dock-tab-active-surface)] data-[active]:text-fg",
                "data-[dragging]:opacity-50",
              )}
            >
              <TabsPrimitive.Tab
                value={tab.id}
                data-chrome-focus=""
                className={cn(
                  "inline-flex h-full min-w-0 max-w-40 items-center gap-1.5 rounded-[inherit] border-0 bg-transparent py-0 text-ui-sm font-normal text-inherit focus-visible:outline-none",
                  tab.onClose ? "pl-2 pr-1" : "px-2",
                )}
              >
                {tab.icon && <Icon name={tab.icon} size="sm" className="shrink-0 opacity-70" />}
                <span className="truncate">{tab.title}</span>
                {tab.badge != null && (
                  <span className="shrink-0 font-mono text-ui-2xs leading-none text-fg-faint tabular-nums">
                    {tab.badge}
                  </span>
                )}
              </TabsPrimitive.Tab>
              {tab.onClose && (
                <IconButton
                  icon="x"
                  size="xs"
                  quiet
                  title={tab.closeLabel}
                  onClick={close}
                  className="mr-0.5 invisible opacity-0 transition-opacity duration-[var(--dur-fast)] group-hover:visible group-hover:opacity-100 focus-visible:visible focus-visible:opacity-100"
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
              <ContextMenu.Content className="min-w-[168px]">
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
