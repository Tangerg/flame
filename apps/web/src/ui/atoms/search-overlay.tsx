import * as stylex from "@stylexjs/stylex";
import { type ReactElement, type ReactNode } from "react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { isConcealed } from "@flame/ui";
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { Autocomplete } from "@base-ui/react/autocomplete";
import { Icon } from "@/ui/icons/index";
import { MODAL_SCRIM, modalPanel } from "@flame/ui/floating-surface";
import { color, edge, space, surface } from "@flame/ui/tokens.stylex";
import { floatingRow } from "@/ui/atoms/option-row";
import { guardComboboxEnter } from "@/ui/atoms/combobox-enter";
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
    borderBottomWidth: edge.control,
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
  onSelect: (origin?: HTMLElement) => void;
  children: ReactNode;
}

interface SearchOverlayProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  captureFocusReturn?: (origin: HTMLElement) => (() => void) | null;
  label: string;
  placeholder: string;
  options: (query: string) => readonly SearchOption[];
  empty: ReactNode;
  feedback?: ReactElement | null;
}

interface Opening {
  readonly completion: symbol;
  readonly origin: HTMLElement | null;
  readonly restore: (() => void) | null;
}

interface Delivery {
  readonly completion: symbol;
  deliver: (() => void) | undefined;
}

function captureOpening(captureFocusReturn: SearchOverlayProps["captureFocusReturn"]): Opening {
  const active = document.activeElement;
  const origin = active instanceof HTMLElement ? active : null;
  return {
    completion: Symbol(),
    origin,
    restore: origin ? (captureFocusReturn?.(origin) ?? null) : null,
  };
}

export function SearchOverlay({
  open,
  onOpenChange,
  captureFocusReturn,
  label,
  placeholder,
  options,
  empty,
  feedback,
}: SearchOverlayProps) {
  const [wasOpen, setWasOpen] = useState(open);
  const [opening, setOpening] = useState(() => (open ? captureOpening(captureFocusReturn) : null));
  const pending = useRef<Delivery | null>(null);
  const popupRef = useRef<HTMLDivElement>(null);
  const [completion, setCompletion] = useState<symbol | null>(null);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setOpening(captureOpening(captureFocusReturn));
  }

  useLayoutEffect(() => {
    if (open) pending.current = null;
  }, [open]);

  useEffect(() => {
    const delivery = pending.current;
    if (
      !delivery?.deliver ||
      open ||
      delivery.completion !== completion ||
      completion !== opening?.completion
    )
      return;
    const deliver = delivery.deliver;
    // Keep the intent until reopening: Base UI may still settle its return focus.
    delivery.deliver = undefined;
    deliver();
  }, [completion, open, opening]);

  return (
    <DialogPrimitive.Root
      open={open}
      onOpenChange={onOpenChange}
      onOpenChangeComplete={(nextOpen) => {
        if (nextOpen || open || !opening) return;
        const { origin, restore } = opening;
        if (!pending.current && origin && restore) {
          const popup = popupRef.current;
          pending.current = {
            completion: opening.completion,
            deliver: () => {
              const doc = origin.ownerDocument;
              const active = doc.activeElement;
              if (
                (active === doc.body || active === origin || popup?.contains(active)) &&
                origin.isConnected &&
                !isConcealed(origin)
              )
                restore();
            },
          };
        }
        // The next committed effect runs after the modal's isolation cleanup.
        if (pending.current?.completion === opening.completion) setCompletion(opening.completion);
      }}
    >
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className={stylex.props(MODAL_SCRIM).className} />
        <DialogPrimitive.Popup
          ref={popupRef}
          aria-label={label}
          finalFocus={() => {
            if (pending.current || opening?.restore) return false;
            const opener = opening?.origin;
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
            feedback={feedback}
            onChoose={(option) => {
              if (!open || !opening || pending.current) return;
              const run = option.onSelect;
              const { origin: opener, restore } = opening;
              const popup = popupRef.current;
              pending.current = {
                completion: opening.completion,
                deliver: () => {
                  const doc = popup?.ownerDocument ?? document;
                  const active = doc.activeElement;
                  if (
                    (active === doc.body || popup?.contains(active)) &&
                    opener?.isConnected &&
                    !isConcealed(opener)
                  ) {
                    if (restore) restore();
                    else opener.focus({ preventScroll: true });
                  }
                  run(opener ?? undefined);
                },
              };
              onOpenChange(false);
            }}
          />
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

type SearchOverlayContentProps = Omit<SearchOverlayProps, "onOpenChange" | "captureFocusReturn"> & {
  onChoose: (option: SearchOption) => void;
};

function SearchOverlayContent({
  open,
  label,
  placeholder,
  options,
  empty,
  feedback,
  onChoose,
}: SearchOverlayContentProps) {
  const [query, setQuery] = useState("");
  const viewportRef = useRef<HTMLDivElement>(null);
  const rows = feedback == null ? options(query) : [];

  return (
    <Autocomplete.Root
      inline={feedback == null}
      open={open && feedback == null}
      mode="none"
      items={rows}
      autoHighlight="always"
      loopFocus
      value={query}
      onValueChange={setQuery}
      itemToStringValue={(option) => option.key}
    >
      <div {...stylex.props(styles.contents)}>
        <div {...stylex.props(styles.queryRow)}>
          <Icon name="search" size="md" />
          <Autocomplete.Input
            render={<TextField variant="bare" />}
            data-chrome-focus=""
            autoFocus={open}
            onKeyDown={(event) =>
              guardComboboxEnter(event, open && rows.length > 0 ? viewportRef.current : null)
            }
            placeholder={placeholder}
            aria-label={placeholder}
            {...stylex.props(styles.input)}
          />
        </div>
        <div ref={viewportRef} {...stylex.props(styles.list)}>
          <Autocomplete.List aria-label={label} hidden={feedback != null}>
            {(option: SearchOption, index: number) => (
              <Autocomplete.Item
                key={option.key}
                value={option}
                index={index}
                {...stylex.props(floatingRow("flex", "lg"))}
                onClick={() => onChoose(option)}
              >
                {option.children}
              </Autocomplete.Item>
            )}
          </Autocomplete.List>
          {feedback == null && rows.length === 0 && empty}
          {feedback}
        </div>
      </div>
    </Autocomplete.Root>
  );
}
