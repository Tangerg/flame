import {
  mutationSettlementIsUnknown,
  type ComponentContext,
  type ComponentSource,
} from "@flame/component-sdk";
import type { FlameClient, MutationPromise } from "@flame/runtime-client";
import type {
  EmbeddingRole,
  Model,
  Provider,
  ProviderTestResult,
  UtilityRole,
} from "@flame/runtime-client/wire";
import { ProviderCredentialsDraft } from "./draft";

export type Feedback =
  | { kind: "saved" }
  | { kind: "tested"; outcome: ProviderTestResult["outcome"] }
  | { kind: "error"; error: unknown };
export type Command =
  { phase: "running" } | { phase: "unknown"; error: unknown };
export type Read<T> =
  | { readonly phase: "loading"; readonly data?: T }
  | { readonly phase: "failed"; readonly data?: T; readonly error: unknown }
  | { readonly phase: "ready"; readonly data: T };
interface Results {
  providers: Provider[];
  models: Model[];
  utility: UtilityRole;
  embedding: EmbeddingRole;
}
type ReadKey = keyof Results;
interface Snapshot {
  readonly providers: Read<Provider[]>;
  readonly models: Read<Model[]>;
  readonly utility: Read<UtilityRole>;
  readonly embedding: Read<EmbeddingRole>;
  readonly observationFailure?: { readonly error: unknown };
  readonly drafts: ReadonlyMap<string, ProviderCredentialsDraft>;
  readonly commands: ReadonlyMap<string, Command>;
  readonly feedback: ReadonlyMap<string, Feedback>;
  readonly probing: ReadonlySet<string>;
}
interface Pending {
  original: MutationPromise<unknown>;
  accept(result: unknown): void;
  witness?: { provider: string; draft: ProviderCredentialsDraft };
}

export const providerResource = (id: string) =>
  JSON.stringify(["provider", id]);

export class ProviderSettings {
  readonly #runtimeIdentity: object;
  readonly #component: ComponentSource["component"];
  readonly #signal: AbortSignal;
  readonly #listeners = new Set<() => void>();
  readonly #pending = new Map<string, Pending>();
  readonly #draftEdits = new Map<string, object>();
  readonly #reads = new Map<ReadKey, AbortController>();
  #snapshot: Snapshot = {
    providers: { phase: "loading" },
    models: { phase: "loading" },
    utility: { phase: "loading" },
    embedding: { phase: "loading" },
    drafts: new Map(),
    commands: new Map(),
    feedback: new Map(),
    probing: new Set(),
  };

  constructor(source: ComponentSource) {
    source.signal.throwIfAborted();
    this.#runtimeIdentity = source.runtimeIdentity;
    this.#component = source.component;
    this.#signal = source.signal;
    this.#signal.addEventListener(
      "abort",
      () => {
        this.#pending.clear();
        this.#draftEdits.clear();
        for (const controller of this.#reads.values()) controller.abort();
        this.#reads.clear();
        this.#snapshot = {
          providers: { phase: "loading" },
          models: { phase: "loading" },
          utility: { phase: "loading" },
          embedding: { phase: "loading" },
          drafts: new Map(),
          commands: new Map(),
          feedback: new Map(),
          probing: new Set(),
        };
        this.#publish();
      },
      { once: true },
    );
  }

  getSnapshot = () => this.#snapshot;
  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  observe(context: ComponentContext) {
    this.#assert(context);
    const controller = new AbortController();
    const signal = AbortSignal.any([
      controller.signal,
      context.signal,
      this.#signal,
    ]);
    this.#snapshot = { ...this.#snapshot, observationFailure: undefined };
    this.refresh(context, signal);
    void context.track(async () => {
      try {
        const subscription = await context.runtime.runtimeEvents.subscribe(
          { topics: ["models.changed", "plugins.changed"] },
          signal,
        );
        if (signal.aborted) return;
        this.refresh(context, signal);
        for await (const event of subscription.events) {
          if (signal.aborted) break;
          if (
            event.type === "models.changed" ||
            event.type === "plugins.changed" ||
            event.type === "resync"
          )
            this.refresh(context, signal);
        }
      } catch (error) {
        if (!signal.aborted) {
          this.#snapshot = { ...this.#snapshot, observationFailure: { error } };
          this.#publish();
        }
      }
    });
    return () => controller.abort();
  }

  refresh(context: ComponentContext, signal = context.signal) {
    for (const key of ["providers", "models", "utility", "embedding"] as const)
      this.refreshRead(context, key, signal);
  }

  refreshRead<K extends ReadKey>(
    context: ComponentContext,
    key: K,
    parent = context.signal,
  ) {
    if (this.#signal.aborted || context.signal.aborted || parent.aborted)
      return;
    this.#assert(context);
    this.#cancelRead(key);
    const controller = new AbortController();
    this.#reads.set(key, controller);
    const signal = AbortSignal.any([
      controller.signal,
      this.#signal,
      context.signal,
      parent,
    ]);
    this.#snapshot = {
      ...this.#snapshot,
      [key]: { phase: "loading", data: this.#snapshot[key].data },
    };
    this.#publish();
    void context.track(async () => {
      try {
        const data = await readers[key](context.runtime, signal);
        if (!signal.aborted)
          this.#snapshot = {
            ...this.#snapshot,
            [key]: { phase: "ready", data },
          };
      } catch (error) {
        if (!signal.aborted)
          this.#snapshot = {
            ...this.#snapshot,
            [key]: { phase: "failed", data: this.#snapshot[key].data, error },
          };
      } finally {
        if (this.#reads.get(key) === controller) this.#reads.delete(key);
        if (!signal.aborted) this.#publish();
      }
    });
  }

  edit(
    context: ComponentContext,
    provider: Provider,
    draft: ProviderCredentialsDraft,
  ) {
    this.#assert(context);
    this.#draftEdits.set(provider.id, {});
    const drafts = new Map(this.#snapshot.drafts);
    if (draft.dirty(provider)) drafts.set(provider.id, draft);
    else drafts.delete(provider.id);
    const feedback = new Map(this.#snapshot.feedback);
    feedback.delete(providerResource(provider.id));
    this.#snapshot = { ...this.#snapshot, drafts, feedback };
    this.#publish();
  }

  save(
    context: ComponentContext,
    provider: Provider,
    submitted: ProviderCredentialsDraft,
  ) {
    this.#assert(context);
    if (this.#snapshot.probing.has(provider.id)) return Promise.resolve(false);
    return this.#save(context, provider, submitted);
  }

  #save(
    context: ComponentContext,
    provider: Provider,
    submitted: ProviderCredentialsDraft,
  ) {
    return this.#command(
      context,
      providerResource(provider.id),
      () => context.runtime.providers.update(submitted.toUpdate(provider)),
      (saved) => {
        this.#acceptProvider(saved);
        const current = this.#snapshot.drafts.get(provider.id) ?? submitted;
        const settled = current.settle(submitted);
        const drafts = new Map(this.#snapshot.drafts);
        if (settled.dirty(saved)) drafts.set(provider.id, settled);
        else drafts.delete(provider.id);
        this.#snapshot = { ...this.#snapshot, drafts };
        if (current === submitted)
          this.#feedback(providerResource(provider.id), { kind: "saved" });
      },
      { provider: provider.id, draft: submitted },
    );
  }

  clearKey(context: ComponentContext, provider: string) {
    this.#assert(context);
    if (this.#snapshot.probing.has(provider)) return Promise.resolve(false);
    return this.#command(
      context,
      providerResource(provider),
      () =>
        context.runtime.providers.update({
          provider,
          apiKey: { type: "clear" },
        }),
      (saved) => this.#acceptProvider(saved),
    );
  }

  setRole(
    context: ComponentContext,
    kind: "utility" | "embedding",
    value: UtilityRole | EmbeddingRole,
  ) {
    return this.#command(
      context,
      kind,
      () =>
        kind === "utility"
          ? context.runtime.models.setUtilityRole(value)
          : context.runtime.models.setEmbeddingRole(value),
      (saved) => {
        this.#cancelRead(kind);
        this.#snapshot = {
          ...this.#snapshot,
          [kind]: { phase: "ready", data: saved },
        };
      },
    );
  }

  async test(
    context: ComponentContext,
    provider: Provider,
    draft: ProviderCredentialsDraft,
  ) {
    this.#assert(context);
    const resource = providerResource(provider.id);
    if (this.#pending.has(resource) || this.#snapshot.probing.has(provider.id))
      return;
    const probing = new Set(this.#snapshot.probing).add(provider.id);
    this.#snapshot = { ...this.#snapshot, probing };
    this.#feedback(resource);
    this.#publish();
    const witness = this.#draftEdits.get(provider.id);
    try {
      if (
        draft.dirty(provider) &&
        !(await this.#save(context, provider, draft))
      )
        return;
      context.signal.throwIfAborted();
      this.#feedback(resource);
      this.#publish();
      const signal = AbortSignal.any([this.#signal, context.signal]);
      const result = await context.track(() =>
        context.runtime.providers.test(provider.id, signal),
      );
      if (!signal.aborted && this.#draftEdits.get(provider.id) === witness)
        this.#feedback(resource, { kind: "tested", outcome: result.outcome });
    } catch (error) {
      if (
        !this.#signal.aborted &&
        !context.signal.aborted &&
        this.#draftEdits.get(provider.id) === witness
      )
        this.#feedback(resource, { kind: "error", error });
    } finally {
      if (!this.#signal.aborted) {
        const probing = new Set(this.#snapshot.probing);
        probing.delete(provider.id);
        this.#snapshot = { ...this.#snapshot, probing };
        this.#publish();
      }
    }
  }

  recover(context: ComponentContext, resource: string) {
    this.#assert(context);
    const pending = this.#pending.get(resource);
    if (!pending || this.#snapshot.commands.get(resource)?.phase !== "unknown")
      return;
    return this.#observe(context, resource, pending, true);
  }

  #command<T>(
    context: ComponentContext,
    resource: string,
    execute: () => MutationPromise<T>,
    accept: (result: T) => void,
    witness?: Pending["witness"],
  ) {
    this.#assert(context);
    if (this.#pending.has(resource)) return Promise.resolve(false);
    this.#feedback(resource);
    try {
      const original = context.track(execute);
      const pending: Pending = {
        original,
        accept: (result) => accept(result as T),
        witness,
      };
      this.#pending.set(resource, pending);
      return this.#observe(context, resource, pending, false);
    } catch (error) {
      this.#feedback(resource, { kind: "error", error });
      this.#publish();
      return Promise.resolve(false);
    }
  }

  async #observe(
    context: ComponentContext,
    resource: string,
    pending: Pending,
    recover: boolean,
  ) {
    this.#setCommand(resource, { phase: "running" });
    try {
      const result = await (recover
        ? context.track(() =>
            pending.original.retry({ signal: context.signal }),
          )
        : pending.original);
      if (this.#signal.aborted || this.#pending.get(resource) !== pending)
        return false;
      this.#pending.delete(resource);
      pending.accept(result);
      this.#setCommand(resource);
      if (resource === "utility" || resource === "embedding")
        this.refreshRead(context, resource);
      else {
        this.refreshRead(context, "providers");
        this.refreshRead(context, "models");
      }
      return true;
    } catch (error) {
      if (this.#signal.aborted || this.#pending.get(resource) !== pending)
        return false;
      if (
        mutationSettlementIsUnknown(error) ||
        (recover && context.signal.aborted)
      )
        this.#setCommand(resource, { phase: "unknown", error });
      else {
        this.#pending.delete(resource);
        if (
          !pending.witness ||
          this.#snapshot.drafts.get(pending.witness.provider) ===
            pending.witness.draft
        )
          this.#feedback(resource, { kind: "error", error });
        this.#setCommand(resource);
      }
      return false;
    }
  }

  #assert(context: ComponentContext) {
    this.#signal.throwIfAborted();
    context.signal.throwIfAborted();
    if (
      context.runtime !== this.#runtimeIdentity ||
      context.component.installationId !== this.#component.installationId ||
      context.component.digest !== this.#component.digest ||
      context.component.componentId !== this.#component.componentId
    )
      throw new Error("provider settings belong to another installed source");
  }

  #acceptProvider(saved: Provider) {
    this.#cancelRead("providers");
    const providers = this.#snapshot.providers.data;
    if (providers !== undefined)
      this.#snapshot = {
        ...this.#snapshot,
        providers: {
          phase: "ready",
          data: providers.map((provider) =>
            provider.id === saved.id ? saved : provider,
          ),
        },
      };
  }

  #cancelRead(key: ReadKey) {
    this.#reads.get(key)?.abort();
    this.#reads.delete(key);
  }

  #setCommand(resource: string, value?: Command) {
    const commands = new Map(this.#snapshot.commands);
    if (value) commands.set(resource, value);
    else commands.delete(resource);
    this.#snapshot = { ...this.#snapshot, commands };
    this.#publish();
  }

  #feedback(resource: string, value?: Feedback) {
    const feedback = new Map(this.#snapshot.feedback);
    if (value) feedback.set(resource, value);
    else feedback.delete(resource);
    this.#snapshot = { ...this.#snapshot, feedback };
  }

  #publish() {
    for (const listener of this.#listeners) listener();
  }
}

const readers: {
  [K in ReadKey]: (
    client: FlameClient,
    signal: AbortSignal,
  ) => Promise<Results[K]>;
} = {
  providers: async (client, signal) =>
    (await client.providers.list(signal)).data,
  models: async (client, signal) => {
    const providers = (await client.providers.list(signal)).data.filter(
      (provider) => provider.configured,
    );
    const pages = await Promise.all(
      providers.map((provider) => client.models.list(provider.id, signal)),
    );
    return pages.flatMap((page) => page.data);
  },
  utility: (client, signal) => client.models.getUtilityRole(signal),
  embedding: (client, signal) => client.models.getEmbeddingRole(signal),
};
