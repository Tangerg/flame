import * as stylex from "@stylexjs/stylex";
import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useT } from "@/lib/i18n";
import { Button, Icon } from "@/ui";
import { appendToComposerDraft } from "@/plugins/builtin/chat/composer/public/draft";
import { type QuoteSource, quoteText } from "../application/quoteText";

const styles = stylex.create({
  anchor: { position: "fixed", zIndex: "var(--layer-floating)", translate: "-100% -100%" },
});

interface Pending {
  source: QuoteSource;
  text: string;
  top: number;
  right: number;
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

function readPending(): Pending | null {
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) return null;
  const host = sourceElement(selection.anchorNode);
  if (!host || host !== sourceElement(selection.focusNode)) return null;
  const text = selection.toString();
  if (!text.trim()) return null;
  const source = readSource(host, selection);
  if (!source) return null;
  const rect = selection.getRangeAt(0).getBoundingClientRect();
  return { source, text, top: rect.top - 6, right: rect.right };
}

export function SelectionQuote() {
  const t = useT();
  const [pending, setPending] = useState<Pending | null>(null);

  useEffect(() => {
    let frame = 0;
    const refresh = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => setPending(readPending()));
    };
    const clear = () => setPending(null);
    document.addEventListener("selectionchange", refresh);
    window.addEventListener("scroll", clear, true);
    window.addEventListener("resize", clear);
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener("selectionchange", refresh);
      window.removeEventListener("scroll", clear, true);
      window.removeEventListener("resize", clear);
    };
  }, []);

  if (!pending) return null;
  return createPortal(
    <div
      data-slot="selection-quote"
      style={{ top: pending.top, left: pending.right }}
      {...stylex.props(styles.anchor)}
    >
      <Button
        variant="raised"
        size="xs"
        onMouseDown={(event) => event.preventDefault()}
        onClick={() => {
          appendToComposerDraft(quoteText(pending.source, pending.text));
          window.getSelection()?.removeAllRanges();
          setPending(null);
        }}
      >
        <Icon name="chat" size="xs" />
        {t("quote.selection")}
      </Button>
    </div>,
    document.body,
  );
}
