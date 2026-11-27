import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  failureText,
  useContextState,
  watchRuntimeTopics,
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
import { DocumentNavigation, type Selection } from "./documentNavigation";

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
  const [listPages, setListPages] = useState<Array<string | undefined>>([]);
  const [historyPages, setHistoryPages] = useState<Array<string | undefined>>(
    [],
  );
  const navigation = useMemo(
    () =>
      new DocumentNavigation(
        signal,
        async (id, signal) => {
          const history = await operations.query(
            METHODS.history,
            { id, limit: 1 },
            signal,
          );
          const head = history.entries[0];
          if (!head) throw new Error("This deliverable has no revisions.");
          return head.id;
        },
        (previous, next) => {
          abort(METHODS.read);
          if (previous?.id === next?.id) return;
          abort(METHODS.history);
          abort(METHODS.threads);
          abort(METHODS.suggestions);
          setHistoryPages([]);
        },
      ),
    [signal, operations, abort],
  );
  const { selection, linkError } = useSyncExternalStore(
    navigation.subscribe,
    navigation.snapshot,
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
  useEffect(
    () =>
      context.views?.subscribeTarget((value) => void navigation.openLink(value)),
    [context.views, navigation],
  );
  const selectRevision = useCallback(
    (revision: Revision, captured: Selection | undefined) => {
      navigation.replace(captured, {
        id: revision.documentId,
        revision: revision.id,
      });
    },
    [navigation],
  );

  useEffect(() => {
    setListPages([]);
    setHistoryPages([]);
    void query(METHODS.catalog, {});
    void query(METHODS.list, { limit: LIMITS.pageSize, ...scoped });
    void query(METHODS.uploads, {});
  }, [query, scoped]);
  useEffect(() => {
    if (selection) {
      const { id, revision } = selection;
      void query(METHODS.read, { id, revision });
    }
  }, [query, selection?.id, selection?.revision]);
  useEffect(() => {
    if (!selection) return;
    void query(METHODS.history, { id: selection.id, limit: LIMITS.pageSize });
    void query(METHODS.threads, { id: selection.id, limit: LIMITS.pageSize });
    void query(METHODS.suggestions, {
      id: selection.id,
      status: "pending",
      limit: LIMITS.pageSize,
    });
  }, [query, selection?.id]);

  const refresh = useCallback(async () => {
    const captured = navigation.snapshot().selection;
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
            query(METHODS.read, { id: captured.id, revision: captured.revision }),
            query(METHODS.history, {
              id: captured.id,
              limit: LIMITS.pageSize,
              ...(values[METHODS.history]?.input.id === captured.id &&
              values[METHODS.history]?.input.before
                ? { before: values[METHODS.history]!.input.before }
                : {}),
            }),
            query(METHODS.threads, { id: captured.id, limit: LIMITS.pageSize }),
            query(METHODS.suggestions, {
              id: captured.id,
              status: "pending",
              limit: LIMITS.pageSize,
            }),
          ]
        : []),
    ]);
  }, [navigation, query, scoped, values]);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  // Runtime announces plugin data changes, including the model's tool calls,
  // without their content, so the open views read again.
  useEffect(() => {
    const controller = new AbortController();
    const lifetime = AbortSignal.any([signal, controller.signal]);
    void context.track(() =>
      watchRuntimeTopics(
        context.runtime,
        { topics: ["plugins.dataChanged"] },
        lifetime,
        () => void refreshRef.current(),
      ).catch((error: unknown) => {
        if (!lifetime.aborted) fail(error);
      }),
    );
    return () => controller.abort();
  }, [context, signal, fail]);

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
      const captured = navigation.snapshot().selection;
      const prior = values[METHODS.history];
      const cursor = captured && pageCursor(direction, prior, historyPages);
      if (!captured || !cursor) return;
      const result = await query(METHODS.history, {
        id: captured.id,
        limit: LIMITS.pageSize,
        ...cursor,
      });
      if (result && navigation.snapshot().selection?.id === captured.id)
        setHistoryPages((pages) =>
          movedPages(pages, direction, prior?.input.before),
        );
    },
    [historyPages, listPages, navigation, query, scoped, values],
  );

  return {
    context,
    signal,
    operations,
    selection,
    linkError,
    open: navigation.open,
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
    threads:
      values[METHODS.threads]?.input.id === selection?.id
        ? values[METHODS.threads]
        : undefined,
    suggestions:
      values[METHODS.suggestions]?.input.id === selection?.id
        ? values[METHODS.suggestions]
        : undefined,
    listPrevious: listPages.length > 0,
    historyPrevious: historyPages.length > 0,
    ...journal,
    notice,
    refreshUploads: () => query(METHODS.uploads, {}),
    ...assets,
    clearSelection: navigation.clear,
  };
}

export type DeliverablesState = ReturnType<typeof useDeliverables>;
