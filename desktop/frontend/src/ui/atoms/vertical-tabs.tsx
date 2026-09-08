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
} from "@/styles/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";
import { TabsPrimitive } from "@/ui/primitives";
import { SectionLabel } from "./section-label";

interface VerticalTabItem {
  id: string;
  label: ReactNode;
  icon?: IconName;
  content: ReactNode;
}

interface VerticalTabGroup {
  id: string;
  label: ReactNode;
  items: VerticalTabItem[];
}

const styles = stylex.create({
  // A fixed rail and a measured page: the rail holds every label at full length, and the page
  // is capped so a line of prose stays readable however wide the window gets.
  frame: {
    display: "grid",
    height: "100%",
    width: "100%",
    gridTemplateColumns: "256px 1fr",
    overflow: "hidden",
    backgroundColor: surface.canvas,
  },
  rail: {
    display: "flex",
    minHeight: 0,
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
    paddingInline: "var(--density-navigation-gutter)",
    paddingBottom: space.s6,
  },
  group: { display: "flex", flexDirection: "column", gap: "1px" },
  // A settings pane is chosen from a NAVIGATION rail, so it is measured in the same three
  // tokens every other rail in the product uses. It was the one rail on `--control-height-md`
  // with its own gap and inset, which made it the one rail the Appearance density setting
  // could not reach — a setting whose own copy promises "row heights, gutters".
  tab: {
    display: "flex",
    height: "var(--density-row-height)",
    alignItems: "center",
    gap: "var(--density-row-gap)",
    borderRadius: radius.button,
    borderWidth: 0,
    backgroundColor: {
      default: "transparent",
      ":hover": surface.hover,
      ":is([data-active])": surface.selected,
    },
    paddingInline: space.s2,
    textAlign: "left",
    fontFamily: "var(--font-sans)",
    fontWeight: weight.regular,
    lineHeight: leading.tight,
    color: color.fg,
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: "var(--ease-out)",
    outline: { default: null, ":focus-visible": "none" },
  },
  glyph: { flexShrink: 0 },
  label: { overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" },
  page: { minHeight: 0, minWidth: 0, overflowY: "auto", backgroundColor: surface.canvas },
  measure: {
    marginInline: "auto",
    maxWidth: "720px",
    paddingInline: space.s6,
    paddingBlock: space.s8,
  },
  panel: { outline: "none" },
  heading: { paddingInline: space.s2, paddingBottom: space.s1, paddingTop: space.s4 },
});

export function VerticalTabs({
  ariaLabel,
  groups,
  value,
  onValueChange,
  railHeader,
}: {
  ariaLabel: string;
  groups: VerticalTabGroup[];
  value?: string;
  onValueChange: (value: string | undefined) => void;
  railHeader?: ReactNode;
}) {
  const items = groups.flatMap((group) => group.items);
  // `pane-split` is the seam this rail casts toward the page — a globals mechanism keyed on
  // `data-split-side`, so it composes with the rail's own class list rather than replacing it.
  // Spreading `stylex.props` AFTER a `className` silently drops that class.
  const rail = stylex.props(styles.rail);
  return (
    <TabsPrimitive.Root
      orientation="vertical"
      value={value ?? null}
      onValueChange={(next) => onValueChange(next ? String(next) : undefined)}
      {...stylex.props(styles.frame)}
    >
      <div data-split-side="end" {...rail} className={cn(rail.className, "pane-split")}>
        {railHeader}
        <TabsPrimitive.List {...stylex.props(styles.list)} aria-label={ariaLabel} activateOnFocus>
          {groups.map((group) => (
            <div key={group.id} {...stylex.props(styles.group)}>
              <SectionLabel {...stylex.props(styles.heading)}>{group.label}</SectionLabel>
              {group.items.map((item) => (
                <TabsPrimitive.Tab
                  key={item.id}
                  value={item.id}
                  {...stylex.props(styles.tab, type.uiMd)}
                >
                  {item.icon && <Icon name={item.icon} size="md" {...stylex.props(styles.glyph)} />}
                  <span {...stylex.props(styles.label)}>{item.label}</span>
                </TabsPrimitive.Tab>
              ))}
            </div>
          ))}
        </TabsPrimitive.List>
      </div>
      <div {...stylex.props(styles.page)}>
        <div {...stylex.props(styles.measure)}>
          {items.map((item) => (
            <TabsPrimitive.Panel key={item.id} value={item.id} {...stylex.props(styles.panel)}>
              {item.content}
            </TabsPrimitive.Panel>
          ))}
        </div>
      </div>
    </TabsPrimitive.Root>
  );
}
