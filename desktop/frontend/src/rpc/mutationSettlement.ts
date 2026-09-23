import { mutationSettlementIsUnknown, type MutationPromise } from "./mutation";

export const MUTATION_ATTEMPT_TIMEOUT_MS = 30_000;

export class MutationSettlementClosedError extends Error {
  override readonly name = "MutationSettlementClosedError";

  constructor() {
    super("Mutation settlement owner is closed");
  }
}

type AcceptedAttempt = "released" | "retained";

export interface MutationSettlerConfig {
  acceptedAttempt?: AcceptedAttempt;
}

interface MutationSettleOptions {
  parent?: AbortSignal;
  timeoutMs?: number;
}

interface PendingMutation<T> {
  mutation: MutationPromise<T>;
}

export interface MutationSettler {
  settle<T>(
    identity: string,
    open: (signal: AbortSignal) => MutationPromise<T>,
    options?: MutationSettleOptions,
  ): Promise<T>;
  dispose(): void;
}

interface MutationAttempt {
  readonly signal: AbortSignal;
  readonly deadlineExpired: () => boolean;
  wait<T>(operation: PromiseLike<T>): Promise<T>;
  accept(): void;
  dispose(): void;
}

function createAttempt(
  ownership: AbortSignal,
  timeoutMs: number,
  acceptedAttempt: AcceptedAttempt,
): MutationAttempt {
  const controller = new AbortController();
  let expired = false;
  let deadlineSettled = false;
  let resolveDeadline!: () => void;
  let rejectDeadline!: (reason: unknown) => void;
  const deadline = new Promise<never>((resolve, reject) => {
    resolveDeadline = () => resolve(undefined as never);
    rejectDeadline = reject;
  });
  let timer: ReturnType<typeof setTimeout> | undefined = setTimeout(() => {
    timer = undefined;
    deadlineSettled = true;
    expired = true;
    const error = new DOMException("Mutation attempt timed out", "TimeoutError");
    controller.abort(error);
    rejectDeadline(error);
  }, timeoutMs);

  function detach() {
    ownership.removeEventListener("abort", abortFromOwnership);
  }
  const releaseDeadline = () => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    if (deadlineSettled) return;
    deadlineSettled = true;
    resolveDeadline();
  };
  function abortFromOwnership() {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    detach();
    const reason = ownership.reason ?? new MutationSettlementClosedError();
    if (!controller.signal.aborted) controller.abort(reason);
    if (deadlineSettled) return;
    deadlineSettled = true;
    rejectDeadline(reason);
  }

  if (ownership.aborted) abortFromOwnership();
  else ownership.addEventListener("abort", abortFromOwnership, { once: true });

  return {
    signal: controller.signal,
    deadlineExpired: () => expired,
    wait: (operation) => Promise.race([operation, deadline]),
    accept: () => {
      releaseDeadline();
      if (acceptedAttempt === "released") detach();
    },
    dispose: () => {
      releaseDeadline();
      detach();
      if (!controller.signal.aborted) controller.abort();
    },
  };
}

function driveMutation<T>(
  mutation: MutationPromise<T>,
  first: MutationAttempt,
  ownership: AbortSignal,
  timeoutMs: number,
  acceptedAttempt: AcceptedAttempt,
  markUnknown: () => void,
  replaceMutation: (mutation: MutationPromise<T>) => void,
): Promise<T> {
  return (async () => {
    try {
      const value = await first.wait(mutation);
      first.accept();
      return value;
    } catch (error) {
      const timedOut = first.deadlineExpired();
      const canceled = ownership.aborted;
      first.dispose();
      if (!timedOut || canceled) {
        if (canceled || mutationSettlementIsUnknown(error)) markUnknown();
        throw error;
      }
    }

    const retry = createAttempt(ownership, timeoutMs, acceptedAttempt);
    let replay: MutationPromise<T>;
    try {
      replay = mutation.retry({ signal: retry.signal });
    } catch (error) {
      retry.dispose();
      throw error;
    }
    replaceMutation(replay);
    try {
      const value = await retry.wait(replay);
      retry.accept();
      return value;
    } catch (error) {
      if (retry.deadlineExpired() || ownership.aborted || mutationSettlementIsUnknown(error)) {
        markUnknown();
      }
      retry.dispose();
      throw error;
    }
  })();
}

export function createMutationSettler(config: MutationSettlerConfig = {}): MutationSettler {
  const acceptedAttempt = config.acceptedAttempt ?? "released";
  const pending = new Map<string, PendingMutation<unknown>[]>();
  const replaying = new Map<string, Promise<unknown>>();
  const lifetime = new AbortController();
  let disposed = false;

  const retain = (identity: string, record: PendingMutation<unknown>) => {
    if (disposed) return;
    const queue = pending.get(identity) ?? [];
    queue.push(record);
    pending.set(identity, queue);
  };

  const take = <T>(identity: string): PendingMutation<T> | undefined => {
    const queue = pending.get(identity);
    const record = queue?.shift() as PendingMutation<T> | undefined;
    if (queue?.length === 0) pending.delete(identity);
    return record;
  };

  return {
    settle<T>(
      identity: string,
      open: (signal: AbortSignal) => MutationPromise<T>,
      options: MutationSettleOptions = {},
    ): Promise<T> {
      if (disposed) return Promise.reject(new MutationSettlementClosedError());
      const activeReplay = replaying.get(identity) as Promise<T> | undefined;
      if (activeReplay) return activeReplay;

      const retained = take<T>(identity);

      const timeoutMs = options.timeoutMs ?? MUTATION_ATTEMPT_TIMEOUT_MS;
      const ownership = options.parent
        ? AbortSignal.any([options.parent, lifetime.signal])
        : lifetime.signal;
      const first = createAttempt(ownership, timeoutMs, acceptedAttempt);
      let mutation: MutationPromise<T>;
      try {
        mutation = retained
          ? retained.mutation.retry({ signal: first.signal })
          : open(first.signal);
      } catch (error) {
        first.dispose();
        if (retained) retain(identity, retained as PendingMutation<unknown>);
        return Promise.reject(error);
      }

      const record = retained ?? { mutation };
      record.mutation = mutation;

      let unknown = false;
      const settlement = driveMutation(
        mutation,
        first,
        ownership,
        timeoutMs,
        acceptedAttempt,
        () => {
          unknown = true;
        },
        (replay) => {
          record.mutation = replay;
        },
      );
      const tracked = settlement
        .then(
          (value) => value,
          (error: unknown) => {
            if (unknown) retain(identity, record as PendingMutation<unknown>);
            throw error;
          },
        )
        .finally(() => {
          if (replaying.get(identity) === tracked) replaying.delete(identity);
        });
      if (retained) replaying.set(identity, tracked as Promise<unknown>);
      return tracked;
    },
    dispose(): void {
      if (disposed) return;
      disposed = true;
      lifetime.abort(new MutationSettlementClosedError());
      pending.clear();
      replaying.clear();
    },
  };
}
