export interface SessionProjectionSynchronization {
  request(): Promise<boolean>;
  replace(): Promise<boolean>;
  retire(): void;
  liveStreamSettled(): void;
  dispose(): void;
}

interface SessionProjectionSynchronizationOptions {
  isLiveStreamActive: () => boolean;
  synchronize: (signal: AbortSignal) => Promise<boolean>;
}

export function createSessionProjectionSynchronization({
  isLiveStreamActive,
  synchronize,
}: SessionProjectionSynchronizationOptions): SessionProjectionSynchronization {
  type Refresh = ReturnType<typeof Promise.withResolvers<boolean>>;
  let disposed = false;
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
