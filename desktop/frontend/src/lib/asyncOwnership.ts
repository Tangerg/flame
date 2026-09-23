export const ASYNC_OWNERSHIP_RETIRED = Symbol("async-ownership.retired");

export class GenerationRetiredError extends Error {
  override readonly name = "GenerationRetiredError";

  constructor(readonly owner: string) {
    super(`${owner}_retired`);
  }
}

export function wasGenerationRetired(error: unknown): boolean {
  return error instanceof GenerationRetiredError;
}

export function settleBeforeAbort<T>(
  operation: Promise<T>,
  signal: AbortSignal,
  disposeLateValue?: (value: T) => void,
): Promise<T | typeof ASYNC_OWNERSHIP_RETIRED> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", onAbort);
      resolve(ASYNC_OWNERSHIP_RETIRED);
    };
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });

    void operation.then(
      (value) => {
        if (settled) {
          disposeLateValue?.(value);
          return;
        }
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(value);
      },
      (error: unknown) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

export type NextTaskSettlement<T> =
  { status: "fulfilled"; value: T } | { status: "rejected" } | { status: "pending" };

export function settleWithinNextTask<T>(operation: Promise<T>): Promise<NextTaskSettlement<T>> {
  return new Promise((resolve) => {
    let settled = false;
    const finish = (result: NextTaskSettlement<T>) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(result);
    };
    const timer = setTimeout(() => finish({ status: "pending" }), 0);
    void operation.then(
      (value) => finish({ status: "fulfilled", value }),
      () => finish({ status: "rejected" }),
    );
  });
}

export async function disposeAsyncIterator<T>(iterator: AsyncIterator<T>): Promise<void> {
  try {
    const closing = iterator.return?.();
    if (closing) await settleWithinNextTask(Promise.resolve(closing));
  } catch {}
}

export async function disposeAsyncIterable<T>(iterable: AsyncIterable<T>): Promise<void> {
  let iterator: AsyncIterator<T>;
  try {
    iterator = iterable[Symbol.asyncIterator]();
  } catch {
    return;
  }
  await disposeAsyncIterator(iterator);
}
