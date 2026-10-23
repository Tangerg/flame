import * as stylex from "@stylexjs/stylex";
import { useCallback, useEffect, useEffectEvent, useRef, useState } from "react";
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
  range: Pick<Range, "startContainer" | "startOffset" | "endContainer" | "endOffset"> & {
    text: string;
  };
  keyboardReturn?: HTMLElement;
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

function readSource(
  host: HTMLElement,
  selection: Pick<Selection, "anchorNode" | "focusNode">,
): QuoteSource | null {
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
  if (!selection || selection.isCollapsed || selection.rangeCount !== 1) return null;
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
  const range = selection.getRangeAt(0);
  return {
    source,
    text,
    rect: range.getBoundingClientRect(),
    host,
    focusOrigin,
    range: {
      startContainer: range.startContainer,
      startOffset: range.startOffset,
      endContainer: range.endContainer,
      endOffset: range.endOffset,
      // Rendered selection text and DOM range text can have different separators.
      text: range.toString(),
    },
  };
}

function matchesRange(selection: Selection | null, pending: Pending): boolean {
  if (!selection || selection.rangeCount !== 1) return false;
  const range = selection.getRangeAt(0);
  const original = pending.range;
  return (
    range.startContainer === original.startContainer &&
    range.startOffset === original.startOffset &&
    range.endContainer === original.endContainer &&
    range.endOffset === original.endOffset
  );
}

function sourceRange(pending: Pending): Range | null {
  const { host, range, source } = pending;
  if (!host.isConnected || host.closest("[hidden], [inert]") || !host.getClientRects().length)
    return null;
  for (const [node, offset] of [
    [range.startContainer, range.startOffset],
    [range.endContainer, range.endOffset],
  ] as const) {
    if (
      !host.contains(node) ||
      offset > (node instanceof CharacterData ? node.length : node.childNodes.length)
    )
      return null;
  }
  const currentSource = readSource(host, {
    anchorNode: range.startContainer,
    focusNode: range.endContainer,
  });
  if (
    currentSource?.kind !== source.kind ||
    (currentSource.kind === "file" &&
      source.kind === "file" &&
      (currentSource.path !== source.path ||
        currentSource.lines?.[0] !== source.lines?.[0] ||
        currentSource.lines?.[1] !== source.lines?.[1]))
  )
    return null;
  const current = document.createRange();
  current.setStart(range.startContainer, range.startOffset);
  current.setEnd(range.endContainer, range.endOffset);
  return current.toString() === range.text ? current : null;
}

function ownsKeyboardTask(pending: Pending, button: HTMLButtonElement | null): boolean {
  const selection = window.getSelection();
  return (
    document.activeElement === button &&
    pending.keyboardReturn?.contains(pending.range.startContainer) === true &&
    pending.keyboardReturn.contains(pending.range.endContainer) &&
    sourceRange(pending) !== null &&
    selection !== null &&
    (selection.isCollapsed || matchesRange(selection, pending))
  );
}

export function SelectionQuote() {
  const t = useT();
  const [pending, setPending] = useState<Pending | null>(null);
  const popup = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const frame = useRef<number | null>(null);
  const clear = useCallback(() => {
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    setPending(null);
  }, []);
  const refreshPending = useEffectEvent((focusOrigin: Element | null) => {
    if (pending?.keyboardReturn) {
      if (!ownsKeyboardTask(pending, button.current)) clear();
      return;
    }
    setPending(readPending(focusOrigin));
  });

  useEffect(() => {
    const refresh = () => {
      if (frame.current !== null) cancelAnimationFrame(frame.current);
      const focusOrigin = document.activeElement;
      frame.current = requestAnimationFrame(() => {
        frame.current = null;
        refreshPending(focusOrigin);
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
    const keydown = (event: KeyboardEvent) => {
      if (pending.keyboardReturn && !ownsKeyboardTask(pending, button.current)) {
        clear();
        return;
      }
      if (event.defaultPrevented || isImeKey(event)) return;
      const target = event.target;
      if (!(target instanceof Node)) return;
      const tab = event.key === "Tab" && !event.altKey && !event.ctrlKey && !event.metaKey;
      if (pending.keyboardReturn && (tab || event.key === "Escape")) {
        event.preventDefault();
        event.stopPropagation();
        const selection = window.getSelection();
        if (matchesRange(selection, pending)) selection?.removeAllRanges();
        clear();
        pending.keyboardReturn.focus({ preventScroll: true });
        return;
      }
      const ownsSelection =
        readSelection()?.host === pending.host &&
        (pending.host.contains(target) || target === pending.focusOrigin);
      if (tab && !event.shiftKey && ownsSelection && button.current) {
        const current = readPending(pending.focusOrigin);
        const range = current && sourceRange(current);
        if (!current || !range) {
          clear();
          return;
        }
        const ancestor = range.commonAncestorContainer;
        const element = ancestor instanceof HTMLElement ? ancestor : ancestor.parentElement;
        const reader = element?.closest<HTMLElement>("[tabindex]");
        if (!reader || !(target instanceof Element) || !target.contains(reader)) return;
        event.preventDefault();
        event.stopPropagation();
        if (frame.current !== null) cancelAnimationFrame(frame.current);
        frame.current = null;
        setPending({ ...current, keyboardReturn: reader });
        button.current.focus({ preventScroll: true });
        return;
      }
      if (event.key !== "Escape") return;
      if (!popup.current?.contains(target) && !ownsSelection) return;
      event.preventDefault();
      event.stopPropagation();
      clear();
    };
    // Selection can retain focus outside its source; retire its action before workspace return.
    document.addEventListener("keydown", keydown, true);
    return () => document.removeEventListener("keydown", keydown, true);
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
          ref={button}
          data-focus-proxy={pending.keyboardReturn ? "" : undefined}
          variant="raised"
          size="xs"
          onMouseDown={(event) => event.preventDefault()}
          onBlur={() => {
            if (pending.keyboardReturn) clear();
          }}
          onClick={() => {
            if (
              !sourceRange(pending) ||
              (pending.keyboardReturn && !ownsKeyboardTask(pending, button.current))
            ) {
              clear();
              return;
            }
            const selection = window.getSelection();
            if (matchesRange(selection, pending)) selection?.removeAllRanges();
            clear();
            appendToComposerDraft(quoteText(pending.source, pending.text));
          }}
        >
          <Icon name="quote" size="xs" />
          {t("quote.selection")}
        </Button>
      </Popover.Content>
    </Popover.Root>
  );
}
