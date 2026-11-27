import { METHODS } from "../contracts/methods";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  bindOperations,
  CursorPager,
  failureText,
  type ComponentContext,
  type PageDirection,
} from "@flame/component-sdk";
import {
  CAPABILITY_JSON,
  type MethodChunks,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";
import type { HistoryPage, OperationRecord, Snapshot, Surface } from "./types";
import { LIMITS } from "../contracts/limits";

import { useComputerCommands } from "./useComputerCommands";

type Read<T> = {
  value?: T;
  loading: boolean;
  error?: string;
};

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
  const { signal } = context;
  const [read, setInspection] = useState<Read<Snapshot>>();
  const [surface, setSurfaceRead] = useState<Read<Surface>>();
  const historyPager = useMemo(
    () =>
      new CursorPager<HistoryPage>(signal, (before, readSignal) =>
        operations.query(
          METHODS.history,
          { ...(before ? { before } : {}), limit: LIMITS.maxHistoryPage },
          readSignal,
        ),
      ),
    [operations, signal],
  );
  const history = useSyncExternalStore(
    historyPager.subscribe,
    historyPager.snapshot,
  );
  const [result, setResultRead] = useState<
    Read<OperationRecord> & { id: string }
  >();
  const [watchError, setWatchError] = useState<
    { kind: "ended" } | { kind: "failed"; message: string }
  >();
  const [watchVersion, setWatchVersion] = useState(0);
  const [surfaceVersion, setSurfaceVersion] = useState(0);
  const resultRequest = useRef<AbortController>(undefined);
  const readRequest = useRef<AbortController>(undefined);

  const snapshot = read?.value;

  const refresh = useCallback(async () => {
    signal.throwIfAborted();
    readRequest.current?.abort();
    const request = new AbortController();
    readRequest.current = request;
    const lifetime = AbortSignal.any([signal, request.signal]);
    setInspection((previous) => ({ value: previous?.value, loading: true }));
    try {
      const value = await operations.query(METHODS.read, {}, lifetime);
      if (!lifetime.aborted) setInspection({ value, loading: false });
    } catch (error) {
      if (!lifetime.aborted)
        setInspection((previous) => ({
          value: previous?.value,
          loading: false,
          error: failureText(error),
        }));
    }
  }, [operations, setInspection, signal]);

  const inspect = useCallback(
    async (id: string) => {
      signal.throwIfAborted();
      resultRequest.current?.abort();
      const request = new AbortController();
      resultRequest.current = request;
      const lifetime = AbortSignal.any([signal, request.signal]);
      setResultRead((previous) => ({
        id,
        value: previous?.id === id ? previous.value : undefined,
        loading: true,
      }));
      try {
        const value = await operations.query(METHODS.result, { id }, lifetime);
        if (!lifetime.aborted) setResultRead({ id, value, loading: false });
      } catch (error) {
        if (!lifetime.aborted)
          setResultRead((previous) => ({
            id,
            value: previous?.id === id ? previous.value : undefined,
            loading: false,
            error: failureText(error),
          }));
      }
    },
    [operations, setResultRead, signal],
  );

  const closeInspection = useCallback(() => {
    resultRequest.current?.abort();
    setResultRead(undefined);
  }, []);

  useEffect(() => {
    const request = new AbortController();
    const lifetime = AbortSignal.any([signal, request.signal]);
    setWatchError(undefined);
    setInspection((previous) => ({ value: previous?.value, loading: true }));
    void operations
      .queryStream(
        METHODS.watch,
        {},
        (value, chunkSignal) => {
          if (lifetime.aborted || chunkSignal.aborted) return;
          // A live owner snapshot supersedes an older point read in flight.
          readRequest.current?.abort();
          setInspection({ value, loading: false });
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
            value: previous?.value,
            loading: false,
          }));
        }
      });
    return () => request.abort();
  }, [operations, setInspection, signal, watchVersion]);

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
    void historyPager.refresh();
    return historyPager.stop;
  }, [historyPager, snapshot]);

  useEffect(() => {
    if (!canReadSurface || !sessionId || !workerId || !surfaceDigest) {
      setSurfaceRead(undefined);
      return;
    }
    const request = new AbortController();
    const lifetime = AbortSignal.any([signal, request.signal]);
    setSurfaceRead({ loading: true });
    void operations
      .query(METHODS.context, {}, lifetime)
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
        setSurfaceRead({ value, loading: false });
      })
      .catch((error: unknown) => {
        if (!lifetime.aborted)
          setSurfaceRead({ loading: false, error: failureText(error) });
      });
    return () => request.abort();
  }, [
    setSurfaceRead,
    operations,
    signal,
    sessionId,
    workerId,
    surfaceDigest,
    canReadSurface,
    surfaceVersion,
    refresh,
  ]);

  const onCommandConfirmed = useCallback(() => {
    void refresh();
    void historyPager.refresh();
  }, [refresh, historyPager]);
  const commandControls = useComputerCommands(
    operations,
    signal,
    currentTarget,
    onCommandConfirmed,
  );

  return {
    read,
    snapshot,
    surface,
    history,
    result,
    ...commandControls,
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
    loadHistory: historyPager.refresh,
    inspect,
    closeInspection,
    page: (direction: PageDirection) => historyPager.move(direction),
  };
}
