import { useMemo } from "react";
import type { Contribution } from "../contracts";
import { contributionsTo, useContributions } from "../kernel";
import type { ExtensionPoint } from "../types/extensions";

type Entries<T> = ReadonlyArray<Contribution<T>>;

function derived<T, V>(build: (entries: Entries<T>) => V): (entries: Entries<T>) => V {
  const cache = new WeakMap<object, V>();
  return (entries) => {
    const cached = cache.get(entries);
    if (cached !== undefined) return cached;
    const value = build(entries);
    cache.set(entries, value);
    return value;
  };
}

const itemsOf = derived(<T>(entries: Entries<T>) => entries.map((e) => e.item));

const byKey = derived(<T>(entries: Entries<T>) => new Map(entries.map((e) => [e.key, e] as const)));

function normalized<T>(point: ExtensionPoint<T>, key: string): string {
  return point.normalizeKey ? point.normalizeKey(key) : key;
}

export interface ExtensionEntry<T> {
  key: string;
  item: T;
}

export function lookupExtensionPoint<T>(point: ExtensionPoint<T>): T[] {
  return itemsOf(contributionsTo(point)) as T[];
}

export function useExtensionPoint<T>(point: ExtensionPoint<T>): T[] {
  const entries = useContributions(point);
  return useMemo(() => itemsOf(entries) as T[], [entries]);
}

export function useExtensionEntries<T>(point: ExtensionPoint<T>): Array<ExtensionEntry<T>> {
  const entries = useContributions(point);
  return useMemo(() => entries.map((e) => ({ key: e.key, item: e.item })), [entries]);
}

export function lookupExtensionByKey<T>(point: ExtensionPoint<T>, key: string): T | undefined {
  return byKey(contributionsTo(point)).get(normalized(point, key))?.item;
}

export function useExtensionByKey<T>(point: ExtensionPoint<T>, key: string): T | undefined {
  const entries = useContributions(point);
  const wanted = normalized(point, key);
  return useMemo(() => byKey(entries).get(wanted)?.item, [entries, wanted]);
}

export function lookupExtensionOwner<T>(point: ExtensionPoint<T>, key: string): string | undefined {
  return byKey(contributionsTo(point)).get(normalized(point, key))?.plugin;
}

export function createPointSubIndex<I, V>(
  extract: (item: I, pluginName: string) => { key: string; value: V },
): (entries: Entries<I>) => ReadonlyMap<string, V[]> {
  return derived((entries: Entries<I>) => {
    const index = new Map<string, V[]>();
    for (const entry of entries) {
      const { key, value } = extract(entry.item, entry.plugin);
      const bucket = index.get(key);
      if (bucket) bucket.push(value);
      else index.set(key, [value]);
    }
    return index;
  });
}
