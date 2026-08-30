// Push-pull async channel — single-consumer AsyncIterableIterator backed
// by an optional internal buffer + FIFO consumer/producer queues. The four sites that used
// to roll their own version (memory transport, http transport, run
// event stream, terminal/background stream) all delegate to this now.
//
// Contract:
//   - push(value) buffers OR resolves a waiting next() call.
//     It is the synchronous API for intentionally unbounded channels; on a
//     bounded channel it throws instead of silently violating capacity.
//   - send(value) applies producer backpressure. It resolves true only after
//     the value is buffered or accepted by next(), false if close wins first.
//     fail rejects blocked producers with the channel failure.
//   - close() drains pending values to the consumer and then returns
//     done=true. Idempotent.
//   - fail(error) drains pending values, then rejects iteration with that error.
//   - iterator() returns a fresh iterator object; the underlying queue
//     is shared, so callers must treat the channel as single-consumer
//     (multiple iterators would race for buffered values).
//   - iterator().return() closes the channel — used by `for await` to
//     clean up when the loop breaks out early.

export interface PushPullChannel<T> {
  /** Attempt synchronous delivery without exceeding capacity. */
  tryPush(value: T): boolean;
  push(value: T): void;
  send(value: T): Promise<boolean>;
  /** Close the channel. Idempotent. */
  close(): void;
  /** Terminate the channel with an error. Idempotent. */
  fail(error: unknown): void;
  readonly closed: boolean;
  iterator(): AsyncIterableIterator<T>;
}

export type PushPullChannelOptions = {
  /** Maximum values retained without a consumer. Zero is a rendezvous channel.
   *  `unbounded` must be chosen explicitly by an owner that accepts it. */
  capacity: number | "unbounded";
};

export function createPushPullChannel<T>(options: PushPullChannelOptions): PushPullChannel<T> {
  const capacity = options.capacity === "unbounded" ? Number.POSITIVE_INFINITY : options.capacity;
  if (capacity !== Number.POSITIVE_INFINITY && (!Number.isSafeInteger(capacity) || capacity < 0)) {
    throw new RangeError("channel capacity must be a non-negative safe integer");
  }
  const buffer: T[] = [];
  const waiters: Array<{
    resolve: (result: IteratorResult<T>) => void;
    reject: (error: unknown) => void;
  }> = [];
  const producers: Array<{
    value: T;
    resolve: (accepted: boolean) => void;
    reject: (error: unknown) => void;
  }> = [];
  let isClosed = false;
  let failure: unknown;
  let failed = false;

  function tryPush(value: T): boolean {
    if (isClosed) return false;
    const waiter = waiters.shift();
    if (waiter) {
      waiter.resolve({ value, done: false });
      return true;
    } else if (producers.length === 0 && buffer.length < capacity) {
      buffer.push(value);
      return true;
    }
    return false;
  }

  function push(value: T): void {
    if (tryPush(value) || isClosed) return;
    throw new Error("bounded channel is full; use send() to apply producer backpressure");
  }

  function send(value: T): Promise<boolean> {
    if (isClosed) return Promise.resolve(false);
    const waiter = waiters.shift();
    if (waiter) {
      waiter.resolve({ value, done: false });
      return Promise.resolve(true);
    }
    if (producers.length === 0 && buffer.length < capacity) {
      buffer.push(value);
      return Promise.resolve(true);
    }
    return new Promise<boolean>((resolve, reject) => {
      producers.push({ value, resolve, reject });
    });
  }

  function refillBuffer(): void {
    while (!isClosed && producers.length > 0 && buffer.length < capacity) {
      const producer = producers.shift()!;
      buffer.push(producer.value);
      producer.resolve(true);
    }
  }

  function close(): void {
    if (isClosed) return;
    isClosed = true;
    for (const producer of producers.splice(0)) producer.resolve(false);
    for (const waiter of waiters.splice(0)) {
      waiter.resolve({ value: undefined as never, done: true });
    }
  }

  function fail(error: unknown): void {
    if (isClosed) return;
    isClosed = true;
    failed = true;
    failure = error;
    for (const producer of producers.splice(0)) producer.reject(error);
    for (const waiter of waiters.splice(0)) waiter.reject(error);
  }

  return {
    tryPush,
    push,
    send,
    close,
    fail,
    get closed() {
      return isClosed;
    },
    iterator(): AsyncIterableIterator<T> {
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        async next(): Promise<IteratorResult<T>> {
          if (buffer.length > 0) {
            const value = buffer.shift()!;
            refillBuffer();
            return { value, done: false };
          }
          const producer = producers.shift();
          if (producer) {
            producer.resolve(true);
            return { value: producer.value, done: false };
          }
          if (failed) throw failure;
          if (isClosed) return { value: undefined as never, done: true };
          return new Promise<IteratorResult<T>>((resolve, reject) => {
            waiters.push({ resolve, reject });
          });
        },
        async return(): Promise<IteratorResult<T>> {
          close();
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}
