import * as stylex from "@stylexjs/stylex";
import { type ReactElement, type ReactNode } from "react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { isImeKey } from "@flame/ui";
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { Autocomplete } from "@base-ui/react/autocomplete";
import { Icon } from "@/ui/icons/index";
import { MODAL_SCRIM, modalPanel } from "@flame/ui/floating-surface";
import { color, space, surface } from "@flame/ui/tokens.stylex";
import { Kbd } from "@/ui/atoms/kbd";
import { floatingRow } from "@/ui/atoms/option-row";
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
  onSelect: (origin?: HTMLElement) => void;
  children: ReactNode;
}

interface SearchOverlayProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  label: string;
  placeholder: string;
  options: (query: string) => readonly SearchOption[];
  empty: ReactNode;
  feedback?: ReactElement | null;
}

interface Delivery {
  readonly completion: symbol;
  run: SearchOption["onSelect"] | undefined;
  opener: HTMLElement | null;
  popup: HTMLElement | null;
}

export function SearchOverlay({
  open,
  onOpenChange,
  label,
  placeholder,
  options,
  empty,
  feedback,
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
    run(delivery.opener ?? undefined);
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
            feedback={feedback}
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
  feedback,
  onChoose,
}: SearchOverlayContentProps) {
  const [query, setQuery] = useState("");
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
            onKeyDown={(event) => {
              if (isImeKey(event.nativeEvent)) event.preventBaseUIHandler();
            }}
            placeholder={placeholder}
            aria-label={placeholder}
            {...stylex.props(styles.input)}
          />
          <Kbd>esc</Kbd>
        </div>
        <div {...stylex.props(styles.list)}>
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
