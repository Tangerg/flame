import * as stylex from "@stylexjs/stylex";
import { type ReactElement, type ReactNode, type Ref, useEffect, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type, weight } from "@/styles/tokens.stylex";
import { ComboboxPrimitive } from "@/ui/primitives";
import { Icon, type IconName } from "@/ui/icons";
import { dress } from "./button";
import { Popover } from "./popover";
import { Pressable } from "./pressable";
import { vocab } from "./vocabulary";

const styles = stylex.create({
  // `data-empty:` was Tailwind's variant syntax over a data attribute; StyleX says the same
  // thing natively, and says it at a specificity no utility can undo.
  emptyFlush: { padding: { default: null, ":is([data-empty])": 0 } },
  // A row with nothing in it takes no space. `:empty` and `[data-empty]` are two different
  // questions — one asks whether the element has children, the other whether the LIST it heads
  // is empty — so both live here rather than one standing in for the other.
  hideWhenEmpty: { display: { default: null, ":empty": "none" } },
  hideWhenListEmpty: { display: { default: null, ":is([data-empty])": "none" } },
  // The search box sits INSIDE the popup, so it wears the field's edge rather than the popup's.
  searchBox: {
    marginBottom: space.s1,
    display: "flex",
    height: "var(--field-height-md)",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s2,
    borderRadius: radius.field,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    backgroundColor: surface.canvas,
    paddingInline: space.s2_5,
    // The whole box answers the keyboard, not just the input inside it.
    borderColor: { default: surface.field, ":focus-within": surface.fieldStrong },
    color: { default: color.fgMuted, ":focus-within": color.fg },
  },
  // A search box with a rule under it instead of a border around it: the two-column popup has
  // its own frame, and a second box inside it would read as a nested panel.
  searchRule: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s2,
    borderBottomWidth: "1px",
    borderBottomStyle: "solid",
    borderBottomColor: surface.divider,
    paddingInline: space.s3,
    paddingBlock: space.s2,
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
    outline: "none",
    "::placeholder": { color: color.fgFaint },
  },
  inputShort: { height: space.s6 },

  row: {
    display: "grid",
    cursor: "default",
    gridTemplateColumns: "16px minmax(0, 1fr) 14px",
    alignItems: "center",
    gap: space.s2,
    borderRadius: radius.sm,
    paddingInline: space.s2_5,
    color: color.fg,
    outline: "none",
    userSelect: "none",
    backgroundColor: { default: null, ":is([data-highlighted])": surface.hover },
  },
  // A row that carries a description is two lines tall and needs its own inset; one that does
  // not is a single line and takes the shorter step.
  rowTall: { minHeight: "calc(var(--spacing) * 11)", paddingBlock: space.s1_5 },
  rowShort: { minHeight: space.s9 },
  rowGlyph: { color: color.fgMuted },
  rowText: { minWidth: 0 },
  rowLine: { display: "flex", minWidth: 0, alignItems: "baseline", gap: space.s1_5 },
  caption: { flexShrink: 0, color: color.fgFaint },
  mark: { color: color.accent },

  // One scroller for a catalogue short enough to read at once, and a measured popup so a long
  // label does not decide the width.
  stackedPopup: {
    display: "flex",
    maxHeight: "min(420px, var(--available-height))",
    width: "300px",
    maxWidth: "var(--available-width)",
    flexDirection: "column",
    overflow: "hidden",
    padding: space.s1_5,
  },
  splitPopup: {
    display: "flex",
    width: "400px",
    maxWidth: "var(--available-width)",
    flexDirection: "column",
    overflow: "hidden",
  },
  empty: {
    paddingInline: space.s2_5,
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
    outline: "none",
  },
  listInset: { minWidth: 0, padding: space.s1_5 },
  group: { paddingBottom: space.s1_5, ":last-child": { paddingBottom: 0 } },
  groupLabel: {
    display: "flex",
    alignItems: "center",
    gap: space.s2,
    paddingInline: space.s2_5,
    paddingBottom: space.s1,
    paddingTop: space.s2,
    color: color.fgFaint,
    fontWeight: weight.medium,
    userSelect: "none",
    ":first-child": { paddingTop: space.s1_5 },
  },
  groupCount: { marginLeft: "auto", fontVariantNumeric: "tabular-nums" },

  // The split popup's rail: a fixed measure, because the groups are a stable set and a rail
  // that resizes with the longest label makes the list jump between filters.
  rail: {
    display: "flex",
    width: "132px",
    flexShrink: 0,
    flexDirection: "column",
    gap: space.s0_5,
    overflowY: "auto",
    borderRightWidth: "1px",
    borderRightStyle: "solid",
    borderRightColor: surface.divider,
    padding: space.s1_5,
  },
  railBody: { display: "flex", height: "240px", minHeight: 0 },
  railRow: {
    display: "flex",
    minHeight: space.s7,
    alignItems: "center",
    gap: space.s2,
    borderRadius: radius.sm,
    paddingInline: space.s2,
    transitionProperty: "color, background-color",
    transitionDuration: motion.color,
  },
  railRowOn: { backgroundColor: surface.selected, color: color.fg },
  railRowOff: {
    color: { default: color.fgMuted, ":hover": color.fg },
    backgroundColor: { default: null, ":hover": surface.hover },
  },
  railLabel: {
    minWidth: 0,
    flex: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  railCount: { flexShrink: 0, fontFamily: "var(--font-mono)", color: color.fgFaint },
});

interface CatalogPickerItem {
  id: string;
  label: string;
  icon?: IconName;
  leading?: ReactNode;
  description?: ReactNode;
  keywords?: readonly string[];
  active?: boolean;
  /** The entry's group, for the lists that are not scoped to one. */
  caption?: string;
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

function CatalogRow(item: CatalogPickerItem, groupLabel?: string) {
  const showCaption = item.caption !== undefined && item.caption !== groupLabel;
  return (
    <ComboboxPrimitive.Item
      key={item.id}
      value={item}
      data-current={item.active ? "" : undefined}
      {...stylex.props(styles.row, type.uiMd, item.description ? styles.rowTall : styles.rowShort)}
    >
      {item.leading ?? (
        <Icon name={item.icon ?? "panel-r"} size="sm" {...stylex.props(styles.rowGlyph)} />
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
      {item.active ? <Icon name="check" size="xs" {...stylex.props(styles.mark)} /> : <span />}
    </ComboboxPrimitive.Item>
  );
}

/** Every group stacked in one scroller, for a catalogue short enough to read at once. */
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
                  {(item: CatalogPickerItem) => CatalogRow(item)}
                </ComboboxPrimitive.Collection>
              </ComboboxPrimitive.Group>
            )}
          </ComboboxPrimitive.List>
        </ComboboxPrimitive.Root>
      </Popover.Content>
    </Popover.Root>
  );
}

/**
 * A group rail down one side, a fixed viewport beside it, and a query that replaces both.
 *
 * A query searches EVERY group and deduplicates by id, so a shelf that republishes another
 * group's entries does not answer twice.
 */
export function RailCatalogPicker({
  groups,
  openAtGroupId,
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
  /** Where the rail opens — the group holding what is in force, which only the caller knows. */
  openAtGroupId?: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [groupId, setGroupId] = useState<string | undefined>(openAtGroupId);
  const listRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  const searching = query.trim().length > 0;
  const active = groups.find((group) => group.id === groupId) ?? groups[0];
  const items = searching
    ? [...new Map(groups.flatMap((group) => group.items).map((item) => [item.id, item])).values()]
    : (active?.items ?? []);

  // On open only: re-running on a GROUP change would pull focus off the rail the reader is
  // still using.
  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() => searchRef.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [open]);

  // The entry in force can sit below the fold of a long group.
  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() =>
      listRef.current?.querySelector("[data-current]")?.scrollIntoView({ block: "center" }),
    );
    return () => cancelAnimationFrame(frame);
  }, [open, groupId]);

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
          <div {...stylex.props(styles.searchRule)}>
            <Icon name="search" size="sm" {...stylex.props(styles.glyph)} />
            <ComboboxPrimitive.Input
              ref={searchRef}
              aria-label={placeholder}
              placeholder={placeholder}
              onKeyDown={(event) => {
                // Escape clears the query first and closes only once there is none.
                if (event.key !== "Escape" || !searching) return;
                event.preventDefault();
                event.stopPropagation();
                setQuery("");
              }}
              {...stylex.props(styles.input, styles.inputShort, type.uiMd)}
            />
          </div>

          {/* A measure that does not move: the surface is anchored to a composer control, so a
              body that grows with its group walks the whole popover up the screen. Named,
              because that is a claim about geometry and the only place it can be checked is a
              browser — a jsdom test can reach the element but never its height. */}
          <div data-slot="catalog-body" {...stylex.props(styles.railBody)}>
            {!searching && groups.length > 1 && (
              // Toggle buttons, not a tablist: a `tablist` whose panel is the combobox's
              // `listbox` is a pairing axe reports.
              <div {...stylex.props(styles.rail)}>
                {groups.map((group) => (
                  <Pressable
                    key={group.id}
                    aria-pressed={group.id === active?.id}
                    data-chrome-focus=""
                    onClick={() => {
                      setGroupId(group.id);
                    }}
                    className={
                      stylex.props(
                        styles.railRow,
                        type.uiSm,
                        group.id === active?.id ? styles.railRowOn : styles.railRowOff,
                      ).className
                    }
                  >
                    {group.leading}
                    <span {...stylex.props(styles.railLabel)}>{group.label}</span>
                    {group.count !== undefined && (
                      <span aria-hidden {...stylex.props(styles.railCount, type.uiXs)}>
                        {group.count}
                      </span>
                    )}
                  </Pressable>
                ))}
              </div>
            )}

            <ComboboxPrimitive.Empty
              className={stylex.props(styles.emptySplit, styles.hideWhenEmpty, type.uiSm).className}
            >
              {emptyLabel}
            </ComboboxPrimitive.Empty>
            <ComboboxPrimitive.List
              ref={listRef}
              // One `stylex.props` call, not two joined: StyleX resolves precedence WITHIN a
              // call, and two class lists concatenated leave it to stylesheet order.
              className={
                stylex.props(styles.hideWhenListEmpty, styles.list, styles.listInset).className
              }
            >
              {(item: CatalogPickerItem) => CatalogRow(item, searching ? undefined : active?.label)}
            </ComboboxPrimitive.List>
          </div>
        </ComboboxPrimitive.Root>
      </Popover.Content>
    </Popover.Root>
  );
}
