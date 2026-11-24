import { parseVisualDiff } from "./visualDiff";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { type ComponentContext } from "@flame/component-sdk";
import type { DiffMode } from "@flame/runtime-client/wire";

export type Query = { mode: DiffMode; path?: string };

export type Read = {
  loading: boolean;
  accepted?: ReturnType<typeof parseVisualDiff> & { query: Query; at: string };
  error?: { cause: unknown };
};

export type DiffFile = NonNullable<
  Read["accepted"]
>["diff"]["visual"]["manifest"][number];

// Paths are addressed relative to the workspace root the diff reads.
export function diffWorkspace(captured: ComponentContext["workspace"]) {
  if (!captured || !captured.focus)
    throw new Error("Workspace diff requires a workspace context");
  const prefix = captured.ref.path.endsWith("/")
    ? captured.ref.path
    : `${captured.ref.path}/`;
  const path = captured.focus.path;
  return {
    ref: captured.ref,
    focusPath: path.startsWith(prefix) ? path.slice(prefix.length) : path,
  };
}

// The reader steps through the filtered files while the open file matches the
// filter, and through all files otherwise, so filtering never strands it.
export function fileNavigation(
  files: DiffFile[],
  filter: string,
  openPath: string | undefined,
) {
  const needle = filter.toLocaleLowerCase();
  const matching = files.filter((item) =>
    `${item.path} ${item.previousPath ?? ""}`
      .toLocaleLowerCase()
      .includes(needle),
  );
  const navigationFiles = matching.some((item) => item.path === openPath)
    ? matching
    : files;
  return {
    matching,
    navigationFiles,
    index: navigationFiles.findIndex((item) => item.path === openPath),
  };
}

// One read at a time: a new query or refresh aborts the read it supersedes, and
// a failed read keeps the last accepted diff.
export function useDiffRead({
  runtime,
  signal: componentSignal,
  track,
  workspace,
  query,
  onAccepted,
  onFailed,
}: {
  runtime: ComponentContext["runtime"];
  signal: AbortSignal;
  track: ComponentContext["track"];
  workspace: ReturnType<typeof diffWorkspace>;
  query: Query;
  onAccepted(manifest: DiffFile[]): void;
  onFailed(): void;
}) {
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const callbacks = useRef({ onAccepted, onFailed });
  useLayoutEffect(() => {
    callbacks.current = { onAccepted, onFailed };
  });
  const refresh = useCallback(async () => {
    reading.current?.abort();
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([componentSignal, controller.signal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      const diff = await track(() =>
        runtime
          .workspace(workspace.ref)
          .diff.get({ ...query, format: "visual" }, signal),
      );
      if (!signal.aborted) {
        const projected = parseVisualDiff(diff);
        const at = new Date().toISOString();
        callbacks.current.onAccepted(projected.diff.visual.manifest);
        setRead({ loading: false, accepted: { ...projected, query, at } });
      }
    } catch (error) {
      if (!signal.aborted) {
        callbacks.current.onFailed();
        setRead((previous) => ({
          ...previous,
          loading: false,
          error: { cause: error },
        }));
      }
    }
  }, [runtime, componentSignal, track, query, workspace]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);
  return { read, refresh };
}
