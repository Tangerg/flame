import {
  definePlugin as defineContractPlugin,
  type AnyPlugin,
  type Awaitable,
  type PluginContext as ContractContext,
  type ProvidedServices,
  type Provisions,
  type Requirements,
} from "dougong";
import { normalizePlainRecord } from "dougong/advanced";
import { ContributionScope } from "@/extensions/contributionScope";
import { type AmbientShell } from "@/extensions/context";
import { startTask, type TaskStartOptions } from "@/extensions/tasksStore";
import { createStorage } from "@/extensions/storage";

export { ContributionScope } from "@/extensions/contributionScope";

export type PluginContext<Requires extends Requirements = Requirements> = Omit<
  ContractContext<Requires>,
  "contributions"
> &
  AmbientShell & {
    readonly contributions: ContributionScope;
  };

export interface PluginSpec<
  Requires extends Requirements = Requirements,
  Provides extends Provisions = Provisions,
> {
  readonly name: string;
  readonly requires?: Requires;
  readonly provides?: Provides;
  readonly setup: (
    ctx: PluginContext<Requires>,
  ) => Awaitable<
    keyof NoInfer<Provides> extends never ? void : ProvidedServices<NoInfer<Provides>>
  >;
}

const specFields: ReadonlySet<string> = new Set(["name", "requires", "provides", "setup"]);

export function definePlugin<Requires extends Requirements = {}, Provides extends Provisions = {}>(
  spec: PluginSpec<Requires, Provides>,
): AnyPlugin {
  const { name, requires, provides, setup } = normalizePlainRecord(
    spec,
    "flame plugin declaration",
    { fields: specFields },
  );
  if (typeof setup !== "function") throw new TypeError("plugin setup must be a function");
  return defineContractPlugin<void, Requires, Provides>({
    name,
    ...(requires === undefined ? {} : { requires }),
    ...(provides === undefined ? {} : { provides }),
    setup: (ctx) =>
      setup({
        ...ctx,
        contributions: new ContributionScope(
          ctx.lifetime,
          ctx.contributions,
          ctx.metadata.pluginName,
        ),
        storage: createStorage(ctx.metadata.pluginName),
        startTask: (options: TaskStartOptions) => startTask(ctx.metadata.pluginName, options),
      }) as never,
  });
}

export type Contributor = Pick<ContributionScope, "add">;
