import { createElement, type ReactNode } from "react";
import { Lifetime, Service, QimuError, defineUnit, sameContract } from "@qimu/sdk";
import { parseJSON } from "@qimu/sdk/json";
import { ControlPump, HTTPControl, RealmModules, Runtime, type Use } from "@qimu/browser";
import { type ObjectRef, type PlannedUnit } from "@qimu/sdk/protocol";
import { type FlameClient, type MutationPromise } from "@flame/runtime-client";
import {
  HTTP_ENDPOINTS,
  type PluginBrowserSession,
  type PluginClientComponent,
  type PluginInstallation,
} from "@flame/runtime-client/wire";
import { type PluginComponent } from "@flame/component-sdk";
import { hostModuleProfile, hostModuleSpecifiers } from "@flame/component-sdk/runtime-profile";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { reportPluginError } from "@/extensions/errors";
import { NativeComponentMount } from "@/features/plugins/adapters/NativeComponentMount";

export interface LoadedComponent {
  component: PluginComponent;
  signal: AbortSignal;
  reload(): Promise<void>;
}

export interface ComponentAcquisition {
  readonly use: Use;
  readonly reference: ObjectRef;
  readonly contract: Service<PluginComponent>;
  flush(): Promise<void>;
}

interface ControlChannel {
  readonly lifetime: Lifetime;
  readonly pump: Lifetime;
  readonly control: HTTPControl;
}

// This adapter owns the actual window bootstrap. Qimu owns plugin objects, work,
// dependency protection and proofs; the private UI kernel only presents them.
export class ComponentModules {
  readonly #bootstrap = new Lifetime("flame-browser-bootstrap");
  readonly #opening = Object.freeze({
    operationId: crypto.randomUUID(),
    resumeToken: crypto.randomUUID() + crypto.randomUUID(),
  });
  readonly #modules = new RealmModules((unit, signal) => this.#openArtifact(unit, signal));
  readonly #wrap: (children: ReactNode) => ReactNode;
  #client?: FlameClient;
  #endpoint = "";
  #generation?: object;
  #session?: PluginBrowserSession;
  #runtime?: Runtime;
  #channel?: ControlChannel;
  #connection?: Promise<void>;
  #openingMutation?: MutationPromise<PluginBrowserSession>;
  #closingMutation?: MutationPromise<void>;
  #sealed = false;
  #retiring?: Promise<void>;
  #closed = false;

  constructor(wrap: (children: ReactNode) => ReactNode = (children) => children) {
    this.#wrap = wrap;
  }

  readonly report = (error: unknown): void => {
    reportPluginError("installed-components", "other", error);
  };

  disconnect(): void {
    this.#runtime?.disconnect();
  }

  async #connect(client: FlameClient, endpoint: string, generation: object): Promise<void> {
    if (this.#closed) throw new QimuError("closed", "browser plugin bootstrap is closed");
    if (this.#connection) await this.#connection;
    if (this.#generation === generation && this.#runtime) return;
    const previousClient = this.#client;
    this.#client = client;
    this.#endpoint = endpoint;
    this.#connection = this.#bootstrap
      .run(async (signal) => {
        this.#runtime?.disconnect();
        const previous = this.#channel;
        if (previous) {
          previous.pump.beginClose();
          previous.lifetime.beginClose();
          await Promise.all([previous.pump.close(), previous.lifetime.close()]);
        }
        const request = {
          ...this.#opening,
          ...(this.#session ? { resume: this.#session.realm } : {}),
        };
        this.#openingMutation =
          previousClient === client && this.#openingMutation && !this.#session
            ? this.#openingMutation.retry({ signal })
            : client.plugins.openBrowserRealm(request, signal);
        const session = await this.#openingMutation;
        const lifetime = this.#bootstrap.branch("realm-control");
        const control = new HTTPControl({
          endpoint: new URL(session.controlEndpoint, endpoint).href,
          credential: session.credential,
          maximumMessageBytes: session.maximumMessageBytes,
          lifetime,
        });
        const pumpOwner = this.#bootstrap.branch("realm-pump");
        this.#session = session;
        this.#channel = { lifetime, pump: pumpOwner, control };
        if (this.#runtime) await this.#runtime.reconnect(control);
        else {
          const contract = new Service<PluginComponent>(session.componentContract);
          this.#runtime = new Runtime({
            authority: session.authority,
            realm: session.realm,
            control,
            proofCapacity: session.proofCapacity,
            load: async (unit, owner, plan) => {
              if (
                unit.imports.length ||
                unit.exports.length !== 1 ||
                unit.exports[0]!.name !== "component" ||
                !sameContract(unit.exports[0]!.contract, contract.ref)
              )
                throw new QimuError(
                  "contract_mismatch",
                  "Flame component plan differs from its published native component contract",
                );
              if (!HTMLScriptElement.supports("importmap"))
                throw new QimuError(
                  "unavailable",
                  "this client cannot share installed component modules",
                );
              const configuration = parseJSON(unit.configurationJson) as {
                export?: unknown;
                moduleProfile?: unknown;
                sharedModules?: unknown;
              };
              if (
                !configuration ||
                typeof configuration.export !== "string" ||
                !configuration.export
              )
                throw new QimuError(
                  "contract_mismatch",
                  "Flame component configuration omitted its declared export",
                );
              if (
                !Array.isArray(configuration.sharedModules) ||
                (configuration.moduleProfile !== undefined &&
                  configuration.moduleProfile !== hostModuleProfile) ||
                (configuration.sharedModules.length > 0 &&
                  configuration.moduleProfile !== hostModuleProfile) ||
                configuration.sharedModules.some(
                  (specifier) =>
                    typeof specifier !== "string" ||
                    !hostModuleSpecifiers.some((approved) => approved === specifier),
                )
              )
                throw new QimuError(
                  "contract_mismatch",
                  "component shared imports differ from the current native host module profile",
                );
              const exports = await this.#modules.evaluate(unit, owner, plan);
              const component = exports[configuration.export];
              if (!component || (typeof component !== "function" && typeof component !== "object"))
                throw new QimuError(
                  "contract_mismatch",
                  "declared React component export is unavailable",
                );
              return defineUnit({
                id: unit.instance.source.unitId,
                imports: {},
                exports: { component: contract },
                setup: () => ({ component: component as PluginComponent }),
              });
            },
            remote: () => {
              throw new QimuError(
                "contract_mismatch",
                "Flame component authoring declares no portable setup imports",
              );
            },
          });
        }
        const pump = new ControlPump(this.#runtime, control, {
          intervalMilliseconds: session.pollIntervalMilliseconds,
          onError: this.report,
        });
        void pumpOwner.run((signal) => pump.run(signal)).catch(this.report);
        this.#generation = generation;
      })
      .finally(() => {
        this.#connection = undefined;
      });
    await this.#connection;
  }

  async #openArtifact(unit: PlannedUnit, signal: AbortSignal) {
    const client = this.#client;
    const configuration = parseJSON(unit.configurationJson) as { component?: { id?: unknown } };
    if (!client || typeof configuration.component?.id !== "string" || !configuration.component.id)
      throw new QimuError("invalid_plan", "component execution has no Flame artifact source");
    const resources = await client.plugins.openResources(
      {
        installationId: unit.instance.source.installationId,
        digest: unit.instance.source.release,
        componentId: configuration.component.id,
      },
      signal,
    );
    if (resources.result.entry !== unit.module.path) {
      await resources.close();
      throw new QimuError(
        "contract_mismatch",
        "leased component path differs from the approved artifact",
      );
    }
    const route = HTTP_ENDPOINTS.pluginResources.path
      .replace("{lease}", encodeURIComponent(resources.result.token))
      .replace("*", resources.result.entry.split("/").map(encodeURIComponent).join("/"));
    // The authenticated Flame artifact owner checks the exact release and entry.
    // The browser lease projects that same approved content identity.
    let ended = false;
    void resources.closed.then(
      () => {
        ended = true;
      },
      () => {
        ended = true;
      },
    );
    return {
      url: `${this.#endpoint.replace(/\/+$/, "")}${route}`,
      digest: unit.module.digest,
      closed: resources.closed,
      async close() {
        try {
          await resources.close();
        } catch (error) {
          return { status: ended ? ("released" as const) : ("unconfirmed" as const), error };
        }
      },
    };
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
    await this.#connect(client, endpoint, generation);
    scope.signal.throwIfAborted();
    if (this.#runtime!.connection === "cleanupOnly")
      throw new QimuError(
        "reload_required",
        "the previous browser authority requires a controlled Realm reload",
      );
    const request = Object.freeze({
      installationId: installation.id,
      digest: installation.selected.digest,
      componentId: entry.id,
    });
    const acquire = async (parent?: Use): Promise<ComponentAcquisition> => {
      const runtime = this.#runtime!;
      const session = this.#session!;
      if (runtime.connection !== "connected")
        throw new QimuError(
          "unavailable",
          "component admission requires the current Host control channel",
        );
      const operationId = crypto.randomUUID();
      const grant = await this.#bootstrap.run((signal) =>
        this.#client!.plugins.acquireComponent(
          {
            ...request,
            operationId,
            realm: session.realm,
            ...(parent ? { parentUseId: parent.id } : {}),
          },
          signal,
        ),
      );
      const use = await runtime.accept(grant);
      try {
        if (parent) use.lifetime.dependsOn(parent.lifetime);
        const contract = new Service<PluginComponent>(session.componentContract);
        const objects = use.objects.filter(
          (reference) =>
            reference.export.name === "component" &&
            sameContract(reference.export.contract, contract.ref),
        );
        if (objects.length !== 1)
          throw new QimuError(
            "contract_mismatch",
            "component grant must identify one exact component object",
          );
        return { use, reference: objects[0]!, contract, flush: () => runtime.flushProofs() };
      } catch (error) {
        await use.close();
        throw error;
      }
    };
    const component: PluginComponent = (props) =>
      createElement(NativeComponentMount, {
        acquire,
        wrap: this.#wrap,
        report: this.report,
        ...props,
      });
    return { component, signal: scope.signal, reload: () => this.reload() };
  }

  close(): Promise<void> {
    this.#closed = true;
    this.#retiring ??= this.#close().finally(() => {
      this.#retiring = undefined;
    });
    return this.#retiring;
  }

  async #close(): Promise<void> {
    if (this.#connection) await this.#connection;
    if (this.#session && this.#runtime) {
      if (!this.#sealed) {
        this.#closingMutation = this.#closingMutation
          ? this.#closingMutation.retry()
          : this.#client!.plugins.closeBrowserRealm({ realm: this.#session.realm });
        await this.#closingMutation;
        this.#sealed = true;
        this.#closingMutation = undefined;
      }
      // The original Runtime gate checks actual exits and the complete proof
      // outbox, including reservations whose Host acknowledgement is unknown.
      await this.#runtime.reload(() => {});
    }
    this.#channel?.pump.beginClose();
    this.#channel?.lifetime.beginClose();
    const outcome = await this.#bootstrap.close();
    if (outcome.status !== "released")
      throw new QimuError("close_unconfirmed", "browser control transport has not released");
  }

  async reload(): Promise<void> {
    await this.close();
    location.reload();
  }
}
