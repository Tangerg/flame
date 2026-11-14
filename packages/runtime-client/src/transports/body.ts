export function bodyCancellation(reader: ReadableStreamDefaultReader<Uint8Array>) {
  let cancellation: Promise<void> | undefined;
  return {
    cancel(reason?: unknown): Promise<void> {
      cancellation ??= Promise.resolve().then(() => reader.cancel(reason));
      return cancellation;
    },
    get started(): boolean {
      return cancellation !== undefined;
    },
  };
}

export function isPrimaryFailure(primary: unknown, candidate: unknown): boolean {
  return (
    Object.is(primary, candidate) ||
    (primary instanceof Error &&
      ((Object.hasOwn(primary, "cause") && Object.is(primary.cause, candidate)) ||
        (primary.cause instanceof AggregateError && primary.cause.errors.includes(candidate))))
  );
}

export function retainCleanupFailure(primary: unknown, cleanup: unknown): unknown {
  if (isPrimaryFailure(primary, cleanup)) return primary;
  if (!(primary instanceof Error))
    return new AggregateError(
      [primary, cleanup],
      "response cleanup failed after its primary failure",
      { cause: primary },
    );
  primary.cause = Object.hasOwn(primary, "cause")
    ? new AggregateError(
        [primary.cause, cleanup],
        "response cleanup failed after its primary failure",
      )
    : cleanup;
  return primary;
}

export function isExpectedAbort(error: unknown, signal: AbortSignal): boolean {
  if (!signal.aborted) return false;
  if (Object.is(error, signal.reason)) return true;
  if (!(error instanceof Error)) return false;
  if (error.name === "AbortError") return true;
  return error.cause !== undefined && isExpectedAbort(error.cause, signal);
}
