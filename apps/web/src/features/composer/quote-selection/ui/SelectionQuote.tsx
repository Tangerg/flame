import * as stylex from "@stylexjs/stylex";
import { useCallback, useEffect, useRef, useState } from "react";
import { useT } from "@/lib/i18n/index";
import { Icon, Popover } from "@/ui/index";
import { Button, isImeKey } from "@flame/ui";
import { appendToComposerDraft } from "@/features/composer/public/draft";
import {
  type QuoteSource,
  quoteText,
} from "@/features/composer/quote-selection/application/quoteText";
const styles = stylex.create({
  content: { display: "flex", width: "max-content", maxWidth: "var(--available-width)" },
});

interface Pending {
  source: QuoteSource;
  text: string;
  rect: DOMRect;
  host: HTMLElement;
  focusOrigin: Element | null;
}

function sourceElement(node: Node | null): HTMLElement | null {
  const element = node instanceof HTMLElement ? node : (node?.parentElement ?? null);
  return element?.closest<HTMLElement>("[data-quote-source]") ?? null;
}

function lineOf(node: Node | null): number | null {
  const element = node instanceof HTMLElement ? node : (node?.parentElement ?? null);
  const value = element?.closest<HTMLElement>("[data-quote-line]")?.dataset.quoteLine;
  return value ? Number(value) : null;
}

function readSource(host: HTMLElement, selection: Selection): QuoteSource | null {
  switch (host.dataset.quoteSource) {
    case "file": {
      const a = lineOf(selection.anchorNode);
      const b = lineOf(selection.focusNode);
      const lines = a !== null && b !== null ? ([Math.min(a, b), Math.max(a, b)] as const) : null;
      return { kind: "file", path: host.dataset.quotePath ?? "", lines };
    }
    case "tool-output":
      return { kind: "tool-output" };
    case "message":
      return { kind: "message" };
    default:
      return null;
  }
}

function readSelection() {
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) return null;
  const host = sourceElement(selection.anchorNode);
  if (!host?.isConnected || host !== sourceElement(selection.focusNode)) return null;
  const text = selection.toString();
  if (!text.trim()) return null;
  return { selection, host, text };
}

function readPending(focusOrigin: Element | null): Pending | null {
  const selected = readSelection();
  if (!selected) return null;
  const { selection, host, text } = selected;
  const source = readSource(host, selection);
  if (!source) return null;
  const rect = selection.getRangeAt(0).getBoundingClientRect();
  return { source, text, rect, host, focusOrigin };
}

export function SelectionQuote() {
  const t = useT();
  const [pending, setPending] = useState<Pending | null>(null);
  const popup = useRef<HTMLDivElement>(null);
  const frame = useRef<number | null>(null);
  const clear = useCallback(() => {
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    setPending(null);
  }, []);

  useEffect(() => {
    const refresh = () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
      const focusOrigin = document.activeElement;
      frame.current = requestAnimationFrame(() => {
        frame.current = null;
        setPending(readPending(focusOrigin));
      });
    };
    document.addEventListener("selectionchange", refresh);
    window.addEventListener("scroll", clear, true);
    window.addEventListener("resize", clear);
    return () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
      frame.current = null;
      document.removeEventListener("selectionchange", refresh);
      window.removeEventListener("scroll", clear, true);
      window.removeEventListener("resize", clear);
    };
  }, [clear]);

  useEffect(() => {
    if (!pending) return;
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented || isImeKey(event)) return;
      const target = event.target;
      if (!(target instanceof Node)) return;
      const ownsSelection =
        readSelection()?.host === pending.host &&
        (pending.host.contains(target) || target === pending.focusOrigin);
      if (!popup.current?.contains(target) && !ownsSelection) return;
      event.preventDefault();
      event.stopPropagation();
      clear();
    };
    // Selection can retain focus outside its source; retire its action before workspace return.
    document.addEventListener("keydown", escape, true);
    return () => document.removeEventListener("keydown", escape, true);
  }, [clear, pending]);

  if (!pending) return null;
  return (
    <Popover.Root
      open
      modal={false}
      onOpenChange={(open, details) => {
        if (details.reason === "escape-key") {
          details.cancel();
          details.allowPropagation();
          return;
        }
        if (
          details.reason === "outside-press" &&
          details.event.target instanceof Node &&
          sourceElement(details.event.target) === pending.host &&
          readSelection()?.host === pending.host
        ) {
          // A source click may finish extending the native selection while its action is open.
          details.cancel();
          return;
        }
        if (!open) clear();
      }}
    >
      <Popover.Content
        ref={popup}
        data-slot="selection-quote"
        anchor={{ getBoundingClientRect: () => pending.rect }}
        positionMethod="fixed"
        side="top"
        align="end"
        sideOffset={6}
        collisionPadding={8}
        surface="none"
        role="group"
        initialFocus={false}
        finalFocus={false}
        {...stylex.props(styles.content)}
      >
        <Button
          variant="raised"
          size="xs"
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => {
            appendToComposerDraft(quoteText(pending.source, pending.text));
            window.getSelection()?.removeAllRanges();
            clear();
          }}
        >
          <Icon name="quote" size="xs" />
          {t("quote.selection")}
        </Button>
      </Popover.Content>
    </Popover.Root>
  );
}
