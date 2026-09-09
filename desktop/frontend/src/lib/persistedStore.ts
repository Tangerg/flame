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

/**
 * The data half of a store's state — what `persist` writes when nothing narrows it.
 *
 * A store's live state carries its actions too, and a schema must not name those. Filtering
 * by "is this key a function" is what lets one rule serve a store that splits state from
 * actions and one that declares them together.
 */
type DataKeys<State> = {
  [K in keyof State]-?: State[K] extends (...args: never[]) => unknown ? never : K;
}[keyof State];

/**
 * Assert at COMPILE time that a schema names exactly what its store persists.
 *
 * `rehydrateOrDefault` parses through a Zod object, and a Zod object STRIPS keys it does not
 * name. So a field added to a store and not to its schema is written to localStorage and
 * dropped on the next boot: the user sets it, restarts, and it is gone, with nothing logged.
 * The schema and the state are one list written twice, and this is what holds them equal.
 *
 * Only one of the two directions was ever silent, and it is the one this adds. A schema WIDER
 * than its state already fails: `rehydrateOrDefault` constrains `State extends Restored`, so
 * a key the state has not is rejected at the `merge` line. A schema NARROWER than its state
 * satisfies that constraint perfectly — the state extends a subset — which is why a dropped
 * preference compiled, shipped, and lost the user's setting on restart. Both are checked here
 * anyway, so the rule reads as one statement, and each resolves to the offending KEYS rather
 * than to `false` so the type error says which.
 *
 * Declare it beside the schema:
 * `const _paired: Paired<MyState, z.infer<typeof mySchema>> = true;`
 *
 * Not for a store with `partialize` — that one deliberately persists a subset, and its round
 * trip is the check (see `agentSessionStore.test.ts`).
 */
export type Paired<State, Persisted> = [DataKeys<State>] extends [keyof Persisted]
  ? [keyof Persisted] extends [DataKeys<State>]
    ? true
    : { schemaNamesWhatTheStateHasNot: Exclude<keyof Persisted, DataKeys<State>> }
  : { stateHasWhatTheSchemaWillStrip: Exclude<DataKeys<State>, keyof Persisted> };
