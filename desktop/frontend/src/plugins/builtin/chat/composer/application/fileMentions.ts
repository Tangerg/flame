export const MENTION_LISTBOX_ID = "composer-mention-listbox";

export function mentionOptionId(index: number): string {
  return `composer-mention-option-${index}`;
}

import { useCallback, useMemo, useState } from "react";
import { useWorkspaceListFiles } from "@/plugins/builtin/workspace/public/queries";
import { fuzzyFile } from "./fuzzyFile";

const MENTION_ROWS = 8;
const FETCH_LIMIT = 2000;

interface Mention {
  query: string;
  start: number;
  end: number;
}

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
  dismiss: () => void;
  handleKeyDown: (e: { key: string; shiftKey: boolean }) => boolean;
}

export function useFileMentions({ value, caret, cwd, apply }: Args): FileMentions {
  const [selection, setSelection] = useState<{
    candidateKey: string;
    index: number;
  } | null>(null);
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
      const insert = `@${path} `;
      apply(
        value.slice(0, mention.start) + insert + value.slice(mention.end),
        mention.start + insert.length,
      );
      setDismissedStart(null);
    },
    [mention, value, apply],
  );

  const dismiss = useCallback(() => {
    if (mention) setDismissedStart(mention.start);
  }, [mention]);

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
          if (e.shiftKey) return false;
          accept(items[index] ?? items[0]!);
          return true;
        case "Escape":
          dismiss();
          return true;
        default:
          return false;
      }
    },
    [active, items, index, setIndex, accept, dismiss],
  );

  return { active, items, index, setIndex, accept, dismiss, handleKeyDown };
}
