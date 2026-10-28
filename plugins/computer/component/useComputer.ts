import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  bindOperations,
  failureText,
  isErrorType,
  mutationSettlementIsUnknown,
  OperationFailure,
  type ComponentContext,
} from "@flame/component-sdk";
import {
  CAPABILITY_JSON,
  type MethodChunks,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";
import { parseJSON } from "@qimu/sdk/json";
import type { HistoryPage, OperationRecord, Snapshot, Surface } from "./types";
import { LIMITS } from "../contracts/limits";

export const methods = {
  read: "flame.computer/read.v1",
  watch: "flame.computer/watch.v1",
  open: "flame.computer/open.v1",
  close: "flame.computer/close.v1",
  stop: "flame.computer/stop.v1",
  execute: "flame.computer/execute.v1",
  reply: "flame.computer/reply.v1",
  context: "flame.computer/context.v1",
  history: "flame.computer/history.v1",
  result: "flame.computer/result.v1",
} as const satisfies Record<string, keyof MethodInputs>;

type CommandMethod =
  | typeof methods.open
  | typeof methods.close
  | typeof methods.stop
  | typeof methods.execute
  | typeof methods.reply;

const commandMethods: readonly CommandMethod[] = [
  methods.open,
  methods.close,
  methods.stop,
  methods.execute,
  methods.reply,
];

type Read<T> = {
  context: ComponentContext;
  value?: T;
  loading: boolean;
  error?: string;
};

export interface CommandObservation {
  id: string;
  method: CommandMethod;
  target?: string;
  status: "running" | "unknown";
  error?: string;
  recover?: () => Promise<unknown>;
}

function commandTarget(
  method: CommandMethod,
  input: unknown,
): string | undefined {
  if (method === methods.open) return "installation";
  if (!input || typeof input !== "object" || Array.isArray(input)) return;
  const values = input as Record<string, unknown>;
  const fields =
    method === methods.close
      ? ["sessionId"]
      : method === methods.stop
        ? ["sessionId", "operationId"]
        : method === methods.reply
          ? ["sessionId", "requestId"]
          : ["sessionId", "workerId", "surfaceDigest"];
  if (
    fields.some((field) => typeof values[field] !== "string" || !values[field])
  )
    return;
  return JSON.stringify(fields.map((field) => values[field]));
}

function blocks(
  command: CommandObservation,
  method: CommandMethod,
  target: string | undefined,
): boolean {
  if (command.method !== method) return false;
  if (command.target !== undefined) return command.target === target;
  // A missing original receipt cannot establish that a revocation or permission
  // reply concerns the current resource. It must not disable that resource's
  // controls. Execution and lease acquisition remain blocked until identified.
  return method === methods.execute || method === methods.open;
}

export function useComputer(context: ComponentContext) {
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("computer component requires its callable contribution");
    return bindOperations<MethodInputs, MethodOutputs, MethodChunks>(
      context,
      CAPABILITY_JSON,
      { scope: "installation", reference: context.binding.installationId },
    );
  }, [context]);
  const owner = useMemo(() => new AbortController(), [context]);
  const signal = useMemo(
    () => AbortSignal.any([context.signal, owner.signal]),
    [context.signal, owner],
  );
  const [inspection, setInspection] = useState<Read<Snapshot>>();
  const [surfaceRead, setSurfaceRead] = useState<Read<Surface>>();
  const [historyRead, setHistoryRead] = useState<Read<HistoryPage>>();
  const [resultRead, setResultRead] = useState<
    Read<OperationRecord> & { id: string }
  >();
  const [watchError, setWatchError] = useState<
    { kind: "ended" } | { kind: "failed"; message: string }
  >();
  const [watchVersion, setWatchVersion] = useState(0);
  const [surfaceVersion, setSurfaceVersion] = useState(0);
  const [commandView, setCommandView] = useState<CommandObservation[]>([]);
  const commands = useRef(new Map<string, CommandObservation>());
  const [notice, setNotice] = useState<
    { kind: "success"; method: CommandMethod } | { kind: "error"; text: string }
  >();
  const historyRequest = useRef<AbortController>(undefined);
  const resultRequest = useRef<AbortController>(undefined);
  const readRequest = useRef<AbortController>(undefined);
  const cursor = useRef<string>(undefined);
  const [previousPages, setPreviousPages] = useState<Array<string | undefined>>(
    [],
  );

  const read = inspection?.context === context ? inspection : undefined;
  const snapshot = read?.value;
  const surface = surfaceRead?.context === context ? surfaceRead : undefined;
  const history = historyRead?.context === context ? historyRead : undefined;
  const result = resultRead?.context === context ? resultRead : undefined;

  const refresh = useCallback(async () => {
    signal.throwIfAborted();
    readRequest.current?.abort();
    const request = new AbortController();
    readRequest.current = request;
    const lifetime = AbortSignal.any([signal, request.signal]);
    setInspection((previous) => ({
      context,
      value: previous?.context === context ? previous.value : undefined,
      loading: true,
    }));
    try {
      const value = await operations.query(methods.read, {}, lifetime);
      if (!lifetime.aborted) setInspection({ context, value, loading: false });
    } catch (error) {
      if (!lifetime.aborted)
        setInspection((previous) => ({
          context,
          value: previous?.context === context ? previous.value : undefined,
          loading: false,
          error: failureText(error),
        }));
    }
  }, [context, operations, signal]);

  const loadHistory = useCallback(
    async (before: string | undefined = cursor.current) => {
      signal.throwIfAborted();
      historyRequest.current?.abort();
      const request = new AbortController();
      historyRequest.current = request;
      const lifetime = AbortSignal.any([signal, request.signal]);
      setHistoryRead((previous) => ({
        context,
        value: previous?.context === context ? previous.value : undefined,
        loading: true,
      }));
      try {
        const value = await operations.query(
          methods.history,
          { ...(before ? { before } : {}), limit: LIMITS.maxHistoryPage },
          lifetime,
        );
        if (!lifetime.aborted) {
          cursor.current = before;
          setHistoryRead({ context, value, loading: false });
          return true;
        }
        return false;
      } catch (error) {
        if (!lifetime.aborted)
          setHistoryRead((previous) => ({
            context,
            value: previous?.context === context ? previous.value : undefined,
            loading: false,
            error: failureText(error),
          }));
      }
    },
    [context, operations, signal],
  );

  const inspect = useCallback(
    async (id: string) => {
      signal.throwIfAborted();
      resultRequest.current?.abort();
      const request = new AbortController();
      resultRequest.current = request;
      const lifetime = AbortSignal.any([signal, request.signal]);
      setResultRead((previous) => ({
        context,
        id,
        value:
          previous?.context === context && previous.id === id
            ? previous.value
            : undefined,
        loading: true,
      }));
      try {
        const value = await operations.query(methods.result, { id }, lifetime);
        if (!lifetime.aborted)
          setResultRead({ context, id, value, loading: false });
      } catch (error) {
        if (!lifetime.aborted)
          setResultRead((previous) => ({
            context,
            id,
            value:
              previous?.context === context && previous.id === id
                ? previous.value
                : undefined,
            loading: false,
            error: failureText(error),
          }));
      }
    },
    [context, operations, signal],
  );

  const closeInspection = useCallback(() => {
    resultRequest.current?.abort();
    setResultRead(undefined);
  }, []);

  useEffect(() => () => owner.abort(), [owner]);

  useEffect(() => {
    const request = new AbortController();
    const lifetime = AbortSignal.any([signal, request.signal]);
    setWatchError(undefined);
    setInspection((previous) => ({
      context,
      value: previous?.context === context ? previous.value : undefined,
      loading: true,
    }));
    void operations
      .queryStream(
        methods.watch,
        {},
        (value, chunkSignal) => {
          if (lifetime.aborted || chunkSignal.aborted) return;
          // A live owner snapshot supersedes an older point read in flight.
          readRequest.current?.abort();
          setInspection({ context, value, loading: false });
        },
        lifetime,
      )
      .then(() => {
        if (!lifetime.aborted) setWatchError({ kind: "ended" });
      })
      .catch((error: unknown) => {
        if (!lifetime.aborted) {
          setWatchError({ kind: "failed", message: failureText(error) });
          setInspection((previous) => ({
            context,
            value: previous?.context === context ? previous.value : undefined,
            loading: false,
          }));
        }
      });
    return () => request.abort();
  }, [context, operations, signal, watchVersion]);

  const session = snapshot?.session;
  const sessionId = session?.id;
  const workerId = session?.workerId;
  const surfaceDigest = session?.surfaceDigest;
  const canReadSurface =
    session?.state === "ready" || session?.state === "executing";
  const currentTarget = {
    sessionId,
    workerId,
    surfaceDigest,
    operationId: session?.operation?.id,
    requestId: session?.approval?.id,
  };
  useEffect(() => {
    void loadHistory();
  }, [loadHistory, snapshot]);

  useEffect(() => {
    if (!canReadSurface || !sessionId || !workerId || !surfaceDigest) {
      setSurfaceRead(undefined);
      return;
    }
    const request = new AbortController();
    const lifetime = AbortSignal.any([signal, request.signal]);
    setSurfaceRead({ context, loading: true });
    void operations
      .query(methods.context, {}, lifetime)
      .then((value) => {
        if (lifetime.aborted) return;
        if (
          value.sessionId !== sessionId ||
          value.workerId !== workerId ||
          value.surfaceDigest !== surfaceDigest
        ) {
          setSurfaceRead(undefined);
          void refresh();
          return;
        }
        setSurfaceRead({ context, value, loading: false });
      })
      .catch((error: unknown) => {
        if (!lifetime.aborted)
          setSurfaceRead({
            context,
            loading: false,
            error: failureText(error),
          });
      });
    return () => request.abort();
  }, [
    context,
    operations,
    signal,
    sessionId,
    workerId,
    surfaceDigest,
    canReadSurface,
    surfaceVersion,
    refresh,
  ]);

  const publishCommands = useCallback(() => {
    if (!signal.aborted) setCommandView([...commands.current.values()]);
  }, [signal]);

  useEffect(() => {
    commands.current.clear();
    for (const method of commandMethods) {
      for (const original of operations.pendingCommands(method)) {
        commands.current.set(original.idempotencyKey, {
          id: original.idempotencyKey,
          method,
          status: "unknown",
          recover: () => original.recover(signal),
        });
        void operations
          .query(methods.result, { id: original.idempotencyKey }, signal)
          .then((record) => {
            if (signal.aborted) return;
            const pending = commands.current.get(original.idempotencyKey);
            if (!pending) return;
            if (
              record.id !== original.idempotencyKey ||
              record.kind !== "command" ||
              record.method !== method
            )
              throw new Error(
                "computer: original command evidence differs from its identity",
              );
            const target = commandTarget(method, parseJSON(record.inputJSON));
            if (target === undefined)
              throw new Error(
                "computer: original command target is unavailable",
              );
            commands.current.set(original.idempotencyKey, {
              ...pending,
              target,
            });
            publishCommands();
          })
          .catch((error: unknown) => {
            if (signal.aborted) return;
            const pending = commands.current.get(original.idempotencyKey);
            if (!pending) return;
            commands.current.set(original.idempotencyKey, {
              ...pending,
              error: pending.error ?? failureText(error),
            });
            publishCommands();
          });
      }
    }
    publishCommands();
  }, [operations, signal, publishCommands]);

  const observe = useCallback(
    async (original: CommandObservation, promise: Promise<unknown>) => {
      commands.current.set(original.id, {
        ...original,
        target: commands.current.get(original.id)?.target ?? original.target,
        status: "running",
        error: undefined,
      });
      publishCommands();
      setNotice(undefined);
      try {
        await promise;
        if (signal.aborted) return false;
        commands.current.delete(original.id);
        setNotice({ kind: "success", method: original.method });
        await Promise.all([refresh(), loadHistory()]);
        return true;
      } catch (error) {
        if (signal.aborted) return false;
        if (
          mutationSettlementIsUnknown(error) ||
          (original.recover &&
            !(error instanceof OperationFailure) &&
            !isErrorType(error, "plugin_command_retired"))
        ) {
          const retained = operations
            .pendingCommands(original.method)
            .find((command) => command.idempotencyKey === original.id);
          commands.current.set(original.id, {
            ...original,
            target:
              commands.current.get(original.id)?.target ?? original.target,
            status: "unknown",
            error: failureText(error),
            recover: retained
              ? () => retained.recover(signal)
              : original.recover,
          });
        } else {
          commands.current.delete(original.id);
          setNotice({ kind: "error", text: failureText(error) });
        }
        return false;
      } finally {
        publishCommands();
      }
    },
    [signal, operations, publishCommands, refresh, loadHistory],
  );

  const submit = useCallback(
    async <K extends CommandMethod>(method: K, input: MethodInputs[K]) => {
      const target = commandTarget(method, input);
      if (
        signal.aborted ||
        [...commands.current.values()].some((command) =>
          blocks(command, method, target),
        )
      )
        return false;
      try {
        if (target === undefined)
          throw new Error(
            "computer: command requires its captured resource target",
          );
        const promise = operations.command(method, input);
        return await observe(
          { id: promise.idempotencyKey, method, target, status: "running" },
          promise,
        );
      } catch (error) {
        if (!signal.aborted)
          setNotice({ kind: "error", text: failureText(error) });
        return false;
      }
    },
    [operations, signal, observe],
  );

  return {
    read,
    snapshot,
    surface,
    history,
    result,
    notice,
    commands: commandView,
    watchError,
    refresh: () => {
      if (!surface?.value || surface.error)
        setSurfaceVersion((value) => value + 1);
      return refresh();
    },
    reconnect: () => {
      if (!surface?.value || surface.error)
        setSurfaceVersion((value) => value + 1);
      setWatchVersion((value) => value + 1);
    },
    loadHistory,
    inspect,
    closeInspection,
    submit,
    busy: (method: CommandMethod) =>
      commandView.some((command) =>
        blocks(command, method, commandTarget(method, currentTarget)),
      ),
    recover: (command: CommandObservation) => {
      if (signal.aborted || command.status !== "unknown" || !command.recover)
        return;
      void observe(command, command.recover());
    },
    nextPage: async () => {
      if (!history?.value?.next || history.loading) return;
      const previous = cursor.current;
      if (await loadHistory(history.value.next))
        setPreviousPages((pages) => [...pages, previous]);
    },
    previousPage: async () => {
      if (!previousPages.length || history?.loading) return;
      const before = previousPages.at(-1);
      const current = cursor.current;
      cursor.current = before;
      if (await loadHistory(before))
        setPreviousPages((previous) => previous.slice(0, -1));
      else cursor.current = current;
    },
    hasPrevious: previousPages.length > 0,
  };
}
