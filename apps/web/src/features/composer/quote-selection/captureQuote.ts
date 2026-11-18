import { DIFFS_TAG_NAME } from "@pierre/diffs";
import { quotedLineOf, quotedReaderOf } from "@flame/ui";
import { quoteText } from "./quoteText";

export function captureQuote(): string | null {
  const selection = document.getSelection();
  if (!selection || selection.isCollapsed) return null;
  const shadows = [...document.querySelectorAll(DIFFS_TAG_NAME)].flatMap((element) =>
    element.shadowRoot ? [element.shadowRoot] : [],
  );
  const range = selection.getComposedRanges({ shadowRoots: shadows })[0];
  if (!range) return null;
  const reader = quotedReaderOf(range.startContainer);
  if (
    !reader?.host.isConnected ||
    reader.host !== quotedReaderOf(range.endContainer)?.host ||
    reader.host.closest("[hidden],[inert],[aria-hidden='true']")
  )
    return null;
  const text = selection.toString();
  if (!text.trim()) return null;
  if (reader.source.kind !== "file") return quoteText(reader.source, text);
  const first = quotedLineOf(range.startContainer);
  const last = quotedLineOf(range.endContainer);
  return quoteText(
    reader.source,
    text,
    first !== null && last !== null ? [Math.min(first, last), Math.max(first, last)] : null,
  );
}
