import { isErrorType, RpcProtocolError, RpcTransportError } from "./errors";

const MILLISECONDS_PER_SECOND = 1_000;

export interface MutationAttemptOptions {
  signal?: AbortSignal;
}

export interface MutationPromise<T> extends Promise<T> {
  readonly idempotencyKey: string;
  retry(options?: MutationAttemptOptions): MutationPromise<T>;
}

type MutationExecution<T> = (idempotencyKey: string, options: MutationAttemptOptions) => Promise<T>;

function retryableTransportFailure(error: unknown): error is RpcTransportError {
  if (!(error instanceof RpcTransportError)) return false;
  return error.status === undefined || error.status === 408 || (error.status ?? 0) >= 500;
}

export function mutationSettlementIsUnknown(error: unknown): boolean {
  if (error instanceof RpcProtocolError) return true;
  if (retryableTransportFailure(error)) return true;
  return isErrorType(error, "idempotency_in_progress");
}

function abortReason(signal: AbortSignal): unknown {
  return signal.reason ?? new DOMException("The operation was aborted", "AbortError");
}

async function waitForReplay(seconds: number, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) throw abortReason(signal);
  await new Promise<void>((resolve, reject) => {
    const finish = () => {
      signal?.removeEventListener("abort", abort);
      resolve();
    };
    const abort = () => {
      clearTimeout(timer);
      reject(abortReason(signal!));
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
      return await execute(idempotencyKey, options);
    } catch (error) {
      if (options.signal?.aborted) throw error;
      if (!replayedTransportFailure && retryableTransportFailure(error)) {
        replayedTransportFailure = true;
        continue;
      }
      if (!waitedForInProgress && isErrorType(error, "idempotency_in_progress")) {
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
  idempotencyKey: string = crypto.randomUUID(),
  options: MutationAttemptOptions = {},
): MutationPromise<T> {
  const promise = Promise.resolve().then(() => settleMutation(execute, idempotencyKey, options));
  return Object.defineProperties(promise, {
    idempotencyKey: { enumerable: true, value: idempotencyKey },
    retry: {
      enumerable: true,
      value: (retryOptions: MutationAttemptOptions = options) =>
        createMutationPromise(execute, idempotencyKey, retryOptions),
    },
  }) as MutationPromise<T>;
}
