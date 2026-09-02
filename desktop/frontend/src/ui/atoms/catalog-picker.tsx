import { type ReactElement, type ReactNode, type Ref, useEffect, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import { ComboboxPrimitive } from "@/ui/primitives";
import { Icon, type IconName } from "@/ui/icons";
import { buttonStyles } from "./button";
import { Popover } from "./popover";
import { Pressable } from "./pressable";

export interface CatalogPickerItem {
  id: string;
  label: string;
  icon?: IconName;
  leading?: ReactNode;
  description?: ReactNode;
  keywords?: readonly string[];
  active?: boolean;
  /** Which group this entry belongs to, for the lists that are not scoped to one — a flat
   *  page of search results cannot say it, and a recents shelf holds entries from several. */
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
  return (
    <Popover.Trigger
      aria-label={label}
      title={label}
      data-slot="button"
      data-variant="ghost"
      className={cn(
        buttonStyles({ variant: "ghost", size: "icon-sm" }),
        "data-[popup-open]:bg-selected data-[popup-open]:text-fg",
        className,
      )}
    >
      <Icon name="plus" size="sm" />
    </Popover.Trigger>
  );
}

function CatalogSearch({
  placeholder,
  onEscape,
  ref,
}: {
  placeholder: string;
  onEscape?: (event: React.KeyboardEvent) => void;
  ref?: Ref<HTMLInputElement>;
}) {
  return (
    <div className="mb-1 flex h-[var(--field-height-md)] shrink-0 items-center gap-2 rounded-[var(--field-radius)] border-[length:var(--control-edge-width)] border-field bg-canvas px-2.5 text-fg-muted focus-within:border-field-strong focus-within:text-fg">
      <Icon name="search" size="sm" className="shrink-0" />
      <ComboboxPrimitive.Input
        ref={ref}
        aria-label={placeholder}
        placeholder={placeholder}
        onKeyDown={onEscape}
        className="h-full min-w-0 flex-1 border-0 bg-transparent font-sans text-ui-md text-fg outline-none placeholder:text-fg-faint"
      />
    </div>
  );
}

function CatalogRow(item: CatalogPickerItem, groupLabel?: string) {
  // The caption earns its place only where the list is not already scoped to it: a page of
  // search results, or a shelf holding entries from several groups. Inside a group it would
  // repeat that group's own heading on every row.
  const showCaption = item.caption !== undefined && item.caption !== groupLabel;
  return (
    <ComboboxPrimitive.Item
      key={item.id}
      value={item}
      data-current={item.active ? "" : undefined}
      className={cn(
        "grid cursor-default grid-cols-[16px_minmax(0,1fr)_14px] items-center gap-2 rounded-[var(--shape-sm)] px-2.5 text-ui-md text-fg outline-none select-none data-[highlighted]:bg-hover",
        item.description ? "min-h-11 py-1.5" : "min-h-9",
      )}
    >
      {item.leading ?? <Icon name={item.icon ?? "panel-r"} size="sm" className="text-fg-muted" />}
      <span className="min-w-0">
        <span className="flex min-w-0 items-baseline gap-1.5">
          <span className="truncate">{item.label}</span>
          {showCaption && item.caption && (
            <span className="shrink-0 text-ui-xs text-fg-faint">{item.caption}</span>
          )}
        </span>
        {item.description}
      </span>
      {item.active ? <Icon name="check" size="xs" className="text-accent" /> : <span />}
    </ComboboxPrimitive.Item>
  );
}

/** Every group stacked in one scroller. For a catalogue whose groups are short enough to
 *  read at once — a handful of views, a handful of recipes. */
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
        className={cn(
          "flex max-h-[min(420px,var(--available-height))] w-[300px] max-w-[var(--available-width)] flex-col overflow-hidden p-1.5",
          contentClassName,
        )}
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

          <ComboboxPrimitive.Empty className="empty:hidden px-2.5 py-6 text-center text-ui-sm text-fg-faint">
            {emptyLabel}
          </ComboboxPrimitive.Empty>
          <ComboboxPrimitive.List className="min-h-0 flex-1 overflow-y-auto overscroll-contain scroll-py-1 outline-none data-empty:p-0">
            {(group: CatalogPickerGroup) => (
              <ComboboxPrimitive.Group
                key={group.id}
                items={group.items}
                className="pb-1.5 last:pb-0"
              >
                <ComboboxPrimitive.GroupLabel className="flex items-center gap-2 px-2.5 pb-1 pt-2 text-ui-xs font-medium text-fg-faint select-none first:pt-1.5">
                  {group.leading}
                  <span>{group.label}</span>
                  {group.count !== undefined && (
                    <span className="ml-auto tabular-nums">{group.count}</span>
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
 * An explicit component rather than a mode on the one above: a catalogue deep enough to need
 * a rail needs a measure that does not move, a group to open on, and a search that leaves the
 * rail behind — and none of those mean anything to a stacked list. The two share the row, the
 * field and the trigger.
 *
 * A query searches EVERY group, because a reader who types a name is no longer thinking in
 * groups. Items are deduplicated by id on that path, so a group that republishes another's
 * entries — a recents shelf — does not answer twice, and `item.caption` carries the group a
 * result came from, which is the one thing a flat list of results cannot say for itself.
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

  // The caret goes to the search ON OPEN, and only on open: a catalogue this deep is reached
  // by typing at least as often as by pointing, and a field that has to be clicked into first
  // costs a gesture. Re-running this when the GROUP changes would take focus away from the
  // rail the reader just used — a keyboard user could never stay on it to try a second group.
  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() => searchRef.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [open]);

  // Opening onto the group in force is only half of it — the entry itself can sit below the
  // fold of a long group, and a picker that opens without showing what it is picking from
  // makes the reader scroll to find out what they already have.
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
        className={cn(
          "flex w-[400px] max-w-[var(--available-width)] flex-col overflow-hidden",
          contentClassName,
        )}
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
          <div className="flex shrink-0 items-center gap-2 border-b border-divider px-3 py-2 text-fg-muted focus-within:text-fg">
            <Icon name="search" size="sm" className="shrink-0" />
            <ComboboxPrimitive.Input
              ref={searchRef}
              aria-label={placeholder}
              placeholder={placeholder}
              onKeyDown={(event) => {
                // A query is the reader's narrowing, not their exit. Escape gives that back
                // first and only closes once there is nothing left to give back.
                if (event.key !== "Escape" || !searching) return;
                event.preventDefault();
                event.stopPropagation();
                setQuery("");
              }}
              className="h-6 min-w-0 flex-1 border-0 bg-transparent font-sans text-ui-md text-fg outline-none placeholder:text-fg-faint"
            />
          </div>

          {/* A measure that does not move: the surface is anchored to a composer control, so a
              body that grows with its group walks the whole popover up the screen. */}
          <div className="flex h-[240px] min-h-0">
            {!searching && groups.length > 1 && (
              // Toggle buttons, not a tablist: the pane beside this is the combobox's own
              // listbox, and a `tablist` whose panel is a `listbox` is an unallowed pairing
              // that axe reports. `aria-pressed` says the same thing and needs no roving
              // tabindex, which nothing here may hand-roll.
              <div className="flex w-[132px] shrink-0 flex-col gap-0.5 overflow-y-auto border-r border-divider p-1.5">
                {groups.map((group) => (
                  <Pressable
                    key={group.id}
                    aria-pressed={group.id === active?.id}
                    data-chrome-focus=""
                    onClick={() => {
                      setGroupId(group.id);
                    }}
                    className={cn(
                      "flex min-h-7 items-center gap-2 rounded-[var(--shape-sm)] px-2 text-ui-sm",
                      "transition-colors duration-[var(--dur-color)]",
                      group.id === active?.id
                        ? "bg-selected text-fg"
                        : "text-fg-muted hover:bg-hover hover:text-fg",
                    )}
                  >
                    {group.leading}
                    <span className="min-w-0 flex-1 truncate">{group.label}</span>
                    {group.count !== undefined && (
                      <span
                        aria-hidden
                        className="shrink-0 font-mono text-ui-xs text-fg-faint tabular-nums"
                      >
                        {group.count}
                      </span>
                    )}
                  </Pressable>
                ))}
              </div>
            )}

            <ComboboxPrimitive.Empty className="empty:hidden flex-1 px-3 py-6 text-center text-ui-sm text-fg-faint">
              {emptyLabel}
            </ComboboxPrimitive.Empty>
            <ComboboxPrimitive.List
              ref={listRef}
              className="min-w-0 flex-1 overflow-y-auto overscroll-contain scroll-py-1 p-1.5 outline-none data-empty:hidden"
            >
              {(item: CatalogPickerItem) => CatalogRow(item, searching ? undefined : active?.label)}
            </ComboboxPrimitive.List>
          </div>
        </ComboboxPrimitive.Root>
      </Popover.Content>
    </Popover.Root>
  );
}
