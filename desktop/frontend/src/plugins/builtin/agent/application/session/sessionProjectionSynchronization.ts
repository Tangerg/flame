export interface SessionProjectionSynchronization {
  /** Coalesce an authoritative refresh request. A request made while the live
   *  stream owns the session is retained until that stream becomes idle. The
   *  returned promise settles with the refresh's authoritative commit fact. */
  request(): Promise<boolean>;
  /** Supersede the active Runtime generation. The current synchronization is
   * retired even when one of its reads does not cooperate with cancellation. */
  replace(): Promise<boolean>;
  /** Revoke the active and queued generation without admitting a successor. */
  retire(): void;
  liveStreamSettled(): void;
  dispose(): void;
}

interface SessionProjectionSynchronizationOptions {
  isLiveStreamActive: () => boolean;
  synchronize: (signal: AbortSignal) => Promise<boolean>;
}

/**
 * Serializes the two fact channels which feed one mounted session projection.
 *
 * Run events are the ordered, low-latency owner while a stream is active. The
 * durable snapshot is the reconciliation owner while no stream is active.
 * Change notifications may arrive at any time, so requests are coalesced and
 * drained only at that ownership boundary instead of racing both writers.
 */
export function createSessionProjectionSynchronization({
  isLiveStreamActive,
  synchronize,
}: SessionProjectionSynchronizationOptions): SessionProjectionSynchronization {
  type Refresh = ReturnType<typeof Promise.withResolvers<boolean>>;
  let disposed = false;
  // Snapshot settlement leaves its subscription alive until this generation retires.
  let generationAbort: AbortController | null = null;
  let pending: Refresh | null = null;
  let active: Refresh | null = null;

  const drain = (): void => {
    if (disposed || active || !pending || isLiveStreamActive()) return;
    const refresh = pending;
    pending = null;
    active = refresh;
    generationAbort?.abort();
    const controller = new AbortController();
    generationAbort = controller;
    void settleBeforeAbort(synchronize(controller.signal), controller.signal)
      .then((committed) => (committed === ABORTED ? false : committed))
      .catch(() => false)
      .then(refresh.resolve)
      .finally(() => {
        active = null;
        drain();
      });
  };

  const enqueue = (): Promise<boolean> => {
    if (disposed) return Promise.resolve(false);
    pending ??= Promise.withResolvers<boolean>();
    const promise = pending.promise;
    drain();
    return promise;
  };

  const retire = (): void => {
    generationAbort?.abort();
    pending?.resolve(false);
    pending = null;
    active?.resolve(false);
  };

  return {
    request: enqueue,
    replace() {
      generationAbort?.abort();
      return enqueue();
    },
    retire,
    liveStreamSettled: drain,
    dispose() {
      disposed = true;
      retire();
      generationAbort = null;
    },
  };
}

import { ASYNC_OWNERSHIP_RETIRED as ABORTED, settleBeforeAbort } from "@/lib/asyncOwnership";
