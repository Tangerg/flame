import { DIFFS_TAG_NAME } from "@pierre/diffs";
import { quotedLineOf, quotedReaderOf, isConcealed } from "@flame/ui";
import type { ComposerAccess } from "./composer";
import { type ComposerReference } from "@/features/composer/domain/draft";
import { quoteReference } from "../quote-selection/quoteReference";
import { fileContentRows } from "@/lib/highlight/pierreMarkup";

export function captureComposerSelectionQuote(
  composer: ComposerAccess,
  trigger?: Node,
): (() => boolean) | null {
  const reference = captureQuote(trigger);
  if (!reference) return null;
  const add = composer.captureAddReference();
  return () => add(reference);
}

function captureQuote(trigger?: Node): ComposerReference | null {
  const selection = document.getSelection();
  if (!selection) return null;
  // WebKit can report a collapsed document selection for selected shadow text.
  const text = selection.toString();
  if (!text.trim()) return null;
  const shadows = [...document.querySelectorAll(DIFFS_TAG_NAME)].flatMap((element) =>
    element.shadowRoot ? [element.shadowRoot] : [],
  );
  const range = selection.getComposedRanges({ shadowRoots: shadows })[0];
  if (!range || range.collapsed) return null;
  const reader = quotedReaderOf(range.startContainer);
  if (
    !reader?.host.isConnected ||
    reader.host !== quotedReaderOf(range.endContainer)?.host ||
    (trigger && reader.host !== quotedReaderOf(trigger)?.host) ||
    isConcealed(reader.host)
  )
    return null;
  if (reader.source.kind !== "file") return quoteReference(reader.source, text);
  const first = quotedLineOf(range.startContainer);
  const last = quotedLineOf(range.endContainer);
  return quoteReference(
    reader.source,
    text,
    first !== null && last !== null
      ? [Math.min(first, last), Math.max(first, last)]
      : wholeSourceLineRange(range),
  );
}

function wholeSourceLineRange(range: StaticRange): [number, number] | null {
  const { startContainer: content, endContainer, startOffset, endOffset } = range;
  if (
    content !== endContainer ||
    startOffset !== 0 ||
    endOffset !== content.childNodes.length ||
    !(content instanceof HTMLElement)
  )
    return null;
  const rows = fileContentRows(content);
  const root = content.getRootNode();
  if (!rows || !(root instanceof ShadowRoot) || root.host.localName !== DIFFS_TAG_NAME) return null;

  // Select All addresses the source column; its marked rows carry the served bounds.
  let first: number | null = null;
  let last: number | null = null;
  for (const row of rows) {
    const line = quotedLineOf(row);
    if (line === null) continue;
    first ??= line;
    last = line;
  }
  return first !== null && last !== null ? [first, last] : null;
}
