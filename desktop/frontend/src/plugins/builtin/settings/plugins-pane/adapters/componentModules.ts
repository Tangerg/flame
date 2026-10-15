import type { FlameClient } from "@flame/runtime-contract/client";
import { HTTP_ENDPOINTS } from "@flame/runtime-contract/wire";
import type { PluginClientComponent, PluginInstallation } from "@flame/runtime-contract/wire";
import type { PluginComponent } from "@flame/plugin-sdk";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { asyncDisposeSymbol } from "dougong";

interface EvaluatedModule {
  exports: Record<string, unknown>;
  signal: AbortSignal;
}
export interface LoadedComponent {
  component: PluginComponent;
  signal: AbortSignal;
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
  private generation: object | undefined;
  private readonly releases = new Map<string, string>();
  private readonly modules = new Map<string, Promise<EvaluatedModule>>();

  async component(
    scope: ContributionLifetime,
    client: FlameClient,
    endpoint: string,
    generation: object,
    installation: PluginInstallation,
    entry: PluginClientComponent,
  ): Promise<LoadedComponent> {
    if (!HTMLScriptElement.supports("importmap"))
      throw new Error("This client cannot load the required shared plugin modules.");
    if (
      (this.generation && this.generation !== generation && this.releases.size) ||
      (this.releases.has(installation.id) &&
        this.releases.get(installation.id) !== installation.selected.digest)
    )
      throw new ModuleReloadRequired();
    this.generation = generation;
    const key = `${installation.id}/${installation.selected.digest}/${entry.entry}`;
    let module = this.modules.get(key);
    if (!module) {
      const moduleOwner = scope.lifetime(`module:${key}`);
      module = (async () => {
        const resources = await client.plugins.openResources(
          {
            installationId: installation.id,
            digest: installation.selected.digest,
            componentId: entry.id,
          },
          moduleOwner.signal,
        );
        moduleOwner.cleanup(() => resources.close());
        scope.spawn(async (signal) => {
          try {
            await resources.closed;
            if (!signal.aborted && !moduleOwner.signal.aborted)
              throw new Error("Plugin resource ownership ended unexpectedly.");
          } catch (error) {
            if (!signal.aborted && !moduleOwner.signal.aborted) {
              try {
                await moduleOwner[asyncDisposeSymbol]();
              } catch (cleanupError) {
                throw new AggregateError(
                  [error, cleanupError],
                  "Plugin resources could not be retired.",
                );
              }
              throw error;
            }
          }
        });
        moduleOwner.signal.throwIfAborted();
        const route = HTTP_ENDPOINTS.pluginResources.path
          .replace("{lease}", encodeURIComponent(resources.result.token))
          .replace("*", resources.result.entry.split("/").map(encodeURIComponent).join("/"));
        const url = `${endpoint.replace(/\/+$/, "")}${route}`;
        this.releases.set(installation.id, installation.selected.digest);
        try {
          const exports = (await import(/* @vite-ignore */ url)) as Record<string, unknown>;
          return { exports, signal: moduleOwner.signal };
        } catch (error) {
          await moduleOwner[asyncDisposeSymbol]();
          throw new Error(
            "The plugin module could not be evaluated. Reload the window before trying again.",
            { cause: error },
          );
        }
      })();
      this.modules.set(key, module);
    }
    const loaded = await module;
    scope.signal.throwIfAborted();
    if (!(entry.export in loaded.exports))
      throw new Error("The declared component export is unavailable.");
    return { component: loaded.exports[entry.export] as PluginComponent, signal: loaded.signal };
  }
}
