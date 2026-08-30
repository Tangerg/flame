/**
 * `version` here means "storage written before this is DROPPED" — there are no migrations
 * (CLAUDE.md §3), and each store's `merge` reads `undefined` as "boot from defaults".
 *
 * Declared rather than omitted: zustand treats an ABSENT `migrate` as a failure, logging at
 * error level and leaving storage un-rewritten, so the stale payload and its error survive
 * every boot. The cast is what it costs to say "produces no state".
 */
export const discardOlderVersions = () => undefined as never;
