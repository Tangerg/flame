import * as stylex from "@stylexjs/stylex";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { cn } from "@/lib/classNames";
import {
  color,
  leading,
  motion,
  radius,
  space,
  surface,
  type,
  weight,
} from "@flame/ui/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons/index";
import { Tabs as TabsPrimitive } from "@base-ui/react/tabs";
import { ContentVisibility, ItemHighlight, HoverTrack, useHoverTrackItem, vocab } from "@flame/ui";
import { SectionLabel } from "@/ui/atoms/section-label";
interface VerticalTabItem {
  id: string;
  label: string;
  icon?: IconName;
  content: ReactNode;
}

interface VerticalTabGroup {
  id: string;
  label: ReactNode;
  items: VerticalTabItem[];
}

const styles = stylex.create({
  container: { width: "100%", height: "100%", minWidth: 0, containerType: "inline-size" },
  frame: {
    display: "grid",
    height: "100%",
    width: "100%",
    gridTemplateColumns: {
      default: "256px minmax(0, 1fr)",
      "@container (max-width: 40rem)": "minmax(0, 1fr)",
    },
    gridTemplateRows: {
      default: "minmax(0, 1fr)",
      "@container (max-width: 40rem)": `fit-content(max(40%, calc(var(--vertical-tabs-header-height, 0px) + var(--density-row-height) + ${space.s6}))) minmax(0, 1fr)`,
    },
    overflow: "hidden",
    backgroundColor: surface.canvas,
  },
  rail: {
    display: "flex",
    minHeight: 0,
    minWidth: 0,
    flexDirection: "column",
    backgroundColor: surface.surface,
    backgroundImage: { default: null, ":is([data-paper-texture] *)": surface.texture },
  },
  list: {
    display: "flex",
    minHeight: 0,
    flex: 1,
    flexDirection: "column",
    gap: "1px",
    overflowY: "auto",
    overscrollBehavior: "contain",
    paddingInline: "var(--density-navigation-gutter)",
    paddingBottom: space.s6,
  },
  group: { display: "flex", flexDirection: "column", gap: "1px" },
  tab: {
    display: "grid",
    gridTemplateColumns: "var(--icon-sm) minmax(0, 1fr)",
    height: "var(--density-row-height)",
    alignItems: "center",
    columnGap: "var(--density-row-gap)",
    borderRadius: radius.row,
    borderWidth: 0,
    position: "relative",
    isolation: "isolate",
    backgroundColor: {
      default: "transparent",
      ":hover": "transparent",
      ":is([data-active])": surface.selected,
      ":is([data-active]):hover": surface.selected,
    },
    paddingInline: space.s2,
    textAlign: "start",
    fontFamily: "var(--font-sans)",
    fontWeight: weight.regular,
    lineHeight: leading.tight,
    color: color.fg,
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  highlight: { borderRadius: radius.row },
  glyph: { gridColumn: 1 },
  label: { gridColumn: 2 },
  page: {
    gridColumn: { default: 2, "@container (max-width: 40rem)": 1 },
    gridRow: { default: 1, "@container (max-width: 40rem)": 2 },
    minHeight: 0,
    minWidth: 0,
    // Clip scrolling content before the reserved inset focus perimeter.
    borderWidth: "2px",
    borderStyle: "solid",
    borderColor: "transparent",
    overflowY: "auto",
    overscrollBehavior: "contain",
    scrollPaddingBlock: space.s4,
    backgroundColor: surface.canvas,
    backgroundImage: { default: null, ":is([data-paper-texture] *)": surface.texture },
  },
  inactive: { visibility: "hidden" },
  heading: { paddingInline: space.s2, paddingBottom: space.s1, paddingTop: space.s4 },
});

function RailTab({ item }: { item: VerticalTabItem }) {
  const track = useHoverTrackItem();
  return (
    <TabsPrimitive.Tab
      value={item.id}
      onPointerEnter={track?.onPointerEnter}
      {...stylex.props(styles.tab, type.uiMd)}
    >
      {track?.hovered && <ItemHighlight layoutId={track.layoutId} styles={styles.highlight} />}
      {item.icon && <Icon name={item.icon} size="sm" {...stylex.props(styles.glyph)} />}
      <span {...stylex.props(vocab.truncate, styles.label)}>{item.label}</span>
    </TabsPrimitive.Tab>
  );
}

function RetainedPanel({ item, active }: { item: VerticalTabItem; active: boolean }) {
  const [visited, setVisited] = useState(active);
  if (active && !visited) setVisited(true);
  return (
    <TabsPrimitive.Panel
      value={item.id}
      aria-label={item.label}
      keepMounted={visited}
      hidden={false}
      aria-hidden={!active || undefined}
      {...stylex.props(styles.page, !active && styles.inactive)}
    >
      <ContentVisibility visible={active}>{item.content}</ContentVisibility>
    </TabsPrimitive.Panel>
  );
}

export function VerticalTabs({
  ariaLabel,
  groups,
  value,
  onValueChange,
  railHeader,
  railFilter,
  railEmpty,
}: {
  ariaLabel: string;
  groups: VerticalTabGroup[];
  value?: string;
  onValueChange: (value: string | undefined) => void;
  railHeader?: ReactNode;
  railFilter?: (item: VerticalTabItem) => boolean;
  railEmpty?: ReactNode;
}) {
  const items = groups.flatMap((group) => group.items);
  const railGroups = railFilter
    ? groups
        .map((group) => ({ ...group, items: group.items.filter(railFilter) }))
        .filter((group) => group.items.length > 0)
    : groups;
  const frameRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const frame = frameRef.current;
    const list = listRef.current;
    const rail = list?.parentElement;
    if (!frame || !list || !rail) return;
    const liveState = { value: true };
    const revealSelected = () => {
      if (!liveState.value || !list.isConnected) return;
      // The stacked cap must fit the actual header and one complete category.
      const headerHeight = `${list.getBoundingClientRect().top - rail.getBoundingClientRect().top}px`;
      if (frame.style.getPropertyValue("--vertical-tabs-header-height") !== headerHeight)
        frame.style.setProperty("--vertical-tabs-header-height", headerHeight);
      if (list.clientHeight === 0) return;
      const tab = list.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]');
      if (!tab?.isConnected) return;
      const target = tab.getBoundingClientRect();
      if (target.height === 0) return;
      const top = list.getBoundingClientRect().top + list.clientTop;
      const bottom = top + list.clientHeight;
      if (target.top < top) {
        list.scrollTop -= top - target.top;
      } else if (target.bottom > bottom) {
        list.scrollTop += Math.min(target.bottom - bottom, target.top - top);
      }
    };
    revealSelected();
    const observer = new ResizeObserver(revealSelected);
    observer.observe(list);
    observer.observe(rail);
    // Groups sit inside HoverTrack's boxless wrapper, so find them through their tabs.
    for (const tab of list.querySelectorAll('[role="tab"]')) {
      if (tab.parentElement) observer.observe(tab.parentElement);
    }
    return () => {
      liveState.value = false;
      observer.disconnect();
    };
  }, [value, railGroups]);
  const rail = stylex.props(styles.rail);
  return (
    <div {...stylex.props(styles.container)}>
      <TabsPrimitive.Root
        ref={frameRef}
        orientation="vertical"
        value={value ?? null}
        onValueChange={(next) => onValueChange(next ? String(next) : undefined)}
        {...stylex.props(styles.frame)}
      >
        <div data-split-side="end" {...rail} className={cn(rail.className, "pane-split")}>
          {railHeader}
          <TabsPrimitive.List
            ref={listRef}
            {...stylex.props(styles.list)}
            aria-label={ariaLabel}
            activateOnFocus
          >
            <HoverTrack>
              {railGroups.length === 0 && railEmpty}
              {railGroups.map((group) => (
                <div key={group.id} {...stylex.props(styles.group)}>
                  <SectionLabel {...stylex.props(styles.heading)}>{group.label}</SectionLabel>
                  {group.items.map((item) => (
                    <RailTab key={item.id} item={item} />
                  ))}
                </div>
              ))}
            </HoverTrack>
          </TabsPrimitive.List>
        </div>
        {items.map((item) => (
          <RetainedPanel key={item.id} item={item} active={item.id === value} />
        ))}
      </TabsPrimitive.Root>
    </div>
  );
}
