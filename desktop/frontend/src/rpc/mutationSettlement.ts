import { mutationSettlementIsUnknown, type MutationPromise } from "./mutation";

export const MUTATION_ATTEMPT_TIMEOUT_MS = 30_000;

export class MutationSettlementClosedError extends Error {
  override readonly name = "MutationSettlementClosedError";

  constructor() {
    super("Mutation settlement owner is closed");
  }
}

/**
 * Whether the winning attempt's signal still belongs to the generation after the command
 * settles. `retained` is for a result that carries a live stream owned by that signal: only
 * the opening deadline is released, so disposing the settler still revokes the stream.
 */
export type AcceptedAttempt = "released" | "retained";

export interface MutationSettlerConfig {
  acceptedAttempt?: AcceptedAttempt;
}

export interface MutationSettleOptions {
  /** The caller's own cancellation, joined with this settler's lifetime. */
  parent?: AbortSignal;
  timeoutMs?: number;
}

interface PendingMutation<T> {
  mutation: MutationPromise<T>;
}

export interface MutationSettler {
  /**
   * Settle one product command. `identity` names the command while its outcome remains
   * unknown; a later call with the same identity replays the retained MutationPromise instead
   * of opening a second logical mutation.
   */
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
    // A released deadline can only settle after its operation already won the race (or before
    // no race was installed). Resolving the `never` branch is an ownership signal, never a
    // product result.
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
    // Do not depend on a transport honoring AbortSignal to settle. The signal stops
    // cooperative work; the race independently releases the product command latch when a
    // socket or custom transport ignores cancellation.
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
      // A cancel leaves the command's outcome unknown — the Runtime may already hold it — so
      // the identity is retained for a later owner rather than retried here.
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
      // The journal refuses a replay it no longer owns by throwing from here, before any
      // promise exists, which would otherwise leave this attempt's deadline armed.
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

/**
 * Own unresolved mutation identities for one Runtime adapter generation. Product layers choose
 * a semantic identity; transport and idempotency handles stay here, so a component unmount or
 * a bounded settlement failure cannot turn the next explicit retry into a new Runtime command.
 */
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

      // Only a command that already returned settlement-unknown may be reused. Two fresh
      // same-shaped calls can still be separate product intents; their application owner, not
      // the transport adapter, decides whether to join.
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
