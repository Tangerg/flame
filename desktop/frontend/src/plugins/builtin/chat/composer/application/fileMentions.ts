import { useCallback, useMemo, useState } from "react";
import { useWorkspaceListFiles } from "@/plugins/builtin/workspace/public/queries";
import { fuzzyFile } from "./fuzzyFile";
import { formatFileReference } from "./draftContext";
import { type SuggestionList, useSuggestionIndex } from "./suggestions";

const MENTION_ROWS = 8;
const LISTING_PAGE_SIZE = 2000;

function workspaceListing(cwd: string) {
  return { cwd, recursive: true, limit: LISTING_PAGE_SIZE };
}

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

type FileMentionStatus = "no-workspace" | "loading" | "error" | "empty" | "ready";

export interface FileMentions extends SuggestionList<string> {
  status: FileMentionStatus;
  failure: string | undefined;
  retry: () => void;
}

export function useFileMentions({ value, caret, cwd, apply }: Args): FileMentions {
  const [dismissedStart, setDismissedStart] = useState<number | null>(null);

  const mention = useMemo(() => activeMention(value, caret), [value, caret]);
  const open = mention !== null && mention.start !== dismissedStart;

  const listing = useWorkspaceListFiles(
    open && cwd !== undefined ? workspaceListing(cwd) : undefined,
  );
  const files = listing.data;

  const items = useMemo(() => {
    if (!open || !mention || !files) return [];
    return fuzzyFile(
      mention.query,
      files.map((f) => f.path),
      MENTION_ROWS,
    );
  }, [open, mention, files]);

  const status: FileMentionStatus =
    cwd === undefined
      ? "no-workspace"
      : listing.isError
        ? "error"
        : files === undefined
          ? "loading"
          : items.length === 0
            ? "empty"
            : "ready";

  const candidateKey = [cwd, mention?.start, mention?.query, ...items].join("\0");
  const { index, setIndex } = useSuggestionIndex(candidateKey, items.length);

  const accept = useCallback(
    (path: string) => {
      if (!mention) return;
      const insert = `${formatFileReference(path)} `;
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

  const { refetch } = listing;
  const retry = useCallback(() => void refetch(), [refetch]);

  return {
    open,
    items,
    index,
    setIndex,
    accept,
    dismiss,
    status,
    failure: listing.isError ? listing.error?.message : undefined,
    retry,
  };
}

const NO_PATHS: ReadonlySet<string> = new Set();

export function useKnownWorkspacePaths(
  cwd: string | undefined,
  enabled: boolean,
): ReadonlySet<string> {
  const { data } = useWorkspaceListFiles(
    enabled && cwd !== undefined ? workspaceListing(cwd) : undefined,
  );
  return useMemo(() => (data ? new Set(data.map((file) => file.path)) : NO_PATHS), [data]);
}
