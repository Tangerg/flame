import type { WorkspaceAccess } from "@/features/workspace/public/workspace";
import { type ThemeAccess } from "@/features/theme/public/theme";
import { asyncDisposeSymbol } from "dougong";
import { type FlameClient } from "@flame/runtime-client";
import { type ComponentContext } from "@flame/component-sdk";
import { type ContributionScope, type PluginContext } from "@/extensions/definePlugin";
import { type RuntimeStream, followRuntimeGeneration } from "@/features/connection/public/services";
import { registerPackageContributions } from "@/features/plugins/contributions";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { createInstalledPackageState } from "@/features/plugins/adapters/packages";
import { type ReactNode } from "react";
export function createInstalledPlugins(
  runtimeClient: () => FlameClient,
  runtimeEndpoint: () => string,
  workspace: WorkspaceAccess,
  themeAccess: ThemeAccess,
  saveFile: ComponentContext["saveFile"],
  wrap: (children: ReactNode) => ReactNode = (children) => children,
) {
  const access = createInstalledPackageState(runtimeEndpoint);
  const modules = new ComponentModules(wrap);
  let current: ContributionScope | undefined;
  let retiring = Promise.resolve();
  let latest: object = {};
  let stop: (() => void) | null = null;
  let disposal: Promise<void> | undefined;
  return {
    access,
    reload: () => modules.reload(),
    start(
      ctx: Pick<PluginContext<{}>, "contributions" | "lifetime" | "logger">,
      runtime: RuntimeStream,
    ) {
      if (disposal) throw new Error("Installed plugins are disposed");
      if (stop) throw new Error("Installed plugins are already started");
      const replace = () => {
        modules.disconnect();
        const intent = {};
        latest = intent;
        const predecessor = current;
        current = undefined;
        const release = predecessor?.[asyncDisposeSymbol]() ?? Promise.resolve();
        retiring = Promise.all([retiring, release]).then(async () => {
          const generation = runtime.connectionGeneration();
          if (disposal || ctx.lifetime.signal.aborted || latest !== intent || generation === null)
            return;
          const owned = ctx.contributions.child("runtime-packages");
          current = owned;
          try {
            registerPackageContributions(
              access,
              owned,
              runtimeClient(),
              modules,
              generation,
              runtimeEndpoint(),
              workspace,
              themeAccess,
              saveFile,
            );
          } catch (error) {
            if (current === owned) current = undefined;
            try {
              await owned[asyncDisposeSymbol]();
            } catch (cleanup) {
              throw new AggregateError(
                [error, cleanup],
                "Installed plugin registration and retirement failed",
              );
            }
            if (disposal || latest !== intent || ctx.lifetime.signal.aborted) return;
            throw error;
          }
        });
        void retiring.catch((error: unknown) => {
          if (!disposal && !ctx.lifetime.signal.aborted) ctx.logger.error(error);
        });
      };
      stop = followRuntimeGeneration(runtime, replace);
      replace();
    },
    dispose() {
      if (disposal) return disposal;
      // Withdrawn observers can synchronously reenter and must join this same exit.
      const completion = Promise.withResolvers<void>();
      disposal = completion.promise;
      latest = {};
      const unsubscribe = stop;
      stop = null;
      const owned = current;
      current = undefined;
      const releases = [
        () => unsubscribe?.(),
        () => owned?.[asyncDisposeSymbol](),
        () => access.clear(),
      ];
      void Promise.allSettled([retiring, ...releases.map(async (release) => release())])
        .then(async (retirements) => {
          const failures = retirements.flatMap((result) =>
            result.status === "rejected" ? [result.reason] : [],
          );
          try {
            await modules.close();
          } catch (error) {
            failures.push(error);
          }
          if (failures.length)
            throw new AggregateError(failures, "Installed plugin retirement failed");
        })
        .then(completion.resolve, completion.reject);
      return disposal;
    },
  };
}
export type InstalledPluginsOwner = ReturnType<typeof createInstalledPlugins>;
