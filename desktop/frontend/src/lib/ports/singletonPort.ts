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
 * Not a second contract system. dougong owns every capability whose AVAILABILITY has to be
 * ordered — a plugin that reads another context during its own setup declares
 * `requires: { … }` on a `service()` token, and the Host resolves the graph, starts the
 * provider first and rolls the whole installation back if it fails. Reach for a Service
 * whenever the question is "has the thing that answers this been installed yet".
 *
 * This is the other case: a within-context adapter slot read from a React render or an
 * event handler, long after startup settled, where the answer is always yes. The Host can
 * answer those too (`Host.get`), but making every one of them a Service would put a
 * contract graph between a component and its own context's store, and would make a unit
 * test install a Host to swap one function.
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
