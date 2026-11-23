import {
  asyncDisposeSymbol,
  disposeSymbol,
  type ChildLifetime,
  type ContributionOperations,
  type LifetimeContext,
} from "dougong";
import { batchSnapshotNotifications, capturePlainRecord } from "dougong/advanced";
import { ExactSequence } from "@/foundation/exactSequence";
import { type Contribution } from "@/extensions/contracts";
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

interface ExtensionContribution<T> {
  update(value: T): void;
  [Symbol.dispose](): void;
}

export class ContributionScope {
  readonly #publications = new Set<{ [Symbol.dispose](): void }>();
  readonly #children = new Set<ContributionScope>();
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
        : capturePlainRecord(options, "flame contribution options", {
            fields: contributionFields,
          });
    const key = domainKey(point, item, captured);
    const publish = (value: T): Contribution<T> => ({ key, plugin: this.#plugin, item: value });
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

  #withdraw(): void {
    for (const child of this.#children) child.#withdraw();
    for (const publication of this.#publications) publication[disposeSymbol]();
    this.#publications.clear();
  }

  #seal(): void {
    this.#sealed = true;
    for (const child of this.#children) child.#seal();
  }

  [asyncDisposeSymbol](): Promise<void> {
    if (this.#retirement) return this.#retirement;
    if (!this.#owned) return Promise.reject(new Error("the host owns the root contribution scope"));
    const retirement = Promise.withResolvers<void>();
    this.#retirement = retirement.promise;
    this.#seal();
    // Withdrawal notifies the kernel, which disposes its leases on these
    // values before the owned lifetime stops.
    batchSnapshotNotifications(() => this.#withdraw());
    void this.#owned[asyncDisposeSymbol]().then(() => {
      if (this.#parent) this.#parent.#children.delete(this);
      retirement.resolve();
    }, retirement.reject);
    return retirement.promise;
  }
}
