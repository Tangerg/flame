import {
  definePlugin as defineContractPlugin,
  normalizePlainRecord,
  asyncDisposeSymbol,
  type AnyPlugin,
  type Awaitable,
  type PluginContext as ContractContext,
  type Contribution as ContractContribution,
  type ProvidedServices,
  type Provisions,
  type Requirements,
  type LifetimeContext,
} from "dougong";
import type { Contribution } from "./contracts";
import { notifyFrom } from "./notifications";
import type { AmbientShell } from "./services";
import { startTask } from "./tasksStore";
import { createStorage } from "./storage";
import type { ExtensionContributionOptions, ExtensionPoint } from "./types/extensions";
import type { NotificationLevel, TaskStartOptions } from "./types/infra";
import { ExactSequence } from "@/foundation/exactSequence";

type ExtensionContribution<T> = Pick<ContractContribution<T>, "dispose" | "update">;

export type PluginContext<Requires extends Requirements = Requirements> = Omit<
  ContractContext<Requires>,
  "contribute" | "lifetime"
> &
  AmbientShell & {
    lifetime(label: string): ContributionLifetime;
    contribute<T>(
      point: ExtensionPoint<T>,
      item: T,
      opts?: ExtensionContributionOptions,
    ): ExtensionContribution<T>;
  };

export type ContributionLifetime = Omit<LifetimeContext, "contribute" | "lifetime"> &
  Pick<PluginContext, "contribute" | "lifetime">;

export interface PluginSpec<
  Requires extends Requirements = Requirements,
  Provides extends Provisions = Provisions,
> {
  readonly name: string;
  readonly requires?: Requires;
  readonly provides?: Provides;
  readonly setup: (
    ctx: PluginContext<Requires>,
  ) => Awaitable<keyof Provides extends never ? void : ProvidedServices<Provides>>;
}

const mintedIds = new ExactSequence();
const specFields: ReadonlySet<string> = new Set(["name", "requires", "provides", "setup"]);
const contributionFields: ReadonlySet<string> = new Set(["id", "key"]);

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

function createContribute(ctx: Pick<LifetimeContext, "contribute">, name: string) {
  return <T>(
    point: ExtensionPoint<T>,
    item: T,
    opts?: ExtensionContributionOptions,
  ): ExtensionContribution<T> => {
    const options =
      opts === undefined
        ? undefined
        : normalizePlainRecord(opts, "flame contribution options", { fields: contributionFields });
    const key = domainKey(point, item, options);
    const envelope: Contribution<T> = { key, plugin: name, item };
    const contribution = ctx.contribute(point.token, key, envelope);
    return {
      dispose: () => contribution.dispose(),
      update(next) {
        if (point.keying === "single" && domainKey(point, next, options) !== key) {
          throw new Error(`extension contribution "${point.id}" cannot change its key`);
        }
        contribution.update({ ...envelope, item: next });
      },
    };
  };
}

function bindLifetime(scope: LifetimeContext, name: string): ContributionLifetime {
  return Object.freeze({
    get signal() {
      return scope.signal;
    },
    cleanup: (dispose) => scope.cleanup(dispose),
    spawn: (task) => scope.spawn(task),
    on: (token, listener) => scope.on(token, listener),
    emit: (token, ...payload) => scope.emit(token, ...payload),
    contribute: createContribute(scope, name),
    lifetime: (label: string) => bindLifetime(scope.lifetime(label), name),
    dispose: () => scope.dispose(),
    [asyncDisposeSymbol]: () => scope[asyncDisposeSymbol](),
  } satisfies ContributionLifetime);
}

function bindContext<Requires extends Requirements>(
  ctx: ContractContext<Requires>,
  name: string,
): PluginContext<Requires> {
  const wrapped = {
    ...ctx,
    contribute: createContribute(ctx as ContractContext<Requirements>, name),
    lifetime: (label: string) => bindLifetime(ctx.lifetime(label), name),
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
  const { name, requires, provides, setup } = normalizePlainRecord(
    spec,
    "flame plugin declaration",
    {
      fields: specFields,
    },
  );
  if (typeof setup !== "function") throw new TypeError("plugin setup must be a function");
  return defineContractPlugin<void, Requires, Provides>({
    name,
    ...(requires === undefined ? {} : { requires }),
    ...(provides === undefined ? {} : { provides }),
    setup: (ctx) => setup(bindContext(ctx, ctx.meta.pluginName)) as never,
  });
}

export type Contributor = Pick<PluginContext, "contribute">;
