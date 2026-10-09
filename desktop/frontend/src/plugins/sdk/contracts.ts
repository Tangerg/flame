import { extensionPoint, normalizePlainRecord } from "dougong";
import type { ExtensionKeying, ExtensionPoint } from "./types/extensions";

export interface Contribution<T> {
  readonly key: string;
  readonly plugin: string;
  readonly item: T;
}

const taken = new Set<string>();
const specFields: ReadonlySet<string> = new Set(["id", "keying", "keyOf", "normalizeKey"]);

interface ExtensionPointSpec<T> {
  readonly id: string;
  readonly keying: ExtensionKeying;
  readonly keyOf?: (item: T) => string;
  readonly normalizeKey?: (key: string) => string;
}

export function defineExtensionPoint<T>(spec: ExtensionPointSpec<T>): ExtensionPoint<T> {
  const declaration = normalizePlainRecord(spec, "flame extension point", { fields: specFields });
  if (declaration.keying !== "single" && declaration.keying !== "multi")
    throw new TypeError("extension keying must be single or multi");
  for (const callback of [declaration.keyOf, declaration.normalizeKey]) {
    if (callback !== undefined && typeof callback !== "function")
      throw new TypeError("extension key policy must be a function");
  }
  if (taken.has(declaration.id)) {
    throw new Error(`Extension point "${declaration.id}" is already defined`);
  }
  const point = Object.freeze({
    ...declaration,
    token: extensionPoint<Contribution<T>>(declaration.id),
  });
  taken.add(point.id);
  return point;
}
