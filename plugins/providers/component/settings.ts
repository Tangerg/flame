import {
  mutationSettlementIsUnknown,
  type ComponentContext,
  type ComponentSource,
  watchRuntimeTopics,
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

type SettingsContext = Pick<
  ComponentContext,
  "component" | "runtime" | "signal" | "track"
>;

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

const ROLE_KINDS = ["utility", "embedding"] as const;
export type RoleKind = (typeof ROLE_KINDS)[number];

function isRoleKind(resource: string): resource is RoleKind {
  return (ROLE_KINDS as readonly string[]).includes(resource);
}
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
interface CommandObservation {
  readonly original: MutationPromise<unknown>;
  readonly submission?: CredentialSubmission;
}

interface CredentialSubmission {
  readonly providerId: string;
  readonly revision: symbol | undefined;
  settle(): void;
}

type TrackedCommand = Command & { readonly observation: CommandObservation };

interface SettingsState extends Omit<Snapshot, "commands"> {
  readonly commands: ReadonlyMap<string, TrackedCommand>;
}

export const providerResource = (id: string) =>
  JSON.stringify(["provider", id]);

function loadingSnapshot(): SettingsState {
  return {
    providers: { phase: "loading" },
    models: { phase: "loading" },
    utility: { phase: "loading" },
    embedding: { phase: "loading" },
    drafts: new Map(),
    commands: new Map(),
    feedback: new Map(),
    probing: new Set(),
  };
}

export class ProviderSettings {
  readonly #runtimeIdentity: object;
  readonly #component: ComponentSource["component"];
  readonly #signal: AbortSignal;
  readonly #listeners = new Set<() => void>();
  readonly #reads = new Map<ReadKey, AbortController>();
  #snapshot = loadingSnapshot();

  constructor(source: ComponentSource) {
    source.signal.throwIfAborted();
    this.#runtimeIdentity = source.runtimeIdentity;
    this.#component = source.component;
    this.#signal = source.signal;
    this.#signal.addEventListener(
      "abort",
      () => {
        for (const controller of this.#reads.values()) controller.abort();
        this.#reads.clear();
        this.#snapshot = loadingSnapshot();
        this.#publish();
      },
      { once: true },
    );
  }

  getSnapshot = (): Snapshot => this.#snapshot;
  subscribe = (listener: () => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  observe(context: SettingsContext) {
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
        await watchRuntimeTopics(
          context.runtime,
          { topics: ["models.changed", "plugins.changed"] },
          signal,
          () => this.refresh(context, signal),
        );
      } catch (error) {
        if (!signal.aborted) {
          this.#snapshot = { ...this.#snapshot, observationFailure: { error } };
          this.#publish();
        }
      }
    });
    return () => controller.abort();
  }

  refresh(context: SettingsContext, signal = context.signal) {
    for (const key of ["providers", "models", ...ROLE_KINDS] as const)
      this.refreshRead(context, key, signal);
  }

  refreshRead<K extends ReadKey>(
    context: SettingsContext,
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
    context: SettingsContext,
    provider: Provider,
    draft: ProviderCredentialsDraft,
  ) {
    this.#assert(context);
    const resource = providerResource(provider.id);
    // A value equal to the current provider may be a correction to a pending save.
    const savingCredentials =
      this.#snapshot.commands.get(resource)?.observation.submission !==
      undefined;
    this.#storeDraft(
      provider.id,
      savingCredentials ? draft : draft.rebase(provider),
    );
    this.#feedback(resource);
    this.#publish();
  }

  save(
    context: SettingsContext,
    provider: Provider,
    submitted: ProviderCredentialsDraft,
  ) {
    this.#assert(context);
    if (this.#snapshot.probing.has(provider.id)) return Promise.resolve(false);
    return this.#save(context, provider, submitted);
  }

  #save(
    context: SettingsContext,
    provider: Provider,
    submitted: ProviderCredentialsDraft,
  ) {
    return this.#command(
      context,
      providerResource(provider.id),
      () => context.runtime.providers.update(submitted.toUpdate(provider)),
      {
        providerId: provider.id,
        revision: submitted.revision,
        settle: () => {
          const current = this.#snapshot.drafts.get(provider.id) ?? submitted;
          this.#storeDraft(provider.id, current.settle(submitted));
          if (current.revision === submitted.revision)
            this.#feedback(providerResource(provider.id), { kind: "saved" });
        },
      },
    );
  }

  clearKey(context: SettingsContext, provider: string) {
    this.#assert(context);
    if (this.#snapshot.probing.has(provider)) return Promise.resolve(false);
    return this.#command(context, providerResource(provider), () =>
      context.runtime.providers.update({
        provider,
        apiKey: { type: "clear" },
      }),
    );
  }

  setRole(
    context: SettingsContext,
    kind: RoleKind,
    value: UtilityRole | EmbeddingRole,
  ) {
    return this.#command(context, kind, () =>
      kind === "utility"
        ? context.runtime.models.setUtilityRole(value)
        : context.runtime.models.setEmbeddingRole(value),
    );
  }

  async test(
    context: SettingsContext,
    provider: Provider,
    draft: ProviderCredentialsDraft,
  ) {
    this.#assert(context);
    const resource = providerResource(provider.id);
    if (
      this.#snapshot.commands.has(resource) ||
      this.#snapshot.probing.has(provider.id)
    )
      return;
    const probing = new Set(this.#snapshot.probing).add(provider.id);
    this.#snapshot = { ...this.#snapshot, probing };
    this.#feedback(resource);
    this.#publish();
    const revision = this.#snapshot.drafts.get(provider.id)?.revision;
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
      if (
        !signal.aborted &&
        this.#snapshot.drafts.get(provider.id)?.revision === revision
      )
        this.#feedback(resource, { kind: "tested", outcome: result.outcome });
    } catch (error) {
      if (
        !this.#signal.aborted &&
        !context.signal.aborted &&
        this.#snapshot.drafts.get(provider.id)?.revision === revision
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

  recover(context: SettingsContext, resource: string) {
    this.#assert(context);
    const command = this.#snapshot.commands.get(resource);
    if (command?.phase !== "unknown") return;
    return this.#observe(context, resource, command.observation, true);
  }

  #command(
    context: SettingsContext,
    resource: string,
    execute: () => MutationPromise<unknown>,
    submission?: CredentialSubmission,
  ) {
    this.#assert(context);
    if (this.#snapshot.commands.has(resource)) return Promise.resolve(false);
    this.#feedback(resource);
    try {
      const original = context.track(execute);
      const observation: CommandObservation = { original, submission };
      return this.#observe(context, resource, observation, false);
    } catch (error) {
      this.#feedback(resource, { kind: "error", error });
      this.#publish();
      return Promise.resolve(false);
    }
  }

  async #observe(
    context: SettingsContext,
    resource: string,
    observation: CommandObservation,
    recover: boolean,
  ) {
    const running: TrackedCommand = { phase: "running", observation };
    this.#setCommand(resource, running);
    this.#publish();
    try {
      await (recover
        ? context.track(() =>
            observation.original.retry({ signal: context.signal }),
          )
        : observation.original);
    } catch (error) {
      if (this.#snapshot.commands.get(resource) !== running) return false;
      if (
        mutationSettlementIsUnknown(error) ||
        (recover && context.signal.aborted)
      )
        this.#setCommand(resource, { phase: "unknown", observation, error });
      else {
        const submitted = observation.submission;
        if (
          !submitted ||
          this.#snapshot.drafts.get(submitted.providerId)?.revision ===
            submitted.revision
        )
          this.#feedback(resource, { kind: "error", error });
        this.#setCommand(resource);
      }
      this.#publish();
      return false;
    }
    if (this.#snapshot.commands.get(resource) !== running) return false;
    this.#setCommand(resource);
    observation.submission?.settle();
    this.#publish();
    if (isRoleKind(resource)) this.refreshRead(context, resource);
    else {
      this.refreshRead(context, "providers");
      this.refreshRead(context, "models");
    }
    return true;
  }

  #assert(context: SettingsContext) {
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

  #cancelRead(key: ReadKey) {
    this.#reads.get(key)?.abort();
    this.#reads.delete(key);
  }

  #storeDraft(providerId: string, draft: ProviderCredentialsDraft) {
    const drafts = new Map(this.#snapshot.drafts).set(providerId, draft);
    this.#snapshot = { ...this.#snapshot, drafts };
  }

  #setCommand(resource: string, value?: TrackedCommand) {
    const commands = new Map(this.#snapshot.commands);
    if (value) commands.set(resource, value);
    else commands.delete(resource);
    this.#snapshot = { ...this.#snapshot, commands };
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
