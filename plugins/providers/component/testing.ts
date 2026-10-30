import { vi } from "vitest";
import type { ComponentContext } from "@flame/component-sdk";
import type { MutationPromise } from "@flame/runtime-client";
import type {
  Model,
  Provider,
  ProviderTestResult,
  UpdateProviderRequest,
  UtilityRole,
} from "@flame/runtime-client/wire";

export const provider: Provider = {
  id: "openai-compatible",
  configured: true,
  credentialRequirement: "apiKeyOptional",
  baseUrl: "https://old.example.test/v1",
  requiresBaseUrl: true,
  embeddingCapable: true,
  defaultEmbeddingModel: "embed-default",
};
export const model: Model = {
  provider: provider.id,
  id: "model-one",
  displayName: "Shared name",
};

export function mutation<T>(
  promise: Promise<T>,
  retry = vi.fn<MutationPromise<T>["retry"]>(),
): MutationPromise<T> {
  return Object.assign(promise, { idempotencyKey: "original-command", retry });
}

export function fixture(initial = [provider]) {
  const source = new AbortController();
  const views: AbortController[] = [];
  let providers = initial;
  let utility: UtilityRole = { provider: provider.id, model: model.id };
  let embedding: UtilityRole = {};
  const list = vi.fn(async (_signal?: AbortSignal) => ({ data: providers }));
  const update = vi.fn(
    (input: UpdateProviderRequest): MutationPromise<Provider> => {
      const saved: Provider = {
        ...providers.find((value) => value.id === input.provider)!,
        credential:
          input.apiKey?.type === "clear"
            ? undefined
            : providers.find((value) => value.id === input.provider)
                ?.credential,
        baseUrl: input.baseUrl
          ? input.baseUrl.type === "set"
            ? input.baseUrl.value
            : undefined
          : providers.find((value) => value.id === input.provider)?.baseUrl,
      };
      providers = providers.map((value) =>
        value.id === saved.id ? saved : value,
      );
      return mutation(Promise.resolve(saved));
    },
  );
  const test = vi
    .fn<
      (_provider: string, _signal?: AbortSignal) => Promise<ProviderTestResult>
    >()
    .mockResolvedValue({ outcome: "reachable" });
  const models = vi.fn(async (_provider: string, _signal?: AbortSignal) => ({
    data: [model],
  }));
  const getUtilityRole = vi.fn(
    async (_signal?: AbortSignal): Promise<UtilityRole> => utility,
  );
  const getEmbeddingRole = vi.fn(
    async (_signal?: AbortSignal): Promise<UtilityRole> => embedding,
  );
  const setUtilityRole = vi.fn((value: UtilityRole) => {
    utility = value;
    return mutation(Promise.resolve(value));
  });
  const setEmbeddingRole = vi.fn((value: UtilityRole) => {
    embedding = value;
    return mutation(Promise.resolve(value));
  });
  const subscribe = vi.fn(
    (_request: unknown, signal: AbortSignal) =>
      new Promise<never>((_resolve, reject) =>
        signal.addEventListener("abort", () => reject(signal.reason), {
          once: true,
        }),
      ),
  );
  const runtime = {
    providers: { list, update, test },
    models: {
      list: models,
      getUtilityRole,
      getEmbeddingRole,
      setUtilityRole,
      setEmbeddingRole,
    },
    runtimeEvents: { subscribe },
  };
  const tracked: Promise<unknown>[] = [];
  function openView(lifetimeSignal = source.signal) {
    const view = new AbortController();
    views.push(view);
    const signal = AbortSignal.any([lifetimeSignal, view.signal]);
    const context = {
      runtime,
      component: {
        installationId: "captured-installation",
        digest: "captured-release",
        componentId: "settings",
      },
      locale: "en",
      signal,
      track: <P extends Promise<unknown>>(
        operation: (signal: AbortSignal) => P,
      ): P => {
        const original = operation(signal);
        tracked.push(original);
        return original;
      },
    } as unknown as ComponentContext;
    return { context, view };
  }
  return {
    ...openView(),
    source,
    list,
    update,
    test,
    models,
    getUtilityRole,
    getEmbeddingRole,
    setUtilityRole,
    setEmbeddingRole,
    subscribe,
    tracked,
    openView,
    dispose() {
      source.abort();
      for (const view of views) view.abort();
    },
  };
}
