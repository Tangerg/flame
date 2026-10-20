import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
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
import { Icon, type IconName } from "@/ui/icons";
import { TabsPrimitive } from "@/ui/primitives";
import { ItemHighlight, HoverTrack, useHoverTrackItem } from "@flame/ui";
import { SectionLabel } from "./section-label";

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
      "@container (max-width: 40rem)": "fit-content(40%) minmax(0, 1fr)",
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
    display: "flex",
    height: "var(--density-row-height)",
    alignItems: "center",
    gap: "var(--density-row-gap)",
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
  glyph: { flexShrink: 0 },
  label: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  page: {
    minHeight: 0,
    minWidth: 0,
    overflowY: "auto",
    overscrollBehavior: "contain",
    scrollPaddingBlock: space.s4,
    backgroundColor: surface.canvas,
  },
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
      <span {...stylex.props(styles.label)}>{item.label}</span>
    </TabsPrimitive.Tab>
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
  const rail = stylex.props(styles.rail);
  return (
    <div {...stylex.props(styles.container)}>
      <TabsPrimitive.Root
        orientation="vertical"
        value={value ?? null}
        onValueChange={(next) => onValueChange(next ? String(next) : undefined)}
        {...stylex.props(styles.frame)}
      >
        <div data-split-side="end" {...rail} className={cn(rail.className, "pane-split")}>
          {railHeader}
          <TabsPrimitive.List {...stylex.props(styles.list)} aria-label={ariaLabel} activateOnFocus>
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
        <div {...stylex.props(styles.page)}>
          {items.map((item) => (
            <TabsPrimitive.Panel key={item.id} value={item.id} aria-label={item.label}>
              {item.content}
            </TabsPrimitive.Panel>
          ))}
        </div>
      </TabsPrimitive.Root>
    </div>
  );
}
