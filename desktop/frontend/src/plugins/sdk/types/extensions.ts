// Unlike layout slots, which carry React components rendered by <Slot>, an extension point
// carries arbitrary typed DATA consumed programmatically. The kernel owns none of them.

import type { ExtensionPoint as ContractToken } from "dougong";
import type { Contribution } from "../contracts";

export type ExtensionKeying =
  // Last contributor of a key wins; the ones it shadows stay in the store and come back if
  // it unloads.
  | "single"
  // Every contribution stands on its own; nothing shadows anything.
  | "multi";

/**
 * A typed handle to an extension point, shared as a module const between the
 * plugin that contributes and the one that consumes — it re-adds the type
 * inference a raw string API would erase. The handle holds no state; the Host's
 * contribution store is the single source of truth.
 */
export interface ExtensionPoint<T> {
  readonly id: string;
  readonly keying: ExtensionKeying;
  readonly token: ContractToken<Contribution<T>>;
  /**
   * Derives the domain key for `single` points. Defaults to `item.id`; set it where the
   * key is something else (a tool fn name, a data-provider `key`, a content-block `kind`).
   */
  readonly keyOf?: (item: T) => string;
  /**
   * Applied on BOTH contribute and lookup, so registrations and lookups agree — e.g.
   * shortcuts fold "Cmd+K" and "mod+k" to one canonical combo.
   */
  readonly normalizeKey?: (key: string) => string;
}

export interface ExtensionContributionOptions {
  /**
   * Pass it so a same-name plugin RELOAD overwrites its prior contribution rather than
   * stacking a duplicate. Ignored by `single` points, which key via `keyOf`.
   */
  id?: string;
  /**
   * For `single` points whose key is not carried on the item. Takes precedence over the
   * point's `keyOf`; ignored by `multi` points.
   */
  key?: string;
  /** Lower comes first. Falls back to the item's own `order`. */
  order?: number;
}
