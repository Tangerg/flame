import { useCallback, useMemo, useState } from "react";
import {
  useWorkspaceListFiles,
  useWorkspaceFileMatches,
} from "@/features/workspace/public/queries";
import {
  formatFileReference,
  mentionCandidates,
} from "@/features/composer/application/draftContext";
import {
  type SuggestionList,
  useSuggestionIndex,
} from "@/features/composer/application/suggestions";
const MENTION_ROWS = 8;

function mentionRequest(cwd: string, nameQuery: string) {
  return { cwd, recursive: true, nameQuery, limit: MENTION_ROWS };
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

interface FileMentionRetry {
  status: "ready" | "pending";
  run: () => void;
}

export interface FileMentions extends SuggestionList<string> {
  status: FileMentionStatus;
  failure: string | undefined;
  retry: FileMentionRetry | undefined;
}

export function useFileMentions({ value, caret, cwd, apply }: Args): FileMentions {
  const [dismissedStart, setDismissedStart] = useState<number | null>(null);

  const mention = useMemo(() => activeMention(value, caret), [value, caret]);
  if (mention === null && dismissedStart !== null) setDismissedStart(null);
  const open = mention !== null && mention.start !== dismissedStart;

  const listing = useWorkspaceListFiles(
    open && cwd !== undefined ? mentionRequest(cwd, mention.query) : undefined,
  );
  const files = listing.data;

  const items = useMemo(() => {
    if (!open || !mention || !files) return [];
    return files.map((file) => file.path);
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
  const runRetry = useCallback(() => void refetch({ cancelRefetch: false }), [refetch]);
  // Settled-read history preserves Retry while refetch clears the current error.
  const retry: FileMentionRetry | undefined =
    open &&
    cwd !== undefined &&
    (listing.isError || (files === undefined && listing.isFetching && listing.isFetched))
      ? { status: listing.isFetching ? "pending" : "ready", run: runRetry }
      : undefined;

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

export function useKnownWorkspacePaths(
  cwd: string | undefined,
  value: string,
): ReadonlySet<string> {
  const paths = useMemo(
    () => [...new Set(mentionCandidates(value).map((mention) => mention.path))],
    [value],
  );
  const matches = useWorkspaceFileMatches(
    cwd === undefined ? [] : paths.map((path) => mentionRequest(cwd, path)),
  );
  return new Set(
    paths.filter((path, index) => matches[index]?.data?.some((file) => file.path === path)),
  );
}
