import * as stylex from "@stylexjs/stylex";
import {
  type ReactElement,
  type ReactNode,
  type Ref,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { cn } from "@/lib/classNames";
import {
  color,
  corner,
  motion,
  radius,
  space,
  surface,
  type,
  weight,
} from "@/styles/tokens.stylex";
import { ComboboxPrimitive } from "@/ui/primitives";
import { Icon, type IconName } from "@/ui/icons";
import { dress } from "./button";
import { Popover } from "./popover";
import { floatingRow } from "./option-row";
import { Pressable } from "./pressable";
import { vocab } from "./vocabulary";

const styles = stylex.create({
  emptyFlush: { padding: { default: null, ":is([data-empty])": 0 } },
  hideWhenEmpty: { display: { default: null, ":empty": "none" } },
  searchBox: {
    marginBottom: space.s1,
    display: "flex",
    height: "var(--control-height-md)",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s2,
    borderRadius: radius.field,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    backgroundColor: surface.canvas,
    paddingInline: "calc(var(--spacing) * 2 - var(--control-edge-width))",
    borderColor: { default: surface.field, ":focus-within": surface.fieldFocus },
    color: { default: color.fgMuted, ":focus-within": color.fg },
  },
  glyph: { flexShrink: 0 },
  input: {
    height: "100%",
    minWidth: 0,
    flex: 1,
    borderWidth: 0,
    backgroundColor: "transparent",
    fontFamily: "var(--font-sans)",
    color: color.fg,
    "::placeholder": { color: color.fgFaint },
  },
  inputShort: { height: space.s6 },

  row: { cursor: "default", userSelect: "none" },
  rowGlyph: { color: color.fgMuted },
  rowText: { minWidth: 0 },
  rowLine: { display: "flex", minWidth: 0, alignItems: "baseline", gap: space.s1_5 },
  caption: { flexShrink: 0, color: color.fgFaint },
  mark: { color: color.accent },

  stackedPopup: {
    display: "flex",
    maxHeight: "min(420px, var(--available-height))",
    width: "300px",
    maxWidth: "var(--available-width)",
    flexDirection: "column",
    overflow: "hidden",
    padding: space.s1,
  },
  splitPopup: {
    display: "flex",
    width: "400px",
    maxWidth: "var(--available-width)",
    flexDirection: "column",
    overflow: "hidden",
  },
  empty: {
    paddingInline: space.s2,
    paddingBlock: space.s6,
    textAlign: "center",
    color: color.fgFaint,
  },
  emptySplit: {
    flex: 1,
    paddingInline: space.s3,
    paddingBlock: space.s6,
    textAlign: "center",
    color: color.fgFaint,
  },
  list: {
    minHeight: 0,
    flex: 1,
    overflowY: "auto",
    overscrollBehavior: "contain",
    scrollPaddingBlock: space.s1,
  },
  listInset: { minWidth: 0, padding: space.s1 },
  scroller: {
    position: "relative",
    display: { default: null, ":has(> [data-empty])": "none" },
  },
  rowTrailingRoom: {
    gridTemplateColumns:
      "var(--menu-glyph, calc(var(--spacing) * 4)) minmax(0, 1fr) var(--catalog-accessory-width, 0px) auto",
  },
  accessory: {
    position: "absolute",
    display: "flex",
    alignItems: "center",
  },
  group: {
    display: "grid",
    rowGap: "2px",
    paddingBottom: space.s1_5,
    ":last-child": { paddingBottom: 0 },
  },
  groupLabel: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    paddingInline: space.s2,
    paddingBottom: space.s1,
    paddingTop: space.s2,
    color: color.fgFaint,
    fontWeight: weight.medium,
    userSelect: "none",
    ":first-child": { paddingTop: space.s1_5 },
  },
  groupCount: { marginLeft: "auto", fontVariantNumeric: "tabular-nums" },

  rail: {
    display: "flex",
    flexShrink: 0,
    flexDirection: "column",
    gap: space.s0_5,
    overflowY: "auto",
    scrollbarWidth: "none",
    borderRightWidth: "var(--control-edge-width)",
    borderRightStyle: "solid",
    borderRightColor: surface.divider,
    padding: space.s1,
  },
  railBody: { display: "flex", height: "280px", minHeight: 0 },
  railColumn: { display: "flex", minWidth: 0, flex: 1, flexDirection: "column" },
  railHead: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s2,
    paddingInline: space.s3,
    paddingTop: space.s2,
    paddingBottom: space.s1,
  },
  railTitle: { flexShrink: 0, fontWeight: weight.medium, color: color.fgFaint },
  railSearch: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s1_5,
    color: { default: color.fgFaint, ":focus-within": color.fg },
  },
  railSearchInput: { flex: "0 1 auto", maxWidth: "100%", fieldSizing: "content" },
  rowTrailing: {
    gridTemplateColumns: "var(--menu-glyph, calc(var(--spacing) * 4)) minmax(0, 1fr) auto",
  },
  railRow: {
    display: "grid",
    height: space.s8,
    width: space.s8,
    placeItems: "center",
    borderRadius: radius.sm,
    transitionProperty: "color, background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  railRowOn: {
    backgroundColor: { default: surface.selected, ":hover": surface.selectedHover },
    color: color.fg,
  },
  railRowOff: {
    color: { default: color.fgMuted, ":hover": color.fg },
    backgroundColor: { default: null, ":hover": surface.hover },
  },
  radio: {
    display: "grid",
    height: "var(--control-mark-size)",
    width: "var(--control-mark-size)",
    flexShrink: 0,
    placeItems: "center",
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.controlEdge,
  },
  radioOn: { borderColor: color.accent, backgroundColor: color.accent },
  radioDot: {
    height: space.s1_5,
    width: space.s1_5,
    backgroundColor: color.onAccent,
  },
});

interface CatalogPickerItem {
  id: string;
  label: string;
  icon?: IconName;
  leading?: ReactNode;
  description?: ReactNode;
  keywords?: readonly string[];
  active?: boolean;
  caption?: string;
  title?: string;
}

export interface CatalogPickerGroup {
  id: string;
  label: string;
  leading?: ReactNode;
  count?: number;
  items: CatalogPickerItem[];
}

interface CatalogSurfaceProps {
  label: string;
  placeholder: string;
  emptyLabel: string;
  onSelect: (item: CatalogPickerItem) => void;
  className?: string;
  contentClassName?: string;
  trigger?: ReactElement;
  side?: "top" | "bottom" | "left" | "right";
  align?: "start" | "center" | "end";
}

function CatalogTrigger({
  trigger,
  label,
  className,
}: {
  trigger?: ReactElement;
  label: string;
  className?: string;
}) {
  if (trigger) return <Popover.Trigger render={trigger} />;
  const trig = stylex.props(dress({ variant: "ghost", size: "icon-sm" }));
  return (
    <Popover.Trigger
      aria-label={label}
      title={label}
      data-slot="button"
      data-variant="ghost"
      {...trig}
      className={cn(trig.className, className)}
    >
      <Icon name="plus" size="sm" />
    </Popover.Trigger>
  );
}

function CatalogSearch({ placeholder, ref }: { placeholder: string; ref?: Ref<HTMLInputElement> }) {
  return (
    <div {...stylex.props(styles.searchBox)}>
      <Icon name="search" size="sm" {...stylex.props(styles.glyph)} />
      <ComboboxPrimitive.Input
        ref={ref}
        aria-label={placeholder}
        placeholder={placeholder}
        {...stylex.props(styles.input, type.uiMd)}
      />
    </div>
  );
}

type CatalogMark = "check" | "radio";

function RowMark({ mark, active }: { mark: CatalogMark; active?: boolean }) {
  if (mark === "radio") {
    return (
      <span aria-hidden {...stylex.props(styles.radio, corner.pill, active && styles.radioOn)}>
        {active && <span {...stylex.props(styles.radioDot, corner.pill)} />}
      </span>
    );
  }
  return active ? <Icon name="check" size="xs" {...stylex.props(styles.mark)} /> : <span />;
}

const ACCESSORY_ROOM = "data-catalog-accessory-room";

function CatalogRow(
  item: CatalogPickerItem,
  mark: CatalogMark,
  groupLabel?: string,
  accessoryRoom = false,
) {
  const showCaption = item.caption !== undefined && item.caption !== groupLabel;
  return (
    <ComboboxPrimitive.Item
      key={item.id}
      value={item}
      title={item.title}
      data-current={item.active ? "" : undefined}
      {...stylex.props(
        floatingRow("pick", item.description ? "lg" : "sm"),
        styles.row,
        mark === "radio" && styles.rowTrailing,
        accessoryRoom && styles.rowTrailingRoom,
      )}
    >
      {item.leading ?? (
        <Icon name={item.icon ?? "panel-r"} size="md" {...stylex.props(styles.rowGlyph)} />
      )}
      <span {...stylex.props(styles.rowText)}>
        <span {...stylex.props(styles.rowLine)}>
          <span {...stylex.props(vocab.truncate)}>{item.label}</span>
          {showCaption && item.caption && (
            <span {...stylex.props(styles.caption, type.uiXs)}>{item.caption}</span>
          )}
        </span>
        {item.description}
      </span>
      {accessoryRoom && <span aria-hidden {...{ [ACCESSORY_ROOM]: "" }} />}
      <RowMark mark={mark} active={item.active} />
    </ComboboxPrimitive.Item>
  );
}

export function CatalogPicker({
  groups,
  label,
  placeholder,
  emptyLabel,
  onSelect,
  className,
  contentClassName,
  trigger,
  side = "bottom",
  align = "end",
}: CatalogSurfaceProps & { groups: CatalogPickerGroup[] }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  return (
    <Popover.Root
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) setQuery("");
      }}
    >
      <CatalogTrigger trigger={trigger} label={label} className={className} />

      <Popover.Content
        surface="options"
        aria-label={label}
        align={align}
        side={side}
        sideOffset={6}
        className={cn(stylex.props(styles.stackedPopup).className, contentClassName)}
      >
        <ComboboxPrimitive.Root<CatalogPickerItem>
          items={groups}
          value={null}
          inputValue={query}
          onInputValueChange={setQuery}
          onValueChange={(item) => {
            if (!item) return;
            onSelect(item);
            setOpen(false);
          }}
          itemToStringLabel={(item) => [item.label, ...(item.keywords ?? [])].join(" ")}
          autoHighlight
          inline
          open
        >
          <CatalogSearch placeholder={placeholder} />

          <ComboboxPrimitive.Empty
            className={stylex.props(styles.empty, styles.hideWhenEmpty, type.uiSm).className}
          >
            {emptyLabel}
          </ComboboxPrimitive.Empty>
          <ComboboxPrimitive.List
            className={cn(
              stylex.props(styles.emptyFlush).className,
              stylex.props(styles.list).className,
            )}
          >
            {(group: CatalogPickerGroup) => (
              <ComboboxPrimitive.Group
                key={group.id}
                items={group.items}
                {...stylex.props(styles.group)}
              >
                <ComboboxPrimitive.GroupLabel {...stylex.props(styles.groupLabel, type.uiXs)}>
                  {group.leading}
                  <span>{group.label}</span>
                  {group.count !== undefined && (
                    <span {...stylex.props(styles.groupCount)}>{group.count}</span>
                  )}
                </ComboboxPrimitive.GroupLabel>
                <ComboboxPrimitive.Collection>
                  {(item: CatalogPickerItem) => CatalogRow(item, "check")}
                </ComboboxPrimitive.Collection>
              </ComboboxPrimitive.Group>
            )}
          </ComboboxPrimitive.List>
        </ComboboxPrimitive.Root>
      </Popover.Content>
    </Popover.Root>
  );
}

export function RailCatalogPicker({
  groups,
  openAtGroupId,
  heading,
  activeAccessory,
  label,
  placeholder,
  emptyLabel,
  onSelect,
  className,
  contentClassName,
  trigger,
  side = "bottom",
  align = "end",
}: CatalogSurfaceProps & {
  groups: CatalogPickerGroup[];
  openAtGroupId?: string;
  heading: string;
  activeAccessory?: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [groupId, setGroupId] = useState<string | undefined>(openAtGroupId);
  const listRef = useRef<HTMLDivElement>(null);
  const [scroller, setScroller] = useState<HTMLDivElement | null>(null);
  const accessoryRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const [accessorySpot, setAccessorySpot] = useState<{
    top: number;
    left: number;
    height: number;
  } | null>(null);

  const searching = query.trim().length > 0;
  const active = groups.find((group) => group.id === groupId) ?? groups[0];
  const items = searching
    ? [...new Map(groups.flatMap((group) => group.items).map((item) => [item.id, item])).values()]
    : (active?.items ?? []);

  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() => searchRef.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() =>
      listRef.current?.querySelector("[data-current]")?.scrollIntoView({ block: "center" }),
    );
    return () => cancelAnimationFrame(frame);
  }, [open, groupId]);

  const hasAccessory = activeAccessory !== undefined;
  const itemKey = items.map((item) => `${item.id}:${item.active ? 1 : 0}`).join("\0");
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!hasAccessory || !scroller || !list) return;
    const place = () => {
      const accessory = accessoryRef.current;
      if (accessory) {
        scroller.style.setProperty("--catalog-accessory-width", `${accessory.offsetWidth}px`);
      }
      const room = list.querySelector<HTMLElement>(`[${ACCESSORY_ROOM}]`);
      if (!room) {
        setAccessorySpot(null);
        return;
      }
      const frame = scroller.getBoundingClientRect();
      const row = room.parentElement!.getBoundingClientRect();
      const spot = room.getBoundingClientRect();
      setAccessorySpot({
        top: row.top - frame.top + scroller.scrollTop,
        left: spot.left - frame.left + scroller.scrollLeft,
        height: row.height,
      });
    };
    place();
    const observer = new ResizeObserver(place);
    observer.observe(list);
    if (accessoryRef.current) observer.observe(accessoryRef.current);
    return () => observer.disconnect();
  }, [scroller, hasAccessory, itemKey, groupId]);

  return (
    <Popover.Root
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (nextOpen) setGroupId(openAtGroupId);
        setQuery("");
      }}
    >
      <CatalogTrigger trigger={trigger} label={label} className={className} />

      <Popover.Content
        surface="options"
        aria-label={label}
        align={align}
        side={side}
        sideOffset={8}
        className={cn(stylex.props(styles.splitPopup).className, contentClassName)}
      >
        <ComboboxPrimitive.Root<CatalogPickerItem>
          items={items}
          value={null}
          inputValue={query}
          onInputValueChange={setQuery}
          onValueChange={(item) => {
            if (!item) return;
            onSelect(item);
            setOpen(false);
          }}
          itemToStringLabel={(item) => [item.label, ...(item.keywords ?? [])].join(" ")}
          autoHighlight
          inline
          open
        >
          <div data-slot="catalog-body" {...stylex.props(styles.railBody)}>
            {!searching && groups.length > 1 && (
              <div {...stylex.props(styles.rail)}>
                {groups.map((group) => (
                  <Pressable
                    key={group.id}
                    aria-pressed={group.id === active?.id}
                    aria-label={group.label}
                    title={group.label}
                    data-chrome-focus=""
                    onClick={() => {
                      setGroupId(group.id);
                    }}
                    className={
                      stylex.props(
                        styles.railRow,
                        group.id === active?.id ? styles.railRowOn : styles.railRowOff,
                      ).className
                    }
                  >
                    {group.leading}
                  </Pressable>
                ))}
              </div>
            )}

            <div {...stylex.props(styles.railColumn)}>
              <div {...stylex.props(styles.railHead)}>
                <span {...stylex.props(styles.railTitle, type.uiSm)}>{heading}</span>
                <div {...stylex.props(styles.railSearch)}>
                  <ComboboxPrimitive.Input
                    ref={searchRef}
                    aria-label={placeholder}
                    placeholder={placeholder}
                    onKeyDown={(event) => {
                      if (event.key !== "Escape" || !searching) return;
                      event.preventDefault();
                      event.stopPropagation();
                      setQuery("");
                    }}
                    {...stylex.props(
                      styles.input,
                      styles.inputShort,
                      styles.railSearchInput,
                      type.uiSm,
                    )}
                  />
                  <Icon name="search" size="sm" {...stylex.props(styles.glyph)} />
                </div>
              </div>
              <ComboboxPrimitive.Empty
                className={
                  stylex.props(styles.emptySplit, styles.hideWhenEmpty, type.uiSm).className
                }
              >
                {emptyLabel}
              </ComboboxPrimitive.Empty>
              <div ref={setScroller} {...stylex.props(styles.list, styles.scroller)}>
                <ComboboxPrimitive.List
                  ref={listRef}
                  className={stylex.props(styles.listInset).className}
                >
                  {(item: CatalogPickerItem) =>
                    CatalogRow(
                      item,
                      "radio",
                      searching ? undefined : active?.label,
                      hasAccessory && item.active === true,
                    )
                  }
                </ComboboxPrimitive.List>
                {hasAccessory && (
                  <div
                    ref={accessoryRef}
                    {...stylex.props(styles.accessory)}
                    style={
                      accessorySpot
                        ? {
                            top: accessorySpot.top,
                            left: accessorySpot.left,
                            height: accessorySpot.height,
                          }
                        : { visibility: "hidden" }
                    }
                  >
                    {activeAccessory}
                  </div>
                )}
              </div>
            </div>
          </div>
        </ComboboxPrimitive.Root>
      </Popover.Content>
    </Popover.Root>
  );
}
