// SINGLE-CONSUMER: `iterator()` returns a fresh object over a SHARED queue, so two
// iterators race for buffered values.
//
//   push  — synchronous, for intentionally unbounded channels; throws on a bounded one
//           rather than silently violating capacity.
//   send  — applies producer backpressure; resolves false when close wins first.
//   close / fail — drain pending values first, then terminate. Idempotent.

export interface PushPullChannel<T> {
  /** Attempt synchronous delivery without exceeding capacity. */
  tryPush(value: T): boolean;
  push(value: T): void;
  send(value: T): Promise<boolean>;
  /** Idempotent. */
  close(): void;
  /** Idempotent. */
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
