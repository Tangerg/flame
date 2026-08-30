// Hand-rolled rather than Base UI's Combobox — the documented §4 exemption. A Combobox owns
// an input and treats its VALUE as the query, but here the query is one `@token` inside
// otherwise free text. That costs the ARIA the primitive supplies, so the composer wires the
// pattern by hand off `MENTION_LISTBOX_ID` and aria-activedescendant on the textarea.

export const MENTION_LISTBOX_ID = "composer-mention-listbox";

export function mentionOptionId(index: number): string {
  return `composer-mention-option-${index}`;
}

import { useCallback, useMemo, useState } from "react";
import { useWorkspaceListFiles } from "@/plugins/builtin/workspace/public/queries";
import { fuzzyFile } from "./fuzzyFile";

const MENTION_ROWS = 8; // visible suggestions
const FETCH_LIMIT = 2000; // recursive file-list cap fed to the fuzzy matcher

interface Mention {
  query: string;
  start: number; // index of the '@'
  end: number; // caret
}

/** The `@` must START a token (string start or after whitespace), so `user@host` does not
 *  trigger. */
export function activeMention(value: string, caret: number): Mention | null {
  let i = caret - 1;
  for (; i >= 0; i--) {
    const ch = value[i]!;
    if (ch === "@") break;
    if (/\s/.test(ch)) return null;
  }
  if (i < 0 || value[i] !== "@") return null;
  const before = value[i - 1];
  if (i > 0 && before !== undefined && !/\s/.test(before)) return null;
  return { query: value.slice(i + 1, caret), start: i, end: caret };
}

interface Args {
  value: string;
  caret: number;
  cwd: string | undefined;
  apply: (text: string, caret: number) => void;
}

export interface FileMentions {
  active: boolean;
  items: string[];
  index: number;
  setIndex: (i: number) => void;
  accept: (path: string) => void;
  /** True when the picker consumed the key; the caller must then preventDefault. */
  handleKeyDown: (e: { key: string; shiftKey: boolean }) => boolean;
}

export function useFileMentions({ value, caret, cwd, apply }: Args): FileMentions {
  const [selection, setSelection] = useState<{
    candidateKey: string;
    index: number;
  } | null>(null);
  // Suppresses the popup for the ONE mention dismissed with Esc; a new `@` reopens.
  const [dismissedStart, setDismissedStart] = useState<number | null>(null);

  const mention = useMemo(() => activeMention(value, caret), [value, caret]);
  const open = mention !== null && mention.start !== dismissedStart;

  const { data: files } = useWorkspaceListFiles(
    open && cwd !== undefined ? { cwd, recursive: true, limit: FETCH_LIMIT } : undefined,
  );

  const items = useMemo(() => {
    if (!open || !mention || !files) return [];
    return fuzzyFile(
      mention.query,
      files.map((f) => f.path),
      MENTION_ROWS,
    );
  }, [open, mention, files]);

  // Selection belongs to ONE concrete candidate set: deriving the visible index from that
  // identity resets it during render, avoiding an effect and its one-frame stale selection.
  const candidateKey = [cwd, mention?.start, mention?.query, ...items].join("\0");
  const index =
    selection?.candidateKey === candidateKey && selection.index < items.length
      ? selection.index
      : 0;
  const setIndex = useCallback(
    (nextIndex: number) => {
      if (nextIndex < 0 || nextIndex >= items.length) return;
      setSelection({ candidateKey, index: nextIndex });
    },
    [candidateKey, items.length],
  );

  const active = open && items.length > 0;

  const accept = useCallback(
    (path: string) => {
      if (!mention) return;
      const insert = path + " ";
      apply(
        value.slice(0, mention.start) + insert + value.slice(mention.end),
        mention.start + insert.length,
      );
      setDismissedStart(null);
    },
    [mention, value, apply],
  );

  const handleKeyDown = useCallback(
    (e: { key: string; shiftKey: boolean }): boolean => {
      if (!active) return false;
      switch (e.key) {
        case "ArrowDown":
          setIndex((index + 1) % items.length);
          return true;
        case "ArrowUp":
          setIndex((index - 1 + items.length) % items.length);
          return true;
        case "Tab":
          accept(items[index] ?? items[0]!);
          return true;
        case "Enter":
          if (e.shiftKey) return false; // Shift+Enter still inserts a newline
          accept(items[index] ?? items[0]!);
          return true;
        case "Escape":
          if (mention) setDismissedStart(mention.start);
          return true;
        default:
          return false;
      }
    },
    [active, items, index, setIndex, accept, mention],
  );

  return { active, items, index, setIndex, accept, handleKeyDown };
}
