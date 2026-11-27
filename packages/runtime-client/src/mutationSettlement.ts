import { MutationLookupError, type MutationPromise } from "./mutation";

const MUTATION_ATTEMPT_TIMEOUT_MS = 30_000;
const MAXIMUM_MUTATION_ATTEMPT_TIMEOUT_MS = 2 ** 31 - 1;

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

interface MutationSettleOptions<T> {
  parent?: AbortSignal;
  timeoutMs?: number;
  onCommand?: (command: MutationPromise<T>) => void;
}

function projectMutation<T>(
  settlement: Promise<T>,
  command: MutationPromise<T>,
  onCommand: MutationSettleOptions<T>["onCommand"],
): Promise<T> {
  try {
    onCommand?.(command);
  } catch (cause) {
    // A projection failure cannot orphan a dispatched command or its deadline.
    return settlement.then(
      () => {
        throw cause;
      },
      (error: unknown) => {
        throw new AggregateError([cause, error], "mutation identity observer failed", { cause });
      },
    );
  }
  return settlement;
}

export interface MutationSettler {
  settle<T>(
    open: (signal: AbortSignal) => MutationPromise<T>,
    options?: MutationSettleOptions<T>,
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
  // An attempt whose operation never opens is never raced; its rejection is
  // still delivered to every wait().
  deadline.catch(() => undefined);
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

async function driveMutation<T>(
  mutation: MutationPromise<T>,
  first: MutationAttempt,
  ownership: AbortSignal,
  timeoutMs: number,
  acceptedAttempt: AcceptedAttempt,
): Promise<T> {
  try {
    const value = await first.wait(mutation);
    first.accept();
    return value;
  } catch (error) {
    first.dispose();
    if (!first.deadlineExpired() || ownership.aborted) throw error;
  }

  const retry = createAttempt(ownership, timeoutMs, acceptedAttempt);
  let replay: MutationPromise<T>;
  try {
    replay = mutation.retry({ signal: retry.signal });
  } catch (error) {
    retry.dispose();
    throw new MutationLookupError(error);
  }
  try {
    const value = await retry.wait(replay);
    retry.accept();
    return value;
  } catch (error) {
    retry.dispose();
    if (retry.deadlineExpired() && !ownership.aborted) throw new MutationLookupError(error);
    throw error;
  }
}

export function createMutationSettler(config: MutationSettlerConfig = {}): MutationSettler {
  const acceptedAttempt = config.acceptedAttempt ?? "released";
  const lifetime = new AbortController();

  return {
    settle<T>(
      open: (signal: AbortSignal) => MutationPromise<T>,
      options: MutationSettleOptions<T> = {},
    ): Promise<T> {
      if (lifetime.signal.aborted) return Promise.reject(lifetime.signal.reason);
      const timeoutMs = options.timeoutMs ?? MUTATION_ATTEMPT_TIMEOUT_MS;
      if (
        !Number.isInteger(timeoutMs) ||
        timeoutMs <= 0 ||
        timeoutMs > MAXIMUM_MUTATION_ATTEMPT_TIMEOUT_MS
      )
        return Promise.reject(
          new RangeError("mutation attempt timeout must be within the platform timer range"),
        );
      const ownership = options.parent
        ? AbortSignal.any([options.parent, lifetime.signal])
        : lifetime.signal;
      const first = createAttempt(ownership, timeoutMs, acceptedAttempt);
      let mutation: MutationPromise<T>;
      try {
        mutation = open(first.signal);
      } catch (error) {
        first.dispose();
        return Promise.reject(error);
      }
      const settlement = driveMutation(mutation, first, ownership, timeoutMs, acceptedAttempt);
      return projectMutation(settlement, mutation, options.onCommand);
    },
    dispose(): void {
      if (!lifetime.signal.aborted) lifetime.abort(new MutationSettlementClosedError());
    },
  };
}
