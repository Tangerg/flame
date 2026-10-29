import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  bindOperations,
  failureText,
  isErrorType,
  mutationSettlementIsUnknown,
  OperationFailure,
  type ComponentContext,
} from "@flame/component-sdk";
import { stringifyJSON } from "@qimu/sdk/json";
import {
  CAPABILITY_JSON,
  METHODS as DECLARATIONS,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";
import { METHODS } from "../contracts/methods";
import { LIMITS } from "../contracts/limits";
import type { Asset, Revision } from "./types";
import { downloadAsset } from "./transfer";

export type CommandMethod = {
  [
    K in keyof typeof DECLARATIONS
  ]: (typeof DECLARATIONS)[K]["kind"] extends "command" ? K : never;
}[keyof typeof DECLARATIONS];
const commandMethods = (
  Object.keys(DECLARATIONS) as Array<keyof typeof DECLARATIONS>
).filter(
  (method): method is CommandMethod => DECLARATIONS[method].kind === "command",
);
type ViewMethod =
  | typeof METHODS.catalog
  | typeof METHODS.list
  | typeof METHODS.read
  | typeof METHODS.history
  | typeof METHODS.uploads;
type Projection<K extends ViewMethod> = {
  input: MethodInputs[K];
  value?: MethodOutputs[K];
  loading: boolean;
  error?: string;
};
type Projections = { [K in ViewMethod]?: Projection<K> };
export interface Selection {
  id: string;
  revision: string;
}
export interface CommandObservation {
  id: string;
  method: CommandMethod;
  status: "running" | "unknown";
  error?: string;
  recover?: () => Promise<unknown>;
  retire?: () => Promise<void>;
  settled?: (result: unknown) => void;
}
export type Notice =
  | {
      kind: "command";
      method: CommandMethod;
      asset?: Asset;
      revision?: Revision;
    }
  | { kind: "error"; text: string }
  | {
      kind: "save";
      result: "requested" | "saved" | "cancelled";
      filename: string;
    };

function sameSelection(a?: Selection, b?: Selection): boolean {
  return a?.id === b?.id && a?.revision === b?.revision;
}

export function useDeliverables(context: ComponentContext) {
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error(
        "deliverables component requires its callable contribution",
      );
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      {
        scope: "installation",
        reference: context.binding.installationId,
      },
    );
  }, [context]);
  const owner = useMemo(() => new AbortController(), [context]);
  const signal = useMemo(
    () => AbortSignal.any([context.signal, owner.signal]),
    [context.signal, owner],
  );
  const requests = useMemo(
    () => new Map<ViewMethod, AbortController>(),
    [context],
  );
  const [projections, setProjections] = useState<{
    context: ComponentContext;
    values: Projections;
  }>();
  const [target, setTarget] = useState<{
    context: ComponentContext;
    value: Selection;
  }>();
  const selection = target?.context === context ? target.value : undefined;
  const current = useRef(selection);
  current.current = selection;
  const [listPages, setListPages] = useState<Array<string | undefined>>([]);
  const [historyPages, setHistoryPages] = useState<Array<string | undefined>>(
    [],
  );
  const [noticeState, setNotice] = useState<{
    context: ComponentContext;
    value: Notice;
  }>();
  const [commandState, setCommandState] = useState<{
    context: ComponentContext;
    values: CommandObservation[];
  }>();
  const [transfer, setTransfer] = useState<{
    context: ComponentContext;
    filename: string;
    received: number;
    total: number;
  }>();
  const saving = useMemo(
    () => ({
      active: false,
      controller: undefined as AbortController | undefined,
    }),
    [context],
  );
  const commands = useMemo(() => {
    const entries = new Map<string, CommandObservation>();
    for (const method of commandMethods)
      for (const original of operations.pendingCommands(method)) {
        entries.set(original.idempotencyKey, {
          id: original.idempotencyKey,
          method,
          status: "unknown",
          recover: () => original.recover(signal),
          retire: () => original.retire(signal),
        });
      }
    return entries;
  }, [operations, signal]);
  const values = projections?.context === context ? projections.values : {};
  const publish = useCallback(() => {
    if (!signal.aborted)
      setCommandState({ context, values: [...commands.values()] });
  }, [commands, context, signal]);
  const tell = useCallback(
    (value: Notice) => {
      if (!signal.aborted) setNotice({ context, value });
    },
    [context, signal],
  );

  useEffect(() => () => owner.abort(), [owner]);
  useEffect(publish, [publish]);

  const query = useCallback(
    async <K extends ViewMethod>(
      method: K,
      input: MethodInputs[K],
    ): Promise<MethodOutputs[K] | undefined> => {
      if (signal.aborted) return;
      requests.get(method)?.abort();
      const request = new AbortController();
      requests.set(method, request);
      const lifetime = AbortSignal.any([signal, request.signal]);
      setProjections((previous) => {
        const existing = previous?.context === context ? previous.values : {};
        const prior = existing[method] as Projection<K> | undefined;
        const retain =
          method === METHODS.list ||
          method === METHODS.catalog ||
          method === METHODS.uploads ||
          (method === METHODS.history &&
            prior &&
            "id" in prior.input &&
            "id" in input &&
            prior.input.id === input.id) ||
          (prior && stringifyJSON(prior.input) === stringifyJSON(input));
        return {
          context,
          values: {
            ...existing,
            [method]: {
              input: retain && prior?.value ? prior.input : input,
              value: retain ? prior?.value : undefined,
              loading: true,
            },
          },
        };
      });
      try {
        const value = await operations.query(method, input, lifetime);
        if (lifetime.aborted) return;
        setProjections((previous) => ({
          context,
          values: {
            ...(previous?.context === context ? previous.values : {}),
            [method]: { input, value, loading: false },
          },
        }));
        return value;
      } catch (error) {
        if (!lifetime.aborted)
          setProjections((previous) => {
            const existing =
              previous?.context === context ? previous.values : {};
            return {
              context,
              values: {
                ...existing,
                [method]: {
                  ...existing[method],
                  input: existing[method]?.input ?? input,
                  loading: false,
                  error: failureText(error),
                },
              },
            };
          });
      }
    },
    [context, operations, requests, signal],
  );

  const open = useCallback(
    (value: Selection) => {
      if (signal.aborted) return;
      if (sameSelection(current.current, value)) return;
      requests.get(METHODS.read)?.abort();
      if (current.current?.id !== value.id) {
        requests.get(METHODS.history)?.abort();
        setHistoryPages([]);
      }
      current.current = value;
      setTarget({ context, value });
    },
    [context, requests, signal],
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
    void query(METHODS.list, { limit: LIMITS.pageSize });
    void query(METHODS.uploads, {});
  }, [query]);
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
  }, [query, values]);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  const observe = useCallback(
    async (
      original: CommandObservation,
      promise: Promise<unknown>,
    ): Promise<unknown> => {
      commands.set(original.id, {
        ...original,
        status: "running",
        error: undefined,
      });
      publish();
      try {
        const result = await promise;
        if (signal.aborted) return;
        commands.delete(original.id);
        const notice: Notice = { kind: "command", method: original.method };
        if (original.method === METHODS.export) notice.asset = result as Asset;
        if (
          original.method === METHODS.create ||
          original.method === METHODS.edit ||
          original.method === METHODS.import ||
          original.method === METHODS.recalculate
        )
          notice.revision = result as Revision;
        if (
          original.method !== METHODS.append &&
          original.method !== METHODS.upload
        )
          tell(notice);
        // This callback belongs to the original authored action, never the current draft.
        try {
          original.settled?.(result);
        } catch (error) {
          tell({ kind: "error", text: failureText(error) });
        }
        if (
          original.method === METHODS.upload ||
          original.method === METHODS.discard
        )
          void query(METHODS.uploads, {});
        else if (
          original.method !== METHODS.append &&
          original.method !== METHODS.export
        )
          void refreshRef.current();
        return result;
      } catch (error) {
        if (signal.aborted) return;
        const pending = operations
          .pendingCommands(original.method)
          .find((entry) => entry.idempotencyKey === original.id);
        if (
          mutationSettlementIsUnknown(error) ||
          (original.recover &&
            !(error instanceof OperationFailure) &&
            !isErrorType(error, "plugin_command_retired"))
        ) {
          commands.set(original.id, {
            ...original,
            status: "unknown",
            error: failureText(error),
            recover: pending ? () => pending.recover(signal) : original.recover,
            retire: pending ? () => pending.retire(signal) : original.retire,
          });
        } else {
          commands.delete(original.id);
          tell({ kind: "error", text: failureText(error) });
        }
      } finally {
        publish();
      }
    },
    [commands, operations, publish, query, signal, tell],
  );

  const run = useCallback(
    async <K extends CommandMethod>(
      method: K,
      input: MethodInputs[K],
      settled?: (value: MethodOutputs[K]) => void,
    ): Promise<MethodOutputs[K] | undefined> => {
      if (
        signal.aborted ||
        (commands.size && method !== METHODS.discard) ||
        [...commands.values()].some(
          (entry) =>
            entry.method === METHODS.discard && method === METHODS.discard,
        )
      )
        return;
      try {
        const promise = operations.command(method, input);
        return (await observe(
          {
            id: promise.idempotencyKey,
            method,
            status: "running",
            settled: settled
              ? (value) => settled(value as MethodOutputs[K])
              : undefined,
          },
          promise,
        )) as MethodOutputs[K] | undefined;
      } catch (error) {
        tell({ kind: "error", text: failureText(error) });
      }
    },
    [commands, observe, operations, signal, tell],
  );

  const save = useCallback(
    async (asset: Asset) => {
      if (signal.aborted || saving.active) return;
      const controller = new AbortController();
      saving.controller = controller;
      saving.active = true;
      const lifetime = AbortSignal.any([signal, controller.signal]);
      setTransfer({
        context,
        filename: asset.filename,
        received: 0,
        total: Number(asset.size),
      });
      try {
        const bytes = await downloadAsset(
          (input, owned) => operations.query(METHODS.download, input, owned),
          asset,
          lifetime,
          (received, total) => {
            if (!lifetime.aborted)
              setTransfer({
                context,
                filename: asset.filename,
                received,
                total,
              });
          },
        );
        lifetime.throwIfAborted();
        const result = await context.saveFile({
          filename: asset.filename,
          mime: asset.mime,
          bytes,
        });
        if (!signal.aborted)
          tell({ kind: "save", result, filename: asset.filename });
      } catch (error) {
        if (!lifetime.aborted)
          tell({ kind: "error", text: failureText(error) });
      } finally {
        saving.active = false;
        saving.controller = undefined;
        if (!signal.aborted) setTransfer(undefined);
      }
    },
    [context, operations, saving, signal, tell],
  );

  const page = useCallback(
    async (kind: "list" | "history", direction: "next" | "previous") => {
      if (kind === "list") {
        const prior = values[METHODS.list];
        if (prior?.loading) return;
        const before =
          direction === "next" ? prior?.value?.next : listPages.at(-1);
        if (direction === "next" ? !before : !listPages.length) return;
        if (
          await query(METHODS.list, {
            limit: LIMITS.pageSize,
            ...(before ? { before } : {}),
          })
        )
          setListPages((pages) =>
            direction === "next"
              ? [...pages, prior?.input.before]
              : pages.slice(0, -1),
          );
      } else {
        const captured = current.current;
        const prior = values[METHODS.history];
        if (!captured || prior?.loading) return;
        const before =
          direction === "next" ? prior?.value?.next : historyPages.at(-1);
        if (direction === "next" ? !before : !historyPages.length) return;
        const result = await query(METHODS.history, {
          id: captured.id,
          limit: LIMITS.pageSize,
          ...(before ? { before } : {}),
        });
        if (result && current.current?.id === captured.id)
          setHistoryPages((pages) =>
            direction === "next"
              ? [...pages, prior?.input.before]
              : pages.slice(0, -1),
          );
      }
    },
    [historyPages, listPages, query, values],
  );

  return {
    context,
    signal,
    operations,
    selection,
    open,
    selectRevision,
    run,
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
    commands:
      commandState?.context === context
        ? commandState.values
        : [...commands.values()],
    blocked: commands.size > 0,
    notice: noticeState?.context === context ? noticeState.value : undefined,
    refreshUploads: () => query(METHODS.uploads, {}),
    recover: (command: CommandObservation) => {
      if (!signal.aborted && command.status === "unknown" && command.recover)
        void observe(command, command.recover());
    },
    retire: async (command: CommandObservation) => {
      if (signal.aborted || command.status !== "unknown" || !command.retire)
        return;
      commands.set(command.id, {
        ...command,
        status: "running",
        error: undefined,
      });
      publish();
      try {
        await command.retire();
        if (signal.aborted) return;
        commands.delete(command.id);
      } catch (error) {
        if (!signal.aborted)
          commands.set(command.id, { ...command, error: failureText(error) });
      } finally {
        publish();
      }
    },
    save,
    transfer: transfer?.context === context ? transfer : undefined,
    cancelTransfer: () => saving.controller?.abort(),
    clearSelection: (captured: Selection) => {
      if (!signal.aborted && sameSelection(current.current, captured)) {
        requests.get(METHODS.read)?.abort();
        requests.get(METHODS.history)?.abort();
        current.current = undefined;
        setTarget(undefined);
      }
    },
  };
}

export type DeliverablesState = ReturnType<typeof useDeliverables>;
