import * as stylex from "@stylexjs/stylex";
import { type ReactElement, type ReactNode, type Ref, useEffect, useRef, useState } from "react";
import { SearchField } from "@flame/ui";
import { cn } from "@/lib/classNames";
import { color, motion, radius, space, surface, type, weight } from "@flame/ui/tokens.stylex";
import { Combobox as ComboboxPrimitive } from "@base-ui/react/combobox";
import { Icon, type IconName } from "@/ui/icons/index";
import { dress } from "@flame/ui/button";
import { Popover } from "@flame/ui";
import { floatingRow } from "@/ui/atoms/option-row";
import { Pressable } from "@/ui/atoms/pressable";
import { vocab } from "@flame/ui/vocabulary";
const styles = stylex.create({
  content: { display: "flex", flex: 1, minHeight: 0, flexDirection: "column", gap: space.s1 },
  emptyFlush: { padding: { default: null, ":is([data-empty])": 0 } },
  hideWhenEmpty: { display: { default: null, ":empty": "none" } },
  searchBox: { flexShrink: 0 },

  row: { cursor: "default", userSelect: "none" },
  rowGlyph: { color: color.fgMuted },
  rowText: { minWidth: 0 },
  rowLine: { display: "flex", minWidth: 0, alignItems: "baseline", gap: space.s1_5 },
  caption: { flexShrink: 0, color: color.fgFaint },
  mark: { color: color.accentText },

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
  hideWhenListEmpty: { display: { default: null, ":is([data-empty])": "none" } },
  group: {
    display: "grid",
    rowGap: space.s0_5,
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
  railBody: { display: "flex", maxHeight: "min(280px, var(--available-height))", minHeight: 0 },
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
  rowUnmarked: {
    gridTemplateColumns: "var(--menu-glyph, calc(var(--spacing) * 4)) minmax(0, 1fr)",
  },
  rowCurrent: {
    backgroundColor: {
      default: surface.selected,
      ":is([data-highlighted]):not(:disabled, [aria-disabled=true], [data-disabled])":
        surface.selectedHover,
    },
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
  clearLabel: string;
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

function CatalogSearch({
  placeholder,
  clearLabel,
  value,
  onClear,
  ref,
}: {
  placeholder: string;
  clearLabel: string;
  value: string;
  onClear: () => void;
  ref?: Ref<HTMLInputElement>;
}) {
  return (
    <ComboboxPrimitive.Input
      ref={ref}
      render={
        <SearchField
          aria-label={placeholder}
          placeholder={placeholder}
          value={value}
          onClear={onClear}
          clearLabel={clearLabel}
          styles={styles.searchBox}
        />
      }
    />
  );
}

type CatalogMark = "check" | "tint";

function CatalogRow(
  item: CatalogPickerItem,
  mark: CatalogMark,
  groupLabel?: string,
  size: "sm" | "md" = "sm",
) {
  const showCaption = item.caption !== undefined && item.caption !== groupLabel;
  return (
    <ComboboxPrimitive.Item
      key={item.id}
      value={item}
      title={item.title ?? item.label}
      data-current={item.active ? "" : undefined}
      {...stylex.props(
        floatingRow("pick", item.description ? "lg" : size),
        styles.row,
        mark === "tint" && styles.rowUnmarked,
        mark === "tint" && item.active && styles.rowCurrent,
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
      {mark === "check" &&
        (item.active ? <Icon name="check" size="xs" {...stylex.props(styles.mark)} /> : <span />)}
    </ComboboxPrimitive.Item>
  );
}

export function CatalogList({
  groups,
  label,
  placeholder,
  emptyLabel,
  clearLabel,
  onSelect,
  inputRef,
}: Pick<CatalogSurfaceProps, "label" | "placeholder" | "emptyLabel" | "clearLabel" | "onSelect"> & {
  groups: CatalogPickerGroup[];
  inputRef?: Ref<HTMLInputElement>;
}) {
  const [query, setQuery] = useState("");
  return (
    <ComboboxPrimitive.Root<CatalogPickerItem>
      items={groups}
      value={null}
      inputValue={query}
      onInputValueChange={setQuery}
      onValueChange={(item) => {
        if (item) onSelect(item);
      }}
      itemToStringLabel={(item) => [item.label, ...(item.keywords ?? [])].join(" ")}
      autoHighlight
      inline
      open
    >
      <div {...stylex.props(styles.content)}>
        <CatalogSearch
          ref={inputRef}
          placeholder={placeholder}
          clearLabel={clearLabel}
          value={query}
          onClear={() => setQuery("")}
        />
        <ComboboxPrimitive.Empty
          className={stylex.props(styles.empty, styles.hideWhenEmpty, type.uiSm).className}
        >
          {emptyLabel}
        </ComboboxPrimitive.Empty>
        <ComboboxPrimitive.List
          aria-label={label}
          {...stylex.props(styles.emptyFlush, styles.list)}
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
                {(item: CatalogPickerItem) => CatalogRow(item, "check", undefined, "md")}
              </ComboboxPrimitive.Collection>
            </ComboboxPrimitive.Group>
          )}
        </ComboboxPrimitive.List>
      </div>
    </ComboboxPrimitive.Root>
  );
}

export function CatalogPicker({
  groups,
  label,
  placeholder,
  emptyLabel,
  clearLabel,
  onSelect,
  className,
  contentClassName,
  trigger,
  side = "bottom",
  align = "end",
}: CatalogSurfaceProps & { groups: CatalogPickerGroup[] }) {
  const [open, setOpen] = useState(false);
  const selection = useRef<{ deliver: (() => void) | undefined } | null>(null);

  return (
    <Popover.Root
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (nextOpen) selection.current = null;
      }}
      onOpenChangeComplete={(open) => {
        if (open) return;
        // Keep the selection marker: Base UI may settle focus after this delivery.
        const select = selection.current?.deliver;
        if (selection.current) selection.current.deliver = undefined;
        select?.();
      }}
    >
      <CatalogTrigger trigger={trigger} label={label} className={className} />

      <Popover.Content
        aria-label={label}
        finalFocus={() => selection.current === null}
        align={align}
        side={side}
        sideOffset={6}
        className={cn(stylex.props(styles.stackedPopup).className, contentClassName)}
      >
        <CatalogList
          groups={groups}
          label={label}
          placeholder={placeholder}
          emptyLabel={emptyLabel}
          clearLabel={clearLabel}
          onSelect={(item) => {
            selection.current = { deliver: () => onSelect(item) };
            setOpen(false);
          }}
        />
      </Popover.Content>
    </Popover.Root>
  );
}

export function RailCatalogPicker({
  groups,
  openAtGroupId,
  label,
  placeholder,
  emptyLabel,
  clearLabel,
  onSelect,
  className,
  contentClassName,
  trigger,
  side = "bottom",
  align = "end",
}: CatalogSurfaceProps & {
  groups: CatalogPickerGroup[];
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
          <div data-slot="catalog-body" {...stylex.props(styles.railBody)}>
            {!searching && groups.length > 1 && (
              <div {...stylex.props(styles.rail)}>
                {groups.map((group) => (
                  <Pressable
                    key={group.id}
                    aria-pressed={group.id === active?.id}
                    aria-label={group.label}
                    title={group.label}
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
                <CatalogSearch
                  ref={searchRef}
                  placeholder={placeholder}
                  clearLabel={clearLabel}
                  value={query}
                  onClear={() => setQuery("")}
                />
              </div>
              <ComboboxPrimitive.Empty
                className={
                  stylex.props(styles.emptySplit, styles.hideWhenEmpty, type.uiSm).className
                }
              >
                {emptyLabel}
              </ComboboxPrimitive.Empty>
              <ComboboxPrimitive.List
                ref={listRef}
                className={
                  stylex.props(styles.hideWhenListEmpty, styles.list, styles.listInset).className
                }
              >
                {(item: CatalogPickerItem) =>
                  CatalogRow(item, "tint", searching ? undefined : active?.label)
                }
              </ComboboxPrimitive.List>
            </div>
          </div>
        </ComboboxPrimitive.Root>
      </Popover.Content>
    </Popover.Root>
  );
}
