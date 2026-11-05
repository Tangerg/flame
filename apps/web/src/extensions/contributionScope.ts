import {
  asyncDisposeSymbol,
  disposeSymbol,
  type ChildLifetime,
  type ContributionOperations,
  type LifetimeContext,
} from "dougong";
import { batchSnapshotNotifications, normalizePlainRecord } from "dougong/advanced";
import { ExactSequence } from "@/foundation/exactSequence";
import { type Contribution, type PublishedContribution } from "@/extensions/contracts";
import {
  type ExtensionContributionOptions,
  type ExtensionPoint,
} from "@/extensions/types/extensionPoint";

const mintedIds = new ExactSequence();
const contributionFields: ReadonlySet<string> = new Set(["id", "key"]);

function domainKey<T>(
  point: ExtensionPoint<T>,
  item: T,
  options: ExtensionContributionOptions | undefined,
): string {
  if (point.keying === "multi") return options?.id ?? `${point.id}#${mintedIds.issue()}`;
  const itemId =
    typeof item === "object" && item !== null && "id" in item && typeof item.id === "string"
      ? item.id
      : undefined;
  const key = options?.key ?? point.keyOf?.(item) ?? itemId;
  if (!key) throw new Error(`single extension point "${point.id}" requires a non-empty key`);
  return point.normalizeKey ? point.normalizeKey(key) : key;
}

export interface ExtensionContribution<T> {
  update(value: T): void;
  [Symbol.dispose](): void;
}

export class ContributionLease {
  readonly #release = Promise.withResolvers<void>();
  readonly #joined = Promise.withResolvers<void>();
  readonly untilReleased = this.#release.promise;
  readonly settled = this.#joined.promise;

  release(): void {
    this.#release.resolve();
  }

  join(borrow: Promise<unknown>): void {
    void borrow.then(() => this.#joined.resolve(), this.#joined.reject);
  }
}

export class ContributionScope {
  readonly #publications = new Set<{ [Symbol.dispose](): void }>();
  readonly #children = new Set<ContributionScope>();
  readonly #leases = new Set<ContributionLease>();
  readonly #operations: ContributionOperations;
  readonly #plugin: string;
  readonly #owned: ChildLifetime | undefined;
  readonly #parent: ContributionScope | undefined;
  #sealed = false;
  #retirement: Promise<void> | undefined;

  constructor(
    readonly lifetime: LifetimeContext,
    operations: ContributionOperations,
    plugin: string,
    owned?: ChildLifetime,
    parent?: ContributionScope,
  ) {
    this.#operations = operations;
    this.#plugin = plugin;
    this.#owned = owned;
    this.#parent = parent;
  }

  child(label: string): ContributionScope {
    this.assertOpen();
    const owned = this.lifetime.child(label);
    const child = new ContributionScope(owned, this.#operations, this.#plugin, owned, this);
    this.#children.add(child);
    return child;
  }

  add<T>(
    point: ExtensionPoint<T>,
    item: T,
    options?: ExtensionContributionOptions,
  ): ExtensionContribution<T> {
    this.assertOpen();
    const captured =
      options === undefined
        ? undefined
        : normalizePlainRecord(options, "flame contribution options", {
            fields: contributionFields,
          });
    const key = domainKey(point, item, captured);
    const publish = (value: T): PublishedContribution<T> => ({
      key,
      plugin: this.#plugin,
      item: value,
      retain: () => this.#retain(),
    });
    const contribution = this.#operations.add(point.token, key, publish(item), {
      lifetime: this.lifetime,
    });
    const registration: ExtensionContribution<T> = {
      update: (value) => {
        this.assertOpen();
        if (point.keying === "single" && domainKey(point, value, captured) !== key)
          throw new Error(`extension contribution "${point.id}" cannot change its key`);
        contribution.update(publish(value));
      },
      [disposeSymbol]: () => {
        contribution[disposeSymbol]();
        this.#publications.delete(registration);
      },
    };
    this.#publications.add(registration);
    return registration;
  }

  assertOpen(): void {
    this.lifetime.signal.throwIfAborted();
    if (this.#sealed) throw new Error("contribution scope is retiring");
  }

  #retain(): ContributionLease {
    this.assertOpen();
    const lease = new ContributionLease();
    this.#leases.add(lease);
    void lease.settled.then(
      () => this.#leases.delete(lease),
      () => this.#leases.delete(lease),
    );
    return lease;
  }

  #withdraw(): void {
    for (const child of this.#children) child.#withdraw();
    for (const publication of this.#publications) publication[disposeSymbol]();
    this.#publications.clear();
    for (const lease of this.#leases) lease.release();
  }

  #seal(): void {
    this.#sealed = true;
    for (const child of this.#children) child.#seal();
  }

  #readers(): Promise<void>[] {
    return [
      ...[...this.#leases].map((lease) => lease.settled),
      ...[...this.#children].flatMap((child) => child.#readers()),
    ];
  }

  [asyncDisposeSymbol](): Promise<void> {
    if (this.#retirement) return this.#retirement;
    if (!this.#owned) return Promise.reject(new Error("the host owns the root contribution scope"));
    const retirement = Promise.withResolvers<void>();
    this.#retirement = retirement.promise;
    this.#seal();
    // Withdraw the rendering projection before stopping its borrowed generation.
    // Dougong then joins the same borrows before releasing the underlying resources.
    batchSnapshotNotifications(() => this.#withdraw());
    const readers = this.#readers();
    const stopping = readers.length
      ? Promise.all(readers).then(() => this.#owned![asyncDisposeSymbol]())
      : this.#owned[asyncDisposeSymbol]();
    void stopping.then(() => {
      if (this.#parent) this.#parent.#children.delete(this);
      retirement.resolve();
    }, retirement.reject);
    return retirement.promise;
  }
}

export function retainContribution<T>(
  view: import("dougong").ContributionView<PublishedContribution<T>>,
  id: string,
): {
  readonly value?: Contribution<T>;
  readonly release: () => void;
  readonly settled: Promise<void>;
} {
  let lease: ContributionLease | undefined;
  let value: Contribution<T> | undefined;
  const borrow = view.use(id, (entry) => {
    lease = entry.retain();
    value = { key: entry.key, plugin: entry.plugin, item: entry.item };
    return lease.untilReleased;
  });
  if (lease) {
    const retained = lease;
    retained.join(borrow);
    return { value, release: () => retained.release(), settled: retained.settled };
  }
  return { release: () => {}, settled: borrow.then(() => undefined) };
}
