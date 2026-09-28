import { useSyncExternalStore } from "react";
import { SnapshotPublisher, type ContributionView, type Host, type HostSnapshot } from "dougong";
import type { Contribution } from "./contracts";
import type { ExtensionPoint } from "./types/extensions";
import { reportPluginError } from "./errors";

const NOTHING: ReadonlyArray<Contribution<never>> = Object.freeze([]);
const EMPTY_NAMES: ReadonlyArray<string> = Object.freeze([]);

let host: Host | undefined;
let installations: { source: HostSnapshot; names: ReadonlyArray<string> } | undefined;
const publication = new SnapshotPublisher(
  () => host,
  (error) => reportPluginError("kernel", "events", error),
);

let views = new Map<string, ContributionView<Contribution<unknown>>>();
let releases: Array<() => void> = [];
let entries = new Map<string, ReadonlyArray<Contribution<unknown>>>();

function announce(): void {
  publication.invalidate();
}

function retractViews(): void {
  for (const release of releases) release();
  releases = [];
  views = new Map();
  entries = new Map();
  installations = undefined;
}

export function publishKernel(next: Host): void {
  retractViews();
  host = next;
  const subscription = next.diagnostics.subscribe(announce);
  releases.push(() => subscription.dispose());
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
  return publication.view.get();
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
  const subscription = publication.view.subscribe(onChange);
  return () => subscription.dispose();
}

export function subscribeContributions(listener: () => void): () => void {
  return subscribe(listener);
}

function installedSnapshot(): ReadonlyArray<string> {
  const source = host?.diagnostics.get();
  if (!source) return EMPTY_NAMES;
  if (installations?.source !== source) {
    installations = {
      source,
      names: Object.freeze(
        [...new Set([...source.installations.values()].map((entry) => entry.pluginName))].sort(),
      ),
    };
  }
  return installations.names;
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
