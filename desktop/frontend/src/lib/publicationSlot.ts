export interface PublicationSlot<T extends object> {
  /** Publish first, then synchronously retire the exact predecessor. */
  publish(next: T, retire: (predecessor: T) => void): void;
  current(): T | null;
  owns(candidate: T): boolean;
  /** Withdraw only the exact object being disposed. */
  withdraw(candidate: T): boolean;
}

/**
 * The process-local identity primitive shared by replaceable application owners. It owns no
 * tasks, events, caches or business state — only the successor-first publication
 * linearization point and the exact stale-disposer check.
 */
export function createPublicationSlot<T extends object>(): PublicationSlot<T> {
  let current: T | null = null;

  return {
    publish(next, retire) {
      const predecessor = current;
      current = next;
      if (predecessor) retire(predecessor);
    },
    current: () => current,
    owns: (candidate) => current === candidate,
    withdraw(candidate) {
      if (current !== candidate) return false;
      current = null;
      return true;
    },
  };
}
