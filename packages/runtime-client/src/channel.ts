export interface PushPullChannel<T> {
  tryPush(value: T): boolean;
  push(value: T): void;
  send(value: T): Promise<boolean>;
  close(): void;
  cancel(): void;
  fail(error: unknown): void;
  readonly closed: boolean;
  iterator(): AsyncIterableIterator<T>;
}

export type PushPullChannelOptions = {
  capacity: number | "unbounded";
};

type ChannelTermination = { kind: "closed" | "canceled" } | { kind: "failed"; error: unknown };

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
  let state: { kind: "open" } | ChannelTermination = { kind: "open" };

  function tryPush(value: T): boolean {
    if (state.kind !== "open") return false;
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
    if (tryPush(value) || state.kind !== "open") return;
    throw new Error("bounded channel is full; use send() to apply producer backpressure");
  }

  function send(value: T): Promise<boolean> {
    if (state.kind !== "open") return Promise.resolve(false);
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
    while (state.kind === "open" && producers.length > 0 && buffer.length < capacity) {
      const producer = producers.shift()!;
      buffer.push(producer.value);
      producer.resolve(true);
    }
  }

  function terminate(outcome: ChannelTermination): void {
    const wasOpen = state.kind === "open";
    if (outcome.kind === "canceled") buffer.length = 0;
    else if (!wasOpen) return;

    state = outcome;
    if (!wasOpen) return;
    if (outcome.kind === "failed") {
      for (const producer of producers.splice(0)) producer.reject(outcome.error);
      for (const waiter of waiters.splice(0)) waiter.reject(outcome.error);
      return;
    }
    for (const producer of producers.splice(0)) producer.resolve(false);
    for (const waiter of waiters.splice(0)) {
      waiter.resolve({ value: undefined as never, done: true });
    }
  }

  return {
    tryPush,
    push,
    send,
    close: () => terminate({ kind: "closed" }),
    cancel: () => terminate({ kind: "canceled" }),
    fail: (error) => terminate({ kind: "failed", error }),
    get closed() {
      return state.kind !== "open";
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
          if (state.kind === "failed") throw state.error;
          if (state.kind !== "open") return { value: undefined as never, done: true };
          return new Promise<IteratorResult<T>>((resolve, reject) => {
            waiters.push({ resolve, reject });
          });
        },
        async return(): Promise<IteratorResult<T>> {
          terminate({ kind: "canceled" });
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}
