import {
  type AnyPlugin,
  type PluginAuthoring,
  type PluginContext as ContractContext,
  type Provisions,
  type Requirements,
} from "dougong";
import { capturePluginDeclaration, capturePlainRecord } from "dougong/advanced";
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

export type PluginSpec<Requires extends Requirements = {}, Provides extends Provisions = {}> = Pick<
  PluginAuthoring<void, Requires, Provides>,
  "name" | "requires" | "provides"
> & {
  readonly setup: (
    ctx: PluginContext<NoInfer<Requires>>,
  ) => ReturnType<PluginAuthoring<void, Requires, NoInfer<Provides>>["setup"]>;
};

const specFields: ReadonlySet<string> = new Set(["name", "requires", "provides", "setup"]);

export function definePlugin<Requires extends Requirements = {}, Provides extends Provisions = {}>(
  spec: PluginSpec<Requires, Provides>,
): AnyPlugin {
  const { name, requires, provides, setup } = capturePlainRecord(spec, "flame plugin declaration", {
    fields: specFields,
  });
  if (typeof setup !== "function") throw new TypeError("plugin setup must be a function");
  return capturePluginDeclaration({
    name,
    ...(requires === undefined ? {} : { requires }),
    ...(provides === undefined ? {} : { provides }),
    setup: (ctx: ContractContext<Requires>) =>
      setup({
        ...ctx,
        contributions: new ContributionScope(
          ctx.lifetime,
          ctx.contributions,
          ctx.metadata.pluginName,
        ),
        storage: createStorage(ctx.metadata.pluginName),
        startTask: (options: TaskStartOptions) => startTask(ctx.metadata.pluginName, options),
      }),
  });
}

export type Contributor = Pick<ContributionScope, "add">;
