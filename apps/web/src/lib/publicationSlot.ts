export interface PublicationSlot<T extends object> {
  publish(next: T, retire: (predecessor: T) => void): void;
  current(): T | null;
  owns(candidate: T): boolean;
  withdraw(candidate: T): boolean;
}

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
