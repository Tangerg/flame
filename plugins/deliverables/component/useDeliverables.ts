import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  failureText,
  useContextState,
  type ComponentContext,
} from "@flame/component-sdk";
import { METHODS } from "../contracts/methods";
import { LIMITS } from "../contracts/limits";
import type { Asset, CommandMethod, Notice, Revision } from "./types";
import { bindDeliverableOperations } from "./operations";
import { useProjections } from "./useProjections";
import {
  COMMAND_EFFECTS,
  useCommandJournal,
  type CommandObservation,
} from "./useCommandJournal";
import { useAssetSave } from "./useAssetSave";

export interface Selection {
  id: string;
  revision: string;
}

function commandNotice(
  method: CommandMethod,
  result: unknown,
): Notice | undefined {
  const effect = COMMAND_EFFECTS[method];
  if (!effect.announced) return undefined;
  if (effect.result === "asset")
    return { kind: "command", method, asset: result as Asset };
  if (effect.result === "revision")
    return { kind: "command", method, revision: result as Revision };
  return { kind: "command", method };
}

type PageDirection = "next" | "previous";

// Paging keeps a stack of the cursors that opened each earlier page; a page
// still loading or without a further cursor does not move.
function pageCursor(
  direction: PageDirection,
  prior: { loading: boolean; value?: { next?: string } } | undefined,
  pages: ReadonlyArray<string | undefined>,
): { before?: string } | undefined {
  if (prior?.loading) return undefined;
  if (direction === "next")
    return prior?.value?.next ? { before: prior.value.next } : undefined;
  if (!pages.length) return undefined;
  const before = pages.at(-1);
  return before ? { before } : {};
}

function movedPages(
  pages: Array<string | undefined>,
  direction: PageDirection,
  openedBy: string | undefined,
): Array<string | undefined> {
  return direction === "next" ? [...pages, openedBy] : pages.slice(0, -1);
}

function sameSelection(a?: Selection, b?: Selection): boolean {
  return a?.id === b?.id && a?.revision === b?.revision;
}

export function useDeliverables(context: ComponentContext) {
  const operations = useMemo(
    () => bindDeliverableOperations(context),
    [context],
  );
  // Inside a Project the archive lists and records only that Project's documents.
  const project = context.project?.id;
  const scoped = useMemo(() => (project ? { project } : {}), [project]);
  const owner = useMemo(() => new AbortController(), [context]);
  const signal = useMemo(
    () => AbortSignal.any([context.signal, owner.signal]),
    [context.signal, owner],
  );
  const { values, query, abort } = useProjections(context, operations, signal);
  const [selection, setTarget] = useContextState<Selection>(context);
  const current = useRef(selection);
  current.current = selection;
  const [listPages, setListPages] = useState<Array<string | undefined>>([]);
  const [historyPages, setHistoryPages] = useState<Array<string | undefined>>(
    [],
  );
  const [notice, setNotice] = useContextState<Notice>(context);
  const tell = useCallback(
    (value: Notice) => {
      if (!signal.aborted) setNotice(value);
    },
    [setNotice, signal],
  );
  const fail = useCallback(
    (error: unknown) => tell({ kind: "error", text: failureText(error) }),
    [tell],
  );

  useEffect(() => () => owner.abort(), [owner]);

  const open = useCallback(
    (value: Selection) => {
      if (signal.aborted) return;
      if (sameSelection(current.current, value)) return;
      abort(METHODS.read);
      if (current.current?.id !== value.id) {
        abort(METHODS.history);
        setHistoryPages([]);
      }
      current.current = value;
      setTarget(value);
    },
    [abort, setTarget, signal],
  );
  const selectRevision = useCallback(
    (revision: Revision, captured: Selection | undefined) => {
      if (signal.aborted || !sameSelection(current.current, captured)) return;
      open({ id: revision.documentId, revision: revision.id });
    },
    [open, signal],
  );

  useEffect(() => {
    setListPages([]);
    setHistoryPages([]);
    void query(METHODS.catalog, {});
    void query(METHODS.list, { limit: LIMITS.pageSize, ...scoped });
    void query(METHODS.uploads, {});
  }, [query, scoped]);
  useEffect(() => {
    if (selection) void query(METHODS.read, selection);
  }, [query, selection?.id, selection?.revision]);
  useEffect(() => {
    if (selection)
      void query(METHODS.history, { id: selection.id, limit: LIMITS.pageSize });
  }, [query, selection?.id]);

  const refresh = useCallback(async () => {
    const captured = current.current;
    await Promise.all([
      query(METHODS.catalog, {}),
      query(METHODS.list, {
        limit: LIMITS.pageSize,
        ...scoped,
        ...(values[METHODS.list]?.input.before
          ? { before: values[METHODS.list]!.input.before }
          : {}),
      }),
      query(METHODS.uploads, {}),
      ...(captured
        ? [
            query(METHODS.read, captured),
            query(METHODS.history, {
              id: captured.id,
              limit: LIMITS.pageSize,
              ...(values[METHODS.history]?.input.id === captured.id &&
              values[METHODS.history]?.input.before
                ? { before: values[METHODS.history]!.input.before }
                : {}),
            }),
          ]
        : []),
    ]);
  }, [query, scoped, values]);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  const settle = useCallback(
    (command: CommandObservation, result: unknown) => {
      const announced = commandNotice(command.method, result);
      if (announced) tell(announced);
      // This callback belongs to the original authored action, never the current draft.
      try {
        command.settled?.(result);
      } catch (error) {
        fail(error);
      }
      const { reload } = COMMAND_EFFECTS[command.method];
      if (reload === "uploads") void query(METHODS.uploads, {});
      else if (reload === "views") void refreshRef.current();
    },
    [fail, query, tell],
  );
  const journal = useCommandJournal({
    context,
    operations,
    signal,
    scoped,
    settle,
    fail,
  });
  const assets = useAssetSave(context, operations, signal, tell);

  const page = useCallback(
    async (kind: "list" | "history", direction: PageDirection) => {
      if (kind === "list") {
        const prior = values[METHODS.list];
        const cursor = pageCursor(direction, prior, listPages);
        if (!cursor) return;
        const result = await query(METHODS.list, {
          limit: LIMITS.pageSize,
          ...scoped,
          ...cursor,
        });
        if (result)
          setListPages((pages) =>
            movedPages(pages, direction, prior?.input.before),
          );
        return;
      }
      const captured = current.current;
      const prior = values[METHODS.history];
      const cursor = captured && pageCursor(direction, prior, historyPages);
      if (!captured || !cursor) return;
      const result = await query(METHODS.history, {
        id: captured.id,
        limit: LIMITS.pageSize,
        ...cursor,
      });
      if (result && current.current?.id === captured.id)
        setHistoryPages((pages) =>
          movedPages(pages, direction, prior?.input.before),
        );
    },
    [historyPages, listPages, query, scoped, values],
  );

  return {
    context,
    signal,
    operations,
    selection,
    open,
    selectRevision,
    refresh,
    page,
    catalog: values[METHODS.catalog],
    list: values[METHODS.list],
    uploads: values[METHODS.uploads],
    read:
      values[METHODS.read]?.input.id === selection?.id &&
      values[METHODS.read]?.input.revision === selection?.revision
        ? values[METHODS.read]
        : undefined,
    history:
      values[METHODS.history]?.input.id === selection?.id
        ? values[METHODS.history]
        : undefined,
    listPrevious: listPages.length > 0,
    historyPrevious: historyPages.length > 0,
    ...journal,
    notice,
    refreshUploads: () => query(METHODS.uploads, {}),
    ...assets,
    clearSelection: (captured: Selection) => {
      if (!signal.aborted && sameSelection(current.current, captured)) {
        abort(METHODS.read);
        abort(METHODS.history);
        current.current = undefined;
        setTarget(undefined);
      }
    },
  };
}

export type DeliverablesState = ReturnType<typeof useDeliverables>;
