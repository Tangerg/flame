import { useLayoutEffect, useRef, type ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
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
}

// Carries no state: whether the flank is showing is a fact about the row it shares with
// the conversation, so the row declares it once and this stays pure structure.
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

// Goes through the tab primitive for roving focus and arrow-key navigation: buttons styled
// to resemble tabs without those semantics make the dock keyboard-hostile.
export function AgentDockTabs({ tabs, ariaLabel }: { tabs: AgentDockTab[]; ariaLabel: string }) {
  const rootRef = useRef<HTMLDivElement>(null);
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
        {tabs.map((tab) => {
          return (
            <div
              key={tab.id}
              data-active={tab.active ? "" : undefined}
              className={cn(
                "group flex h-[var(--dock-tab-height)] min-w-0 shrink-0 items-center rounded-[var(--dock-tab-radius)]",
                "text-fg-muted transition-[background-color,color] duration-[var(--dur-color)] ease-out",
                "hover:bg-hover hover:text-fg focus-within:text-fg",
                // Fills with the PANEL's ground, not a selection wash: a tab is the top of
                // the thing it opens, and a wash makes the strip read as a row of chips.
                "data-[active]:bg-[var(--dock-tab-active-surface)] data-[active]:text-fg",
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
                  onClick={() => {
                    tab.onClose?.();
                    requestAnimationFrame(() => {
                      rootRef.current
                        ?.querySelector<HTMLElement>('[role="tab"][data-active]')
                        ?.focus({ preventScroll: true });
                    });
                  }}
                  className="mr-0.5 invisible opacity-0 transition-opacity duration-[var(--dur-fast)] group-hover:visible group-hover:opacity-100 group-focus-within:visible group-focus-within:opacity-100"
                />
              )}
            </div>
          );
        })}
      </TabsPrimitive.List>
    </TabsPrimitive.Root>
  );
}
