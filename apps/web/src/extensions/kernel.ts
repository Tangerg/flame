import { useSyncExternalStore } from "react";
import { disposeSymbol, type ContributionView, type Host, type HostSnapshot } from "dougong";
import { SnapshotPublisher } from "dougong/advanced";
import { retainContribution } from "@/extensions/contributionScope";
import { type Contribution, type PublishedContribution } from "@/extensions/contracts";
import { type ExtensionPoint } from "@/extensions/types/extensionPoint";
import { reportPluginError } from "@/extensions/errors";
const NOTHING: ReadonlyArray<Contribution<never>> = Object.freeze([]);

const EMPTY_NAMES: ReadonlyArray<string> = Object.freeze([]);

let host: Host | undefined;

let installations: { source: HostSnapshot; names: ReadonlyArray<string> } | undefined;

const publication = new SnapshotPublisher(
  () => host,
  (error) => reportPluginError("kernel", "events", error),
);

let views = new Map<string, ContributionView<PublishedContribution<unknown>>>();

let releases: Array<() => void> = [];

let entries = new Map<string, ReadonlyArray<Contribution<unknown>>>();
let borrowed = new Map<string, Array<() => void>>();
const pending = new WeakMap<Host, Set<Promise<void>>>();

function releasePoint(id: string): void {
  const release = borrowed.get(id);
  borrowed.delete(id);
  entries.delete(id);
  for (const end of release ?? []) end();
}

function announce(): void {
  publication.invalidate();
}

function retractViews(): void {
  for (const id of borrowed.keys()) releasePoint(id);
  for (const release of releases) release();
  releases = [];
  views = new Map();
  entries = new Map();
  installations = undefined;
}

export function publishKernel(next: Host): void {
  retractViews();
  host = next;
  installedSnapshot();
  const subscription = next.diagnostics.subscribe(() => {
    const previous = installations?.names;
    if (installedSnapshot() !== previous) announce();
  });
  releases.push(() => subscription[disposeSymbol]());
  announce();
}

export function retractKernel(owner: Host): Promise<boolean> {
  if (host !== owner) {
    return Promise.all(pending.get(owner) ?? []).then(() => false);
  }
  retractViews();
  host = undefined;
  announce();
  const readers = pending.get(owner);
  return readers?.size ? Promise.all(readers).then(() => true) : Promise.resolve(true);
}

export function publishedKernel(): Host | undefined {
  return publication.view.get();
}

function viewOf<T>(
  point: ExtensionPoint<T>,
): ContributionView<PublishedContribution<T>> | undefined {
  const owner = host;
  if (!owner) return undefined;
  const cached = views.get(point.id);
  if (cached) return cached as ContributionView<PublishedContribution<T>>;
  const view = owner.contributions(point.token);
  views.set(point.id, view as ContributionView<PublishedContribution<unknown>>);
  const subscription = view.subscribe(() => {
    if (host !== owner) return;
    releasePoint(point.id);
    announce();
  });
  releases.push(() => subscription[disposeSymbol]());
  return view;
}

function sortKey(entry: Contribution<unknown>): number {
  return (entry.item as { order?: number } | null)?.order ?? 100;
}

function resolve<T>(
  point: ExtensionPoint<T>,
  view: ContributionView<PublishedContribution<T>>,
): ReadonlyArray<Contribution<T>> {
  const owner = host;
  if (!owner) return NOTHING;
  const all: Contribution<T>[] = [];
  const active = pending.get(owner) ?? new Set<Promise<void>>();
  pending.set(owner, active);
  const close: Array<() => void> = [];
  for (const id of view.get()) {
    const lease = retainContribution(view, id);
    close.push(lease.release);
    active.add(lease.settled);
    void lease.settled.then(
      () => active.delete(lease.settled),
      (error) => {
        active.delete(lease.settled);
        reportPluginError("kernel", "events", error);
      },
    );
    if (lease.value) all.push(lease.value);
  }
  borrowed.set(point.id, close);
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
  return () => subscription[disposeSymbol]();
}

export function subscribeContributions(listener: () => void): () => void {
  return subscribe(listener);
}

function installedSnapshot(): ReadonlyArray<string> {
  const source = host?.diagnostics.get();
  if (!source) return EMPTY_NAMES;
  if (installations?.source !== source) {
    const names = [...new Set(source.installations.map((entry) => entry.pluginName))].sort();
    const prior = installations?.names;
    installations = {
      source,
      names:
        prior?.length === names.length && prior.every((name, index) => name === names[index])
          ? prior
          : Object.freeze(names),
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
