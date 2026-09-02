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
 * Dependency inversion INSIDE one bounded context: the application declares an interface,
 * that context's own adapter installs the implementation, and both ship in the same plugin.
 * There is no ordering question, because the `setup` that installs the adapter is the same
 * one that will later be read from.
 *
 * The other mechanism is a dougong Service (`<context>/public/services.ts`), and it answers
 * a different question — "has the OTHER installable started yet". Its whole value is the
 * contract graph: `requires`, start order, rollback. Every one of these slots is read only
 * by its own context, so a Service here would mint a token provided and required by the
 * same plugin: a graph with no edges.
 *
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
