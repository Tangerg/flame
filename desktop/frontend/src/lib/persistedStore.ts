import type { ZodType } from "zod";

/**
 * `version` here means "storage written before this is DROPPED" — there are no migrations
 * (CLAUDE.md §3), and rehydration reads `undefined` as "boot from defaults".
 *
 * Declared rather than omitted: zustand treats an ABSENT `migrate` as a failure, logging at
 * error level and leaving storage un-rewritten, so the stale payload and its error survive
 * every boot. The cast is what it costs to say "produces no state".
 */
export const discardOlderVersions = () => undefined as never;

/**
 * localStorage is a trust boundary: a payload that fails to parse boots the defaults and
 * says so once. `project` is for stores whose durable shape differs from their live one —
 * the dock persists session scopes as tuples and rebuilds a Map.
 */
export function rehydrateOrDefault<Persisted, Restored extends object = Persisted & object>(
  storageKey: string,
  schema: ZodType<Persisted>,
  project: (persisted: Persisted) => Restored = (persisted) => persisted as unknown as Restored,
) {
  // Generic in the STATE, not closed over it: zustand hands `merge` the store's full state,
  // which is wider than what was persisted. Pinning the state here would infer it from
  // `project`'s return and reject every store whose live shape carries actions.
  return <State extends Restored>(persisted: unknown, current: State): State => {
    if (persisted === undefined) return current;
    const parsed = schema.safeParse(persisted);
    if (!parsed.success) {
      console.warn(`[${storageKey}] discarding corrupted payload:`, parsed.error.issues);
      return current;
    }
    return { ...current, ...project(parsed.data) };
  };
}
