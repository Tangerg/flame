/** A foreign async operation lost the generation that was allowed to commit. */
export const ASYNC_OWNERSHIP_RETIRED = Symbol("async-ownership.retired");

/**
 * Observe a dependency that may ignore AbortSignal without allowing it to
 * retain generation ownership. Late rejection remains handled; a late value
 * can be explicitly retired by its consumer-owned disposer.
 */
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

/** Observe ordinary microtask settlement, but never let a foreign promise hold
 * teardown beyond the next task. The promise remains observed after timeout. */
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

/** Best-effort retirement for a foreign async iterator. Cooperative iterators
 * join immediately; a broken return() remains observed without blocking the
 * successor generation. */
export async function disposeAsyncIterator<T>(iterator: AsyncIterator<T>): Promise<void> {
  try {
    const closing = iterator.return?.();
    if (closing) await settleWithinNextTask(Promise.resolve(closing));
  } catch {
    // The owner's abort signal remains the authoritative teardown path.
  }
}

/** The same retirement for a source that has not been opened yet. Asking a foreign iterable
 *  for its iterator can itself throw, which is a second failure the caller can do nothing
 *  about — and, like the first, one the owner's abort already fences. */
export async function disposeAsyncIterable<T>(iterable: AsyncIterable<T>): Promise<void> {
  let iterator: AsyncIterator<T>;
  try {
    iterator = iterable[Symbol.asyncIterator]();
  } catch {
    return;
  }
  await disposeAsyncIterator(iterator);
}
