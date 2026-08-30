// Owns exactly ONE thing over Core's `definePlugin`: `contribute` takes a point HANDLE
// rather than a raw key and applies that handle's policy — key derivation and
// normalization — into the envelope the read side needs. Applying it per call site instead
// is how the policy stops being a policy.
//
// No `version` field: a version is distribution metadata and lives on the platform Manifest.

import {
  definePlugin as defineContractPlugin,
  type AnyPlugin,
  type Awaitable,
  type PluginContext as ContractContext,
  type Provisions,
  type Requirements,
} from "dougong";
import type { Contribution } from "./contracts";
import { notifyFrom } from "./notifications";
import type { AmbientShell } from "./services";
import { startTask } from "@/state/tasksStore";
import { createStorage } from "./storage";
import type { Disposable } from "./types/common";
import type { ExtensionContributionOptions, ExtensionPoint } from "./types/extensions";
import type { NotificationLevel, TaskStartOptions } from "./types/infra";
import { ExactSequence } from "@/foundation/exactSequence";

export type PluginContext<Requires extends Requirements = Requirements> = Omit<
  ContractContext<Requires>,
  "contribute"
> &
  AmbientShell & {
    contribute<T>(
      point: ExtensionPoint<T>,
      item: T,
      opts?: ExtensionContributionOptions,
    ): Disposable;
  };

export interface PluginSpec<
  Requires extends Requirements = Requirements,
  Provides extends Provisions = Provisions,
> {
  /** Built-ins use the `flame.builtin.*` namespace. */
  readonly name: string;
  readonly requires?: Requires;
  readonly provides?: Provides;
  readonly setup: (
    ctx: PluginContext<Requires>,
  ) => Awaitable<keyof Provides extends never ? void : { [K in keyof Provides]: unknown }>;
}

// For `multi` contributions with no explicit `opts.id`. Uniqueness only has to hold within
// one point's keyspace under one owner, so a global counter is simpler than per-point ones
// and the ids never reach plugin code.
const mintedIds = new ExactSequence();

function itemId(item: unknown): string | undefined {
  if (typeof item !== "object" || item === null || !("id" in item)) return undefined;
  return typeof item.id === "string" ? item.id : undefined;
}

function domainKey<T>(
  point: ExtensionPoint<T>,
  item: T,
  opts: ExtensionContributionOptions | undefined,
): string {
  if (point.keying === "multi") return opts?.id ?? `${point.id}#${mintedIds.issue()}`;
  const key = opts?.key ?? point.keyOf?.(item) ?? itemId(item);
  if (!key) {
    throw new Error(
      `Single extension point "${point.id}" requires opts.key, keyOf, or a non-empty item.id`,
    );
  }
  return point.normalizeKey ? point.normalizeKey(key) : key;
}

function createContribute(ctx: ContractContext<Requirements>, name: string) {
  return <T>(
    point: ExtensionPoint<T>,
    item: T,
    opts?: ExtensionContributionOptions,
  ): Disposable => {
    const key = domainKey(point, item, opts);
    const envelope: Contribution<T> = { key, order: opts?.order, plugin: name, item };
    return ctx.contribute(point.token, key, envelope);
  };
}

// `signal` is a getter on Core's frozen context and the requirement aliases are
// dynamic, so this spreads for the aliases and re-declares `signal` as a getter
// rather than freezing whatever it happened to read at wrap time.
function bindContext<Requires extends Requirements>(
  ctx: ContractContext<Requires>,
  name: string,
): PluginContext<Requires> {
  const wrapped = {
    ...ctx,
    contribute: createContribute(ctx as ContractContext<Requirements>, name),
    notify: (message: string, level: NotificationLevel = "info") =>
      notifyFrom(name, message, level),
    storage: createStorage(name),
    startTask: (opts: TaskStartOptions) => startTask(name, opts),
  };
  Object.defineProperty(wrapped, "signal", { get: () => ctx.signal, enumerable: true });
  return Object.freeze(wrapped) as unknown as PluginContext<Requires>;
}

export function definePlugin<Requires extends Requirements = {}, Provides extends Provisions = {}>(
  spec: PluginSpec<Requires, Provides>,
): AnyPlugin {
  return defineContractPlugin<void, Requires, Provides>({
    name: spec.name,
    ...(spec.requires ? { requires: spec.requires } : {}),
    ...(spec.provides ? { provides: spec.provides } : {}),
    setup: (ctx) => spec.setup(bindContext(ctx, spec.name)) as never,
  });
}

/** The MINIMUM a registration helper needs: a helper handed the whole Host quietly grows a
 *  second and third thing it touches. */
export type Contributor = Pick<PluginContext, "contribute">;
