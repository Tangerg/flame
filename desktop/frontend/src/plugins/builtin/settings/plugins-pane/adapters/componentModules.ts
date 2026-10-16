import type { FlameClient } from "@flame/runtime-contract/client";
import { HTTP_ENDPOINTS } from "@flame/runtime-contract/wire";
import type { PluginClientComponent, PluginInstallation } from "@flame/runtime-contract/wire";
import type { PluginComponent } from "@flame/plugin-sdk";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { asyncDisposeSymbol } from "dougong";
import { trackContributionIO } from "@/plugins/sdk/contributionIO";

interface EvaluatedModule {
  exports: Record<string, unknown>;
  signal: AbortSignal;
}
export interface LoadedComponent {
  component: PluginComponent;
  signal: AbortSignal;
}

interface LoadingModule {
  readonly generation: object;
  readonly evaluation: Promise<EvaluatedModule>;
}

class ModuleReloadRequired extends Error {
  constructor() {
    super("Reload this window to establish a clean plugin module generation.");
    this.name = "ModuleReloadRequired";
  }
}

// The browser window owns ESM evaluation, which may never finish. Connection
// generations own resource leases and refuse late exports; they cannot join the
// browser's evaluation. A different release or generation requires a reload.
export class ComponentModules {
  private evaluatedGeneration: object | undefined;
  private readonly evaluatedReleases = new Map<string, string>();
  private readonly modules = new Map<string, LoadingModule>();

  private assertGeneration(generation: object, installation: PluginInstallation) {
    if (
      (this.evaluatedGeneration && this.evaluatedGeneration !== generation) ||
      (this.evaluatedReleases.has(installation.id) &&
        this.evaluatedReleases.get(installation.id) !== installation.selected.digest)
    )
      throw new ModuleReloadRequired();
  }

  private async retire(owner: ContributionLifetime, error: unknown): Promise<unknown> {
    try {
      await owner[asyncDisposeSymbol]();
      return error;
    } catch (cleanupError) {
      return new AggregateError([error, cleanupError], "Plugin resources could not be retired.");
    }
  }

  async component(
    scope: ContributionLifetime,
    client: FlameClient,
    endpoint: string,
    generation: object,
    installation: PluginInstallation,
    entry: PluginClientComponent,
  ): Promise<LoadedComponent> {
    scope.signal.throwIfAborted();
    if (!HTMLScriptElement.supports("importmap"))
      throw new Error("This client cannot load the required shared plugin modules.");
    this.assertGeneration(generation, installation);
    const key = `${installation.id}/${installation.selected.digest}/${entry.entry}`;
    let module = this.modules.get(key);
    if (!module || module.generation !== generation) {
      const moduleOwner = scope.lifetime(`module:${key}`);
      const prepare = async () => {
        const resources = await client.plugins.openResources(
          {
            installationId: installation.id,
            digest: installation.selected.digest,
            componentId: entry.id,
          },
          moduleOwner.signal,
        );
        if (moduleOwner.signal.aborted) {
          await resources.close();
          moduleOwner.signal.throwIfAborted();
        }
        moduleOwner.cleanup(() => resources.close());
        scope.spawn(async (signal) => {
          try {
            await resources.closed;
            if (!signal.aborted && !moduleOwner.signal.aborted)
              throw new Error("Plugin resource ownership ended unexpectedly.");
          } catch (error) {
            if (!signal.aborted && !moduleOwner.signal.aborted) {
              throw await this.retire(moduleOwner, error);
            }
          }
        });
        moduleOwner.signal.throwIfAborted();
        return resources;
      };
      const rejectPreparation = async (error: unknown): Promise<never> => {
        if (this.modules.get(key) === loading) this.modules.delete(key);
        throw await this.retire(moduleOwner, error);
      };
      const acquisition = trackContributionIO(scope, prepare().catch(rejectPreparation));
      const loading: LoadingModule = {
        generation,
        evaluation: acquisition.then(async (resources) => {
          try {
            moduleOwner.signal.throwIfAborted();
            this.assertGeneration(generation, installation);
          } catch (error) {
            return rejectPreparation(error);
          }
          const route = HTTP_ENDPOINTS.pluginResources.path
            .replace("{lease}", encodeURIComponent(resources.result.token))
            .replace("*", resources.result.entry.split("/").map(encodeURIComponent).join("/"));
          const url = `${endpoint.replace(/\/+$/, "")}${route}`;
          this.evaluatedGeneration = generation;
          this.evaluatedReleases.set(installation.id, installation.selected.digest);
          try {
            const exports = (await import(/* @vite-ignore */ url)) as Record<string, unknown>;
            return { exports, signal: moduleOwner.signal };
          } catch (error) {
            throw new Error(
              "The plugin module could not be evaluated. Reload the window before trying again.",
              { cause: await this.retire(moduleOwner, error) },
            );
          }
        }),
      };
      this.modules.set(key, loading);
      module = loading;
    }
    const loaded = await module.evaluation;
    scope.signal.throwIfAborted();
    if (loaded.signal.aborted) throw new ModuleReloadRequired();
    if (!(entry.export in loaded.exports))
      throw new Error("The declared component export is unavailable.");
    return { component: loaded.exports[entry.export] as PluginComponent, signal: loaded.signal };
  }
}
