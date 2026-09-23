import {
  ASYNC_OWNERSHIP_RETIRED as ABORTED,
  disposeAsyncIterable,
  disposeAsyncIterator,
  settleBeforeAbort,
  settleWithinNextTask,
} from "@/lib/asyncOwnership";
import type { WorkspaceEventLike } from "../domain/eventInvalidation";
import type { RuntimeConnectionGeneration } from "@/plugins/builtin/runtime/public/services";

const EVENT_OPENING_TIMEOUT_MS = 10_000;
const RETARGET = Symbol("workspace-events.retarget");

class WorkspaceEventOpeningTimeoutError extends Error {
  override readonly name = "WorkspaceEventOpeningTimeoutError";

  constructor() {
    super("runtime_event_subscription_opening_timeout");
  }
}

export interface WorkspaceEventLoopDeps {
  subscribe(input: {
    target: WorkspaceWatchTarget;
    signal: AbortSignal;
  }): Promise<AsyncIterable<WorkspaceEventLike>>;
  handleEvent(ev: WorkspaceEventLike): void;
  invalidateAll(): void;
  reportDisconnect(connectionGeneration: RuntimeConnectionGeneration, error?: unknown): void;
  openingTimeoutMs?: number;
}

export interface WorkspaceEventLoop {
  start(signal: AbortSignal, connectionGeneration: RuntimeConnectionGeneration): Promise<void>;
  retarget(target: WorkspaceWatchTarget): void;
}

export type WorkspaceWatchTarget = { type: "none" } | { type: "workspace"; cwd?: string };

function sameTarget(left: WorkspaceWatchTarget, right: WorkspaceWatchTarget): boolean {
  return (
    left.type === right.type &&
    (left.type === "none" || right.type === "none" || left.cwd === right.cwd)
  );
}

export function createWorkspaceEventLoop(deps: WorkspaceEventLoopDeps): WorkspaceEventLoop {
  let watchTarget: WorkspaceWatchTarget = { type: "none" };
  let iterAbort: AbortController | null = null;
  let generationAbort: AbortController | null = null;
  let generationLease: object = {};

  return {
    start(signal, connectionGeneration) {
      generationAbort?.abort();
      const cohort = new AbortController();
      generationAbort = cohort;
      const ownGeneration = (generationLease = {});
      const abortCohort = () => cohort.abort(signal.reason);
      if (signal.aborted) abortCohort();
      else signal.addEventListener("abort", abortCohort, { once: true });
      return subscribeLoop(
        deps,
        cohort.signal,
        connectionGeneration,
        () => watchTarget,
        (next) => {
          if (generationLease === ownGeneration) iterAbort = next;
        },
      ).finally(() => {
        signal.removeEventListener("abort", abortCohort);
        if (generationLease !== ownGeneration) return;
        iterAbort = null;
        generationAbort = null;
      });
    },
    retarget(target) {
      if (sameTarget(target, watchTarget)) return;
      watchTarget = target;
      iterAbort?.abort(RETARGET);
    },
  };
}

async function subscribeLoop(
  deps: WorkspaceEventLoopDeps,
  signal: AbortSignal,
  connectionGeneration: RuntimeConnectionGeneration,
  watchTarget: () => WorkspaceWatchTarget,
  setIterAbort: (controller: AbortController | null) => void,
): Promise<void> {
  while (!signal.aborted) {
    const iter = new AbortController();
    setIterAbort(iter);
    const onOuterAbort = () => iter.abort();
    signal.addEventListener("abort", onOuterAbort, { once: true });
    let failure: unknown;
    try {
      const opening = deps.subscribe({ target: watchTarget(), signal: iter.signal });
      const events = await settleOpening(
        opening,
        iter,
        deps.openingTimeoutMs ?? EVENT_OPENING_TIMEOUT_MS,
      );
      if (events === ABORTED) continue;
      if (iter.signal.aborted) continue;
      const iterator = events[Symbol.asyncIterator]();
      let iteratorDone = false;
      try {
        deps.invalidateAll();
        let lastSequence = 0;
        while (!iter.signal.aborted) {
          const pendingNext = Promise.resolve(iterator.next());
          const next = await settleBeforeAbort(pendingNext, iter.signal);
          if (next === ABORTED) {
            const lateNext = await settleWithinNextTask(pendingNext);
            if (lateNext.status === "fulfilled" && lateNext.value.done) iteratorDone = true;
            break;
          }
          if (next.done) {
            iteratorDone = true;
            break;
          }
          const ev = next.value;
          if (ev.sequence <= lastSequence) continue;
          if (ev.sequence > lastSequence + 1) {
            deps.invalidateAll();
          }
          lastSequence = ev.sequence;
          deps.handleEvent(ev);
        }
      } finally {
        if (!iteratorDone) await disposeAsyncIterator(iterator);
      }
    } catch (error) {
      if (!signal.aborted && iter.signal.reason !== RETARGET) failure = error;
    } finally {
      signal.removeEventListener("abort", onOuterAbort);
      setIterAbort(null);
    }
    if (signal.aborted) return;
    if (iter.signal.reason === RETARGET) {
      continue;
    }
    deps.reportDisconnect(connectionGeneration, failure);
    return;
  }
}

function settleOpening<T>(
  operation: Promise<AsyncIterable<T>>,
  controller: AbortController,
  timeoutMs: number,
): Promise<AsyncIterable<T> | typeof ABORTED> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let deadlineSettled = false;
  let releaseDeadline!: () => void;
  const deadline = new Promise<never>((_resolve, reject) => {
    releaseDeadline = () => {
      if (deadlineSettled) return;
      deadlineSettled = true;
      reject();
    };
    timer = setTimeout(() => {
      timer = undefined;
      deadlineSettled = true;
      const error = new WorkspaceEventOpeningTimeoutError();
      reject(error);
      controller.abort(error);
    }, timeoutMs);
  });
  return Promise.race([
    settleBeforeAbort(operation, controller.signal, (late) => void disposeAsyncIterable(late)),
    deadline,
  ]).finally(() => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    releaseDeadline();
  });
}
