import { extensionPoint } from "dougong";
import type { ExtensionKeying, ExtensionPoint } from "./types/extensions";

export interface Contribution<T> {
  readonly key: string;
  readonly order: number | undefined;
  readonly plugin: string;
  readonly item: T;
}

const taken = new Set<string>();

interface ExtensionPointSpec<T> {
  readonly id: string;
  readonly keying: ExtensionKeying;
  readonly keyOf?: (item: T) => string;
  readonly normalizeKey?: (key: string) => string;
}

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
