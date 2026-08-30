import type { ReactNode } from "react";
import { useId, useLayoutEffect, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import { DialogPrimitive } from "@/ui/primitives";
import { Icon } from "@/ui/icons";
import { FLOATING_MOTION, MODAL_SCRIM } from "./floating-surface";
import { Kbd } from "./kbd";
import { OptionRow } from "./option-row";
import { TextField } from "./text-field";

export interface SearchOption {
  key: string;
  onSelect: () => void;
  children: ReactNode;
}

interface SearchOverlayProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  label: string;
  placeholder: string;
  /** Called on every render with the current query — the overlay owns the query so that
   *  closing resets it, the highlight and the scroll position together. */
  options: (query: string) => readonly SearchOption[];
  empty: ReactNode;
  /** Restores the control that opened a controlled dialog with no Base UI trigger in tree. */
  finalFocus?: () => HTMLElement | null;
}

function wrap(index: number, count: number, step: number) {
  if (count === 0) return 0;
  return (index + step + count) % count;
}

/**
 * Owns the rows as well as the field: the field announces the active row through
 * `aria-activedescendant` so it needs that row's id, focus never leaves the field so rows
 * must not be tab stops, and the active row has to be scrolled into view. A caller
 * rendering its own rows silently owes all three.
 */
export function SearchOverlay({
  open,
  onOpenChange,
  label,
  placeholder,
  options,
  empty,
  finalFocus,
}: SearchOverlayProps) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop data-slot="search-overlay-backdrop" className={MODAL_SCRIM} />
        <DialogPrimitive.Popup
          data-slot="search-overlay"
          aria-label={label}
          finalFocus={finalFocus}
          className={cn(
            "fixed inset-x-0 top-24 z-[var(--layer-modal)] mx-auto flex w-[min(520px,calc(100vw-32px))]",
            "flex-col overflow-hidden rounded-[var(--floating-panel-radius)] outline-none",
            // Opaque, not the ring's frosted fill: that is for small anchored popovers,
            // and at 520px over a transcript it becomes a window onto the prose beneath.
            "bg-canvas shadow-[var(--shadow-modal)]",
            FLOATING_MOTION,
          )}
        >
          <SearchOverlayContent
            key={open ? "open" : "closed"}
            open={open}
            label={label}
            placeholder={placeholder}
            options={options}
            empty={empty}
          />
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

type SearchOverlayContentProps = Omit<SearchOverlayProps, "onOpenChange" | "finalFocus">;

function SearchOverlayContent({
  open,
  label,
  placeholder,
  options,
  empty,
}: SearchOverlayContentProps) {
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const baseId = useId();
  const listboxId = `${baseId}-list`;

  const rows = options(query);
  // Clamped on read, not stored clamped: one more character can shorten the list under an
  // index held in state, and a stale index renders as no highlight and an Enter that
  // opens nothing.
  const active = rows.length === 0 ? 0 : Math.min(Math.max(highlight, 0), rows.length - 1);
  const activeId = rows.length === 0 ? undefined : `${baseId}-${active}`;

  useLayoutEffect(() => {
    if (!open) return;
    listRef.current?.querySelector("[aria-selected='true']")?.scrollIntoView({ block: "nearest" });
  }, [activeId, open]);

  return (
    <div className="contents">
      <div className="flex items-center gap-2.5 border-b border-line-soft px-3.5 py-2.5 text-fg-muted">
        <Icon name="search" size="md" />
        <TextField
          variant="bare"
          font="sans"
          // oxlint-disable-next-line jsx-a11y/no-autofocus
          autoFocus={open}
          role="combobox"
          aria-expanded
          aria-controls={listboxId}
          aria-activedescendant={activeId}
          value={query}
          onKeyDown={(event) => {
            // Arrow/Enter belong to the IME while composing; the list takes over on commit.
            if (event.nativeEvent.isComposing) return;
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
              event.preventDefault();
              setHighlight(wrap(active, rows.length, event.key === "ArrowDown" ? 1 : -1));
              return;
            }
            if (event.key === "Enter") {
              event.preventDefault();
              rows[active]?.onSelect();
            }
          }}
          onChange={(event) => {
            setQuery(event.target.value);
            setHighlight(0);
          }}
          placeholder={placeholder}
          aria-label={placeholder}
          className="flex-1"
        />
        <Kbd>esc</Kbd>
      </div>
      <div
        ref={listRef}
        id={listboxId}
        role="listbox"
        aria-label={label}
        className="max-h-80 overflow-y-auto p-1.5"
      >
        {rows.length === 0
          ? empty
          : rows.map((option, index) => (
              <OptionRow
                key={option.key}
                id={`${baseId}-${index}`}
                layout="flex"
                size="lg"
                tabIndex={-1}
                selected={index === active}
                onPointerMove={() => setHighlight(index)}
                onClick={option.onSelect}
              >
                {option.children}
              </OptionRow>
            ))}
      </div>
    </div>
  );
}
