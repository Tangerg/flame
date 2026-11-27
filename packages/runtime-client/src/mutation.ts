import {
  isErrorType,
  RpcError,
  RpcProtocolError,
  RpcTransportError,
  RpcCancellationError,
} from "./errors";

const MILLISECONDS_PER_SECOND = 1_000;

export interface MutationAttemptOptions {
  signal?: AbortSignal;
}

export interface MutationPromise<T> extends Promise<T> {
  // A mutation handle is issued only after its original command is staged.
  readonly idempotencyKey: string;
  retry(options?: MutationAttemptOptions): MutationPromise<T>;
}

export function mutationHandle<T>(
  promise: Promise<T>,
  idempotencyKey: string,
  retry: MutationPromise<T>["retry"],
): MutationPromise<T> {
  return Object.defineProperties(promise, {
    idempotencyKey: { enumerable: true, value: idempotencyKey },
    retry: { enumerable: true, value: retry },
  }) as MutationPromise<T>;
}

export function observedMutation<T>(idempotencyKey: string, result: T): MutationPromise<T> {
  const handle: MutationPromise<T> = mutationHandle(
    Promise.resolve(result),
    idempotencyKey,
    () => handle,
  );
  return handle;
}

type MutationExecution<T> = (idempotencyKey: string, options: MutationAttemptOptions) => Promise<T>;

export class MutationLookupError extends Error {
  override readonly name = "MutationLookupError";
  constructor(cause: unknown) {
    super("command acceptance lookup failed", { cause });
  }
}

function retryableTransportFailure(error: unknown): error is RpcTransportError {
  if (!(error instanceof RpcTransportError)) return false;
  return error.status === undefined || error.status === 408 || (error.status ?? 0) >= 500;
}

export function mutationSettlementIsUnknown(error: unknown): boolean {
  if (error instanceof RpcError && error.recordedOutcome) return false;
  if (error instanceof MutationLookupError) return true;
  if (error instanceof RpcProtocolError) return true;
  // HTTP admission failure cannot settle an earlier dispatch under this identity.
  if (error instanceof RpcTransportError) return true;
  // These responses do not settle the original command. Preserve its identity for
  // explicit recovery without expanding the separate automatic replay budget.
  return (
    isErrorType(error, "idempotency_in_progress") ||
    isErrorType(error, "internal_error") ||
    isErrorType(error, "idempotency_conflict") ||
    isErrorType(error, "idempotency_store_mismatch")
  );
}

async function waitForReplay(seconds: number, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) throw new RpcCancellationError(signal.reason);
  await new Promise<void>((resolve, reject) => {
    const finish = () => {
      signal?.removeEventListener("abort", abort);
      resolve();
    };
    const abort = () => {
      clearTimeout(timer);
      reject(new RpcCancellationError(signal?.reason));
    };
    const timer = setTimeout(finish, seconds * MILLISECONDS_PER_SECOND);
    signal?.addEventListener("abort", abort, { once: true });
  });
}

async function settleMutation<T>(
  execute: MutationExecution<T>,
  idempotencyKey: string,
  options: MutationAttemptOptions,
): Promise<T> {
  let replayedTransportFailure = false;
  let waitedForInProgress = false;
  for (;;) {
    try {
      if (options.signal?.aborted) throw new RpcCancellationError(options.signal.reason);
      return await execute(idempotencyKey, options);
    } catch (error) {
      if (options.signal?.aborted) throw error;
      if (!replayedTransportFailure && retryableTransportFailure(error)) {
        replayedTransportFailure = true;
        continue;
      }
      if (
        !waitedForInProgress &&
        isErrorType(error, "idempotency_in_progress") &&
        !error.recordedOutcome
      ) {
        waitedForInProgress = true;
        await waitForReplay(error.data.retryAfterSeconds, options.signal);
        continue;
      }
      throw error;
    }
  }
}

export function createMutationPromise<T>(
  execute: MutationExecution<T>,
  idempotencyKey: string,
  options: MutationAttemptOptions = {},
): MutationPromise<T> {
  const promise = Promise.resolve().then(() => settleMutation(execute, idempotencyKey, options));
  return mutationHandle(promise, idempotencyKey, (retryOptions = options) =>
    createMutationPromise(execute, idempotencyKey, retryOptions),
  );
}
