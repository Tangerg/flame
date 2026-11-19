import { DIFFS_TAG_NAME } from "@pierre/diffs";
import { quotedLineOf, quotedReaderOf } from "@flame/ui";
import type { ComposerAccess } from "./composer";
import { quoteText } from "../quote-selection/quoteText";

export function captureComposerSelectionQuote(
  composer: ComposerAccess,
  trigger?: Node,
): (() => boolean) | null {
  const text = captureQuote(trigger);
  if (!text) return null;
  const append = composer.captureAppendText();
  return () => append(text);
}

function captureQuote(trigger?: Node): string | null {
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
    reader.host.closest("[hidden],[inert],[aria-hidden='true']")
  )
    return null;
  if (reader.source.kind !== "file") return quoteText(reader.source, text);
  const first = quotedLineOf(range.startContainer);
  const last = quotedLineOf(range.endContainer);
  return quoteText(
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
    !(content instanceof HTMLElement) ||
    !content.matches("[data-file] > [data-code] > [data-content]")
  )
    return null;
  const root = content.getRootNode();
  if (!(root instanceof ShadowRoot) || root.host.localName !== DIFFS_TAG_NAME) return null;

  // Select All addresses the source column; its marked rows carry the served bounds.
  let first: number | null = null;
  let last: number | null = null;
  for (const row of content.querySelectorAll(":scope > [data-line]")) {
    const line = quotedLineOf(row);
    if (line === null) continue;
    first ??= line;
    last = line;
  }
  return first !== null && last !== null ? [first, last] : null;
}
