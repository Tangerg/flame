export interface PublicationSlot<T extends object> {
  /** Publish first, then synchronously retire the exact predecessor. */
  publish(next: T, retire: (predecessor: T) => void): void;
  current(): T | null;
  owns(candidate: T): boolean;
  /** Withdraw only the exact object being disposed. */
  withdraw(candidate: T): boolean;
}

/** Successor-first publication plus the exact stale-disposer check. Owns no tasks, events,
 *  caches or business state. */
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
