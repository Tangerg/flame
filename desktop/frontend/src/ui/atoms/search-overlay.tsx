import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { useId, useLayoutEffect, useRef, useState } from "react";
import { DialogPrimitive } from "@/ui/primitives";
import { Icon } from "@/ui/icons";
import { MODAL_SCRIM, modalPanel } from "./floating-surface";
import { color, radius, space, surface } from "@/styles/tokens.stylex";
import { Kbd } from "./kbd";
import { OptionRow } from "./option-row";
import { TextField } from "./text-field";

const styles = stylex.create({
  panel: {
    display: "flex",
    width: "min(520px, calc(100vw - 32px))",
    flexDirection: "column",
    overflow: "hidden",
    borderRadius: radius.floatingPanel,
    backgroundColor: surface.canvas,
  },
  // The popup is the flex column; this wrapper exists to key the content, not to lay it out.
  contents: { display: "contents" },
  queryRow: {
    display: "flex",
    alignItems: "center",
    gap: space.s2_5,
    borderBottomWidth: "1px",
    borderBottomStyle: "solid",
    borderBottomColor: surface.lineSoft,
    paddingInline: "calc(var(--spacing) * 3.5)",
    paddingBlock: space.s2_5,
    color: color.fgMuted,
  },
  input: { flex: 1 },
  list: { maxHeight: "calc(var(--spacing) * 80)", overflowY: "auto", padding: space.s1_5 },
});

interface SearchOption {
  key: string;
  onSelect: () => void;
  children: ReactNode;
}

interface SearchOverlayProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  label: string;
  placeholder: string;
  options: (query: string) => readonly SearchOption[];
  empty: ReactNode;
}

function wrap(index: number, count: number, step: number) {
  if (count === 0) return 0;
  return (index + step + count) % count;
}

export function SearchOverlay({
  open,
  onOpenChange,
  label,
  placeholder,
  options,
  empty,
}: SearchOverlayProps) {
  // A controlled dialog has no trigger node to hand focus back to, and this render is the last
  // moment the element that opened it still holds focus — the popup takes it once it mounts.
  const [wasOpen, setWasOpen] = useState(open);
  const [opener, setOpener] = useState<HTMLElement | null>(null);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open)
      setOpener(document.activeElement instanceof HTMLElement ? document.activeElement : null);
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop
          data-slot="search-overlay-backdrop"
          className={stylex.props(MODAL_SCRIM).className}
        />
        <DialogPrimitive.Popup
          data-slot="search-overlay"
          aria-label={label}
          finalFocus={() => (opener?.isConnected ? opener : null)}
          {...stylex.props(modalPanel("top"), styles.panel)}
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

type SearchOverlayContentProps = Omit<SearchOverlayProps, "onOpenChange">;

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
  const active = rows.length === 0 ? 0 : Math.min(Math.max(highlight, 0), rows.length - 1);
  const activeId = rows.length === 0 ? undefined : `${baseId}-${active}`;

  useLayoutEffect(() => {
    if (!open) return;
    listRef.current?.querySelector("[aria-selected='true']")?.scrollIntoView({ block: "nearest" });
  }, [activeId, open]);

  return (
    <div {...stylex.props(styles.contents)}>
      <div {...stylex.props(styles.queryRow)}>
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
          {...stylex.props(styles.input)}
        />
        <Kbd>esc</Kbd>
      </div>
      <div
        ref={listRef}
        id={listboxId}
        role="listbox"
        aria-label={label}
        {...stylex.props(styles.list)}
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
