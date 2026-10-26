import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { isImeKey } from "@flame/ui";
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { Icon } from "@/ui/icons/index";
import { MODAL_SCRIM, modalPanel } from "@flame/ui/floating-surface";
import { color, space, surface } from "@flame/ui/tokens.stylex";
import { Kbd } from "@/ui/atoms/kbd";
import { OptionRow } from "@/ui/atoms/option-row";
import { TextField } from "@flame/ui/text-field";
const styles = stylex.create({
  panel: {
    display: "flex",
    width: "min(520px, calc(100dvw - var(--layout-compact-gutter) * 2))",
    flexDirection: "column",
    overflow: "hidden",
  },
  contents: { display: "contents" },
  queryRow: {
    display: "flex",
    alignItems: "center",
    gap: space.s2_5,
    borderBottomWidth: "var(--control-edge-width)",
    borderBottomStyle: "solid",
    borderBottomColor: surface.lineSoft,
    paddingInline: "calc(var(--spacing) * 3.5)",
    paddingBlock: space.s2_5,
    color: color.fgMuted,
    flexShrink: 0,
  },
  input: { flex: 1 },
  list: {
    minHeight: 0,
    maxHeight: "calc(var(--spacing) * 80)",
    overflowY: "auto",
    padding: space.s1_5,
    scrollPaddingBlock: space.s1_5,
  },
});

interface SearchOption {
  key: string;
  /** Runs after dismissal so the action can focus outside the modal. */
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

interface Delivery {
  readonly completion: symbol;
  run: (() => void) | undefined;
  opener: HTMLElement | null;
  popup: HTMLElement | null;
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
  const [wasOpen, setWasOpen] = useState(open);
  const [opener, setOpener] = useState<HTMLElement | null>(() =>
    document.activeElement instanceof HTMLElement ? document.activeElement : null,
  );
  const choice = useRef<Delivery | null>(null);
  const popupRef = useRef<HTMLDivElement>(null);
  const [completion, setCompletion] = useState<symbol | null>(null);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setOpener(document.activeElement instanceof HTMLElement ? document.activeElement : null);
    }
  }

  useLayoutEffect(() => {
    if (open) choice.current = null;
  }, [open]);

  useEffect(() => {
    const delivery = choice.current;
    if (open || !delivery || delivery.completion !== completion || !delivery.run) return;
    const run = delivery.run;
    // Keep the intent until reopening: Base UI may still settle its return focus.
    delivery.run = undefined;
    const doc = delivery.popup?.ownerDocument ?? document;
    const active = doc.activeElement;
    if ((active === doc.body || delivery.popup?.contains(active)) && delivery.opener?.isConnected) {
      delivery.opener.focus({ preventScroll: true });
    }
    run();
  }, [completion, open]);

  return (
    <DialogPrimitive.Root
      open={open}
      onOpenChange={onOpenChange}
      onOpenChangeComplete={(nextOpen) => {
        // The next committed effect runs after the modal's isolation cleanup.
        if (!nextOpen && choice.current) setCompletion(choice.current.completion);
      }}
    >
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className={stylex.props(MODAL_SCRIM).className} />
        <DialogPrimitive.Popup
          ref={popupRef}
          aria-label={label}
          finalFocus={() => {
            if (choice.current) return false;
            const active = document.activeElement;
            if (
              active !== document.body &&
              active !== opener &&
              !popupRef.current?.contains(active)
            )
              return false;
            return opener?.isConnected ? opener : null;
          }}
          {...stylex.props(modalPanel("top"), styles.panel)}
        >
          <SearchOverlayContent
            key={open ? "open" : "closed"}
            open={open}
            label={label}
            placeholder={placeholder}
            options={options}
            empty={empty}
            onChoose={(option) => {
              if (!open || choice.current) return;
              choice.current = {
                completion: Symbol(),
                run: option.onSelect,
                opener,
                popup: popupRef.current,
              };
              onOpenChange(false);
            }}
          />
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

type SearchOverlayContentProps = Omit<SearchOverlayProps, "onOpenChange"> & {
  onChoose: (option: SearchOption) => void;
};

function SearchOverlayContent({
  open,
  label,
  placeholder,
  options,
  empty,
  onChoose,
}: SearchOverlayContentProps) {
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const baseId = useId();
  const listboxId = `${baseId}-list`;

  const rows = options(query);
  const active = rows.length === 0 ? 0 : Math.min(Math.max(highlight, 0), rows.length - 1);
  const activeId = rows.length === 0 ? undefined : `${baseId}-${active}`;
  const activeKey = rows[active]?.key;

  useLayoutEffect(() => {
    if (!open) return;
    listRef.current?.querySelector("[aria-selected='true']")?.scrollIntoView({ block: "nearest" });
  }, [activeId, activeKey, open, query]);

  return (
    <div {...stylex.props(styles.contents)}>
      <div {...stylex.props(styles.queryRow)}>
        <Icon name="search" size="md" />
        <TextField
          variant="bare"
          // oxlint-disable-next-line jsx-a11y/no-autofocus
          autoFocus={open}
          role="combobox"
          aria-expanded
          aria-controls={listboxId}
          aria-activedescendant={activeId}
          value={query}
          onKeyDown={(event) => {
            if (isImeKey(event.nativeEvent)) return;
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
              event.preventDefault();
              setHighlight(wrap(active, rows.length, event.key === "ArrowDown" ? 1 : -1));
              return;
            }
            if (event.key === "Enter") {
              event.preventDefault();
              const option = rows[active];
              if (option) onChoose(option);
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
                onClick={() => onChoose(option)}
              >
                {option.children}
              </OptionRow>
            ))}
      </div>
    </div>
  );
}
