// The token carries a `Contribution<T>` ENVELOPE, not the bare item: Core keys by OWNER — a
// key that changes on reinstall and that no consumer can construct — so the domain key
// travels inside the value, and `order` with it, since Core's view is unordered on purpose.
// `keying` is a READ policy only, so an overridden contribution comes BACK when the
// overriding plugin unloads.

import { extensionPoint } from "dougong";
import type { ExtensionKeying, ExtensionPoint } from "./types/extensions";

/**
 * `key` is the domain key `lookupExtensionByKey` matches on, `plugin` the owner kept for
 * error attribution, `order` the contribute-time sort hint — outranked by an `order` the
 * contributed value carries itself.
 */
export interface Contribution<T> {
  readonly key: string;
  readonly order: number | undefined;
  readonly plugin: string;
  readonly item: T;
}

// Ids already taken. Two points under one id would silently share a store, and
// the second definition would read the first's contributions as its own.
const taken = new Set<string>();

interface ExtensionPointSpec<T> {
  readonly id: string;
  readonly keying: ExtensionKeying;
  readonly keyOf?: (item: T) => string;
  readonly normalizeKey?: (key: string) => string;
}

/**
 * Share the returned const between the contributing and consuming plugins: it carries the
 * element type plus the Contract token the kernel routes on, which is what makes
 * `contribute` type-check its item and the read selectors come back typed.
 */
export function defineExtensionPoint<T>(spec: ExtensionPointSpec<T>): ExtensionPoint<T> {
  if (taken.has(spec.id)) {
    throw new Error(`Extension point "${spec.id}" is already defined`);
  }
  const point: ExtensionPoint<T> = {
    ...spec,
    token: extensionPoint<Contribution<T>>(spec.id),
  };
  taken.add(spec.id);
  return point;
}
