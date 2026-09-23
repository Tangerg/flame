import { createPublicationSlot } from "../publicationSlot";

export interface SingletonPort<T> {
  configure(next: T): () => void;
  get(): T;
  peek(): T | null;
}

export function createSingletonPort<T>(notConfiguredMessage: string): SingletonPort<T> {
  const slot = createPublicationSlot<{ value: T }>();

  return {
    configure(next) {
      const published = { value: next };
      slot.publish(published, () => undefined);
      let disposed = false;
      return () => {
        if (disposed) return;
        disposed = true;
        slot.withdraw(published);
      };
    },
    get() {
      const current = slot.current();
      if (!current) throw new Error(notConfiguredMessage);
      return current.value;
    },
    peek() {
      return slot.current()?.value ?? null;
    },
  };
}
