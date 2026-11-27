import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
} from "react";
import {
  CursorPager,
  failureText,
  useContextState,
  watchRuntimeTopics,
  type ComponentContext,
  type PageDirection,
} from "@flame/component-sdk";
import { METHODS } from "../contracts/methods";
import { LIMITS } from "../contracts/limits";
import type { Asset, CommandMethod, Notice, Revision } from "./types";
import { bindDeliverableOperations } from "./operations";
import { useProjections } from "./useProjections";
import {
  COMMAND_EFFECTS,
  useDeliverableCommands,
} from "./useDeliverableCommands";
import { useAssetSave } from "./useAssetSave";
import { DocumentNavigation, type Selection } from "./documentNavigation";
import { type MethodOutputs } from "./methods.generated";

function commandNotice(
  method: CommandMethod,
  result: unknown,
): Notice | undefined {
  const notice = COMMAND_EFFECTS[method].notice;
  if (!notice) return undefined;
  if (notice === "asset")
    return { kind: "command", method, asset: result as Asset };
  if (notice === "revision")
    return { kind: "command", method, revision: result as Revision };
  return { kind: "command", method };
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
  const archivePager = useMemo(
    () =>
      new CursorPager(signal, (before, readSignal) =>
        operations.query(
          METHODS.list,
          { limit: LIMITS.pageSize, ...scoped, ...(before ? { before } : {}) },
          readSignal,
        ),
      ),
    [signal, operations, scoped],
  );
  const list = useSyncExternalStore(
    archivePager.subscribe,
    archivePager.snapshot,
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
          abort(METHODS.threads);
          abort(METHODS.suggestions);
        },
      ),
    [signal, operations, abort],
  );
  const { selection, linkError } = useSyncExternalStore(
    navigation.subscribe,
    navigation.snapshot,
  );
  const selectedDocument = selection?.id;
  const historyPager = useMemo(
    () =>
      new CursorPager<MethodOutputs[typeof METHODS.history]>(
        signal,
        selectedDocument
          ? (before, readSignal) =>
              operations.query(
                METHODS.history,
                {
                  id: selectedDocument,
                  limit: LIMITS.pageSize,
                  ...(before ? { before } : {}),
                },
                readSignal,
              )
          : undefined,
      ),
    [signal, operations, selectedDocument],
  );
  const history = useSyncExternalStore(
    historyPager.subscribe,
    historyPager.snapshot,
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
    void query(METHODS.catalog, {});
    void archivePager.refresh();
    void query(METHODS.uploads, {});
    return archivePager.stop;
  }, [query, archivePager]);
  useEffect(() => {
    if (selection) {
      const { id, revision } = selection;
      void query(METHODS.read, { id, revision });
    }
  }, [query, selection?.id, selection?.revision]);
  useEffect(() => {
    if (!selectedDocument) return;
    void historyPager.refresh();
    void query(METHODS.threads, { id: selectedDocument, limit: LIMITS.pageSize });
    void query(METHODS.suggestions, {
      id: selectedDocument,
      status: "pending",
      limit: LIMITS.pageSize,
    });
    return historyPager.stop;
  }, [query, selectedDocument, historyPager]);

  const refresh = useCallback(async () => {
    const captured = navigation.snapshot().selection;
    await Promise.all([
      query(METHODS.catalog, {}),
      archivePager.refresh(),
      query(METHODS.uploads, {}),
      ...(captured
        ? [
            query(METHODS.read, { id: captured.id, revision: captured.revision }),
            ...(captured.id === selectedDocument ? [historyPager.refresh()] : []),
            query(METHODS.threads, { id: captured.id, limit: LIMITS.pageSize }),
            query(METHODS.suggestions, {
              id: captured.id,
              status: "pending",
              limit: LIMITS.pageSize,
            }),
          ]
        : []),
    ]);
  }, [navigation, query, archivePager, historyPager, selectedDocument]);
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
    (method: CommandMethod, result: unknown, followUp?: () => void) => {
      const announced = commandNotice(method, result);
      if (announced) tell(announced);
      try {
        followUp?.();
      } catch (error) {
        fail(error);
      }
      const { reload } = COMMAND_EFFECTS[method];
      if (reload === "uploads") void query(METHODS.uploads, {});
      else if (reload === "views") void refreshRef.current();
    },
    [fail, query, tell],
  );
  const journal = useDeliverableCommands({
    operations,
    signal,
    scoped,
    settle,
    fail,
  });
  const assets = useAssetSave(context, operations, signal, tell);

  return {
    context,
    signal,
    operations,
    selection,
    linkError,
    open: navigation.open,
    selectRevision,
    refresh,
    page: (kind: "list" | "history", direction: PageDirection) =>
      (kind === "list" ? archivePager : historyPager).move(direction),
    catalog: values[METHODS.catalog],
    list,
    uploads: values[METHODS.uploads],
    read:
      values[METHODS.read]?.input.id === selection?.id &&
      values[METHODS.read]?.input.revision === selection?.revision
        ? values[METHODS.read]
        : undefined,
    history,
    threads:
      values[METHODS.threads]?.input.id === selection?.id
        ? values[METHODS.threads]
        : undefined,
    suggestions:
      values[METHODS.suggestions]?.input.id === selection?.id
        ? values[METHODS.suggestions]
        : undefined,
    listPrevious: Boolean(list?.cursors.length),
    historyPrevious: Boolean(history?.cursors.length),
    ...journal,
    notice,
    refreshUploads: () => query(METHODS.uploads, {}),
    ...assets,
    clearSelection: navigation.clear,
  };
}

export type DeliverablesState = ReturnType<typeof useDeliverables>;
