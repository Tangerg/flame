import {
  ASYNC_OWNERSHIP_RETIRED as ABORTED,
  disposeAsyncIterator,
  settleBeforeAbort,
  settleWithinNextTask,
} from "@/lib/asyncOwnership";
import type { WorkspaceEventLike } from "../domain/eventInvalidation";
import type { RuntimeConnectionGeneration } from "@/plugins/builtin/runtime/public/services";

const RECONNECT_BASE_MS = 1_000;
const RECONNECT_CAP_MS = 30_000;
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

/**
 * `none` means the app-wide topics stay subscribed without a file watch while
 * active-session identity is unresolved. It is intentionally distinct from a
 * resolved workspace with no cwd, which means the Runtime's default workspace.
 */
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
      // The loop itself owns the one active subscription generation. Callers
      // normally withdraw capability before restarting, but correctness must
      // not depend on that ordering: a repeated start atomically supersedes
      // the prior generation even if its caller forgot to abort its signal.
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
  let attempt = 0;
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
      // A transport may resolve its opening promise at the same instant a
      // retarget abort wins. Do not publish the stale subscription's initial
      // resync or any already-buffered event into the new workspace target.
      if (iter.signal.aborted) continue;
      const iterator = events[Symbol.asyncIterator]();
      let iteratorDone = false;
      try {
        attempt = 0;
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
          // Sequence belongs to this subscription generation. Once a forward
          // gap has forced an authoritative resync, a duplicated or delayed
          // lower frame is already covered by that snapshot and must not move
          // the watermark backwards or replace every mounted read model again.
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
      attempt = 0;
      continue;
    }
    // An RPC stream ending without outer cancellation is also a connection
    // signal. Let the Runtime context withdraw this exact connection and recover
    // instead of allowing this consumer to guess global connection health.
    deps.reportDisconnect(connectionGeneration, failure);
    // The connection owner withdraws the generation synchronously. That aborts
    // this exact loop before its asynchronous recovery inspection begins; do not
    // leave a predecessor reconnect timer behind that boundary.
    if (signal.aborted) return;
    const backoff = new AbortController();
    setIterAbort(backoff);
    const abortBackoff = () => backoff.abort();
    signal.addEventListener("abort", abortBackoff, { once: true });
    await delay(Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_CAP_MS), backoff.signal);
    signal.removeEventListener("abort", abortBackoff);
    setIterAbort(null);
    if (signal.aborted) return;
    if (backoff.signal.reason === RETARGET) {
      attempt = 0;
      continue;
    }
    attempt += 1;
  }
}

/** Give the response-stream handshake a terminal lifecycle without applying a
 * wall-clock limit to the accepted stream. Reject the deadline before aborting
 * so this cohort reports a connection failure rather than looking like an
 * ordinary retarget; settleBeforeAbort keeps a non-cooperative late opening
 * observed and retires the foreign iterable when it eventually arrives. */
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
    settleBeforeAbort(operation, controller.signal, disposeIterable),
    deadline,
  ]).finally(() => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    releaseDeadline();
  });
}

function disposeIterable<T>(iterable: AsyncIterable<T>): void {
  try {
    void disposeAsyncIterator(iterable[Symbol.asyncIterator]());
  } catch {
    // The subscription was already superseded, so its signal remains the
    // authoritative teardown path when constructing its iterator fails.
  }
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const timer = setTimeout(done, ms);
    function done(): void {
      clearTimeout(timer);
      signal.removeEventListener("abort", done);
      resolve();
    }
    signal.addEventListener("abort", done, { once: true });
  });
}
