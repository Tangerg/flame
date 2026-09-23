import { useSyncExternalStore } from "react";
import type { ContributionView, Host } from "dougong";
import type { Contribution } from "./contracts";
import type { ExtensionPoint } from "./types/extensions";

const NOTHING: ReadonlyArray<Contribution<never>> = Object.freeze([]);
const EMPTY_NAMES: ReadonlyArray<string> = Object.freeze([]);

let host: Host | undefined;
let names: ReadonlyArray<string> = EMPTY_NAMES;
const listeners = new Set<() => void>();
const pluginNamesByHost = new WeakMap<Host, Set<string>>();

let views = new Map<string, ContributionView<Contribution<unknown>>>();
let releases: Array<() => void> = [];
let entries = new Map<string, ReadonlyArray<Contribution<unknown>>>();

function announce(): void {
  names = Object.freeze([...(host ? pluginNamesFor(host) : [])].sort());
  for (const listener of [...listeners]) listener();
}

function pluginNamesFor(owner: Host): Set<string> {
  const existing = pluginNamesByHost.get(owner);
  if (existing) return existing;
  const created = new Set<string>();
  pluginNamesByHost.set(owner, created);
  return created;
}

function retractViews(): void {
  for (const release of releases) release();
  releases = [];
  views = new Map();
  entries = new Map();
}

export function publishKernel(next: Host): void {
  retractViews();
  host = next;
  announce();
}

export function retractKernel(owner: Host): boolean {
  if (host !== owner) return false;
  retractViews();
  host = undefined;
  announce();
  return true;
}

export function publishedKernel(): Host | undefined {
  return host;
}

export function trackInstalledPlugin(owner: Host, name: string): void {
  pluginNamesFor(owner).add(name);
  if (host === owner) announce();
}

function viewOf<T>(point: ExtensionPoint<T>): ContributionView<Contribution<T>> | undefined {
  const owner = host;
  if (!owner) return undefined;
  const cached = views.get(point.id);
  if (cached) return cached as ContributionView<Contribution<T>>;
  const view = owner.contributions(point.token);
  views.set(point.id, view as ContributionView<Contribution<unknown>>);
  const subscription = view.subscribe(() => {
    if (host !== owner) return;
    entries.delete(point.id);
    announce();
  });
  releases.push(() => subscription.dispose());
  return view;
}

function sortKey(entry: Contribution<unknown>): number {
  const own = (entry.item as { order?: number } | null)?.order;
  return own ?? entry.order ?? 100;
}

function resolve<T>(
  point: ExtensionPoint<T>,
  view: ContributionView<Contribution<T>>,
): ReadonlyArray<Contribution<T>> {
  const all = [...view.get().values()];
  const kept =
    point.keying === "multi"
      ? all
      : [
          ...all
            .reduce((byKey, e) => byKey.set(e.key, e), new Map<string, Contribution<T>>())
            .values(),
        ];
  return kept.sort((a, b) => sortKey(a) - sortKey(b));
}

export function contributionsTo<T>(point: ExtensionPoint<T>): ReadonlyArray<Contribution<T>> {
  const cached = entries.get(point.id);
  if (cached) return cached as ReadonlyArray<Contribution<T>>;
  const view = viewOf(point);
  const resolved = view ? resolve(point, view) : NOTHING;
  entries.set(point.id, resolved as ReadonlyArray<Contribution<unknown>>);
  return resolved as ReadonlyArray<Contribution<T>>;
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

export function subscribeContributions(listener: () => void): () => void {
  return subscribe(listener);
}

function installedSnapshot(): ReadonlyArray<string> {
  return names;
}

export function useInstalledPlugins(): ReadonlyArray<string> {
  return useSyncExternalStore(subscribe, installedSnapshot, () => EMPTY_NAMES);
}

export function useContributions<T>(point: ExtensionPoint<T>): ReadonlyArray<Contribution<T>> {
  return useSyncExternalStore(
    subscribe,
    () => contributionsTo(point),
    () => NOTHING as ReadonlyArray<Contribution<T>>,
  );
}
