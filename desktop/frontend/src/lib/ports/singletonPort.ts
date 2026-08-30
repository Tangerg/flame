import { createPublicationSlot } from "../publicationSlot";

export interface SingletonPort<T> {
  configure(next: T): () => void;
  get(): T;
  /**
   * The adapter if installed, else null — for the callers that have a correct answer without
   * it. `get()` throws because most do not, and reading a port before its adapter exists is
   * a wiring bug; making the answerable callers catch that throw would hide the real ones.
   */
  peek(): T | null;
}

/**
 * Plugin reload can install a new adapter before an older cleanup runs, so a cleanup clears
 * ONLY the exact instance it installed and can never disconnect its successor.
 */
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
