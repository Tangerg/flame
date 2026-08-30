import { type ReactElement, type ReactNode, useState } from "react";
import { cn } from "@/lib/classNames";
import { ComboboxPrimitive } from "@/ui/primitives";
import { Icon, type IconName } from "@/ui/icons";
import { buttonStyles } from "./button";
import { Popover } from "./popover";

export interface CatalogPickerItem {
  id: string;
  label: string;
  icon?: IconName;
  leading?: ReactNode;
  description?: ReactNode;
  keywords?: readonly string[];
  active?: boolean;
}

export interface CatalogPickerGroup {
  id: string;
  label: string;
  leading?: ReactNode;
  count?: number;
  items: CatalogPickerItem[];
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
}: {
  groups: CatalogPickerGroup[];
  label: string;
  placeholder: string;
  emptyLabel: string;
  onSelect: (item: CatalogPickerItem) => void;
  className?: string;
  contentClassName?: string;
  trigger?: ReactElement;
  side?: "top" | "bottom" | "left" | "right";
  align?: "start" | "center" | "end";
}) {
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
      {trigger ? (
        <Popover.Trigger render={trigger} />
      ) : (
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
      )}

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
          <div className="mb-1 flex h-[var(--field-height-md)] shrink-0 items-center gap-2 rounded-[var(--field-radius)] border-[length:var(--control-edge-width)] border-field bg-canvas px-2.5 text-fg-muted focus-within:border-field-strong focus-within:text-fg">
            <Icon name="search" size="sm" className="shrink-0" />
            <ComboboxPrimitive.Input
              aria-label={placeholder}
              placeholder={placeholder}
              className="h-full min-w-0 flex-1 border-0 bg-transparent font-sans text-ui-md text-fg outline-none placeholder:text-fg-faint"
            />
          </div>

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
                  {(item: CatalogPickerItem) => (
                    <ComboboxPrimitive.Item
                      key={item.id}
                      value={item}
                      className={cn(
                        "grid cursor-default grid-cols-[16px_minmax(0,1fr)_14px] items-center gap-2 rounded-[var(--shape-sm)] px-2.5 text-ui-md text-fg outline-none select-none data-[highlighted]:bg-hover",
                        item.description ? "min-h-11 py-1.5" : "min-h-9",
                      )}
                    >
                      {item.leading ?? (
                        <Icon name={item.icon ?? "panel-r"} size="sm" className="text-fg-muted" />
                      )}
                      <span className="min-w-0">
                        <span className="block truncate">{item.label}</span>
                        {item.description}
                      </span>
                      {item.active ? (
                        <Icon name="check" size="xs" className="text-accent" />
                      ) : (
                        <span />
                      )}
                    </ComboboxPrimitive.Item>
                  )}
                </ComboboxPrimitive.Collection>
              </ComboboxPrimitive.Group>
            )}
          </ComboboxPrimitive.List>
        </ComboboxPrimitive.Root>
      </Popover.Content>
    </Popover.Root>
  );
}
