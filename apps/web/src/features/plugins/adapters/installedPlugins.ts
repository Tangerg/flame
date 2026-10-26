import { type ThemeAccess } from "@/features/theme/public/theme";
import { asyncDisposeSymbol } from "dougong";
import { type FlameClient } from "@flame/runtime-client";
import { type ContributionLifetime, type PluginContext } from "@/extensions/definePlugin";
import { type RuntimeStream, followRuntimeGeneration } from "@/features/connection/public/services";
import { registerPackageContributions } from "@/features/plugins/contributions";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { createInstalledPackageState } from "@/features/plugins/adapters/packages";
export function createInstalledPlugins(
  runtimeClient: () => FlameClient,
  runtimeEndpoint: () => string,
  themeAccess: ThemeAccess,
) {
  const access = createInstalledPackageState();
  const modules = new ComponentModules();
  let current: ContributionLifetime | undefined;
  let retiring = Promise.resolve();
  let latest: object = {};
  let stop: (() => void) | null = null;
  let disposed = false;
  let disposal: Promise<void> | undefined;
  return {
    access,
    start(ctx: Pick<PluginContext<{}>, "lifetime" | "signal" | "log">, runtime: RuntimeStream) {
      if (disposed) throw new Error("Installed plugins are disposed");
      if (stop) throw new Error("Installed plugins are already started");
      const replace = () => {
        const intent = {};
        latest = intent;
        const predecessor = current;
        current = undefined;
        const release = predecessor?.[asyncDisposeSymbol]() ?? Promise.resolve();
        retiring = retiring
          .then(async () => {
            await release;
            if (
              disposed ||
              ctx.signal.aborted ||
              latest !== intent ||
              runtime.connectionGeneration() === null
            )
              return;
            const owned = ctx.lifetime("runtime-packages");
            try {
              registerPackageContributions(
                access,
                owned,
                runtimeClient(),
                modules,
                runtime.connectionGeneration()!,
                runtimeEndpoint(),
                themeAccess,
              );
              current = owned;
            } catch (error) {
              await owned[asyncDisposeSymbol]();
              throw error;
            }
          })
          .catch((error: unknown) => {
            if (!disposed && !ctx.signal.aborted) ctx.log.error(error);
          });
      };
      stop = followRuntimeGeneration(runtime, replace);
      replace();
    },
    dispose() {
      if (disposal) return disposal;
      disposed = true;
      latest = {};
      stop?.();
      stop = null;
      const release = current?.[asyncDisposeSymbol]() ?? Promise.resolve();
      current = undefined;
      access.clear();
      disposal = Promise.all([retiring, release]).then(() => undefined);
      return disposal;
    },
  };
}
export type InstalledPluginsOwner = ReturnType<typeof createInstalledPlugins>;
