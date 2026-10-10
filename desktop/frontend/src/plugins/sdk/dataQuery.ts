import { GenerationRetiredError } from "@/lib/asyncOwnership";
import type { Query, UseQueryResult } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import type { Host } from "dougong";
import { queryClient } from "@/lib/queryClient";
import type { Contribution } from "./contracts";
import { DATA_PROVIDER } from "./kernelPoints";
import { contributionsTo, publishedKernel, subscribeContributions } from "./kernel";
import type { DataProviderSpec } from "./types";

const STATIC_QUERY_OPTIONS = {
  staleTime: 5 * 60_000,
  refetchOnWindowFocus: false as const,
};

class DataProviderGeneration {
  readonly #lifetime = new AbortController();

  constructor(readonly provider: DataProviderSpec) {}

  async load<T, P>(params: P | undefined, querySignal: AbortSignal): Promise<T> {
    this.#lifetime.signal.throwIfAborted();
    const attempt = AbortSignal.any([querySignal, this.#lifetime.signal]);
    const value = await (this.provider.fetcher as (params?: P, signal?: AbortSignal) => Promise<T>)(
      params,
      attempt,
    );
    this.#lifetime.signal.throwIfAborted();
    return value;
  }

  retire(): void {
    this.#lifetime.abort(new GenerationRetiredError("data_provider_generation"));
  }
}

class DataQueryOwner {
  #owner: Host | undefined = publishedKernel();
  #entries: ReadonlyArray<Contribution<DataProviderSpec>> = contributionsTo(DATA_PROVIDER);
  #providers = new Map(
    this.#entries.map(({ key, item }) => [key, new DataProviderGeneration(item)]),
  );
  readonly #unsubscribe: () => void;
  #disposed = false;

  constructor() {
    this.#unsubscribe = subscribeContributions(() => this.#reconcileProviders());
  }

  load<T, P>(key: string, params: P | undefined, signal: AbortSignal): Promise<T> {
    const generation = this.#providers.get(key);
    if (!generation)
      return Promise.reject(new Error(`No data provider registered for key "${key}"`));
    return generation.load<T, P>(params, signal);
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#unsubscribe();
    for (const generation of this.#providers.values()) generation.retire();
  }

  #reconcileProviders(): void {
    if (this.#disposed) return;
    const owner = publishedKernel();
    const entries = contributionsTo(DATA_PROVIDER);
    if (owner === this.#owner && entries === this.#entries) return;
    const predecessor = this.#providers;
    const successor = new Map(
      entries.map(({ key, item }) => {
        const previous = predecessor.get(key);
        return [
          key,
          owner === this.#owner && previous?.provider === item
            ? previous
            : new DataProviderGeneration(item),
        ] as const;
      }),
    );
    this.#owner = owner;
    this.#entries = entries;
    this.#providers = successor;
    const affected = new Set<string>();
    for (const key of new Set([...predecessor.keys(), ...successor.keys()])) {
      if (predecessor.get(key) === successor.get(key)) continue;
      affected.add(key);
      predecessor.get(key)?.retire();
    }
    this.#replaceCachedWriters(affected, successor);
  }

  #replaceCachedWriters(
    affected: ReadonlySet<string>,
    successor: ReadonlyMap<string, DataProviderGeneration>,
  ): void {
    if (affected.size === 0) return;
    const affectedQuery = (query: Query) => {
      const key = query.queryKey[0];
      return typeof key === "string" && affected.has(key);
    };
    const predecessorQueries = new Set(
      queryClient.getQueryCache().findAll({ predicate: affectedQuery }),
    );
    if (predecessorQueries.size === 0) return;
    void queryClient
      .cancelQueries({ predicate: (query) => predecessorQueries.has(query) })
      .then(() => {
        if (this.#disposed) return;
        const ownedQuery = (query: Query) => {
          const key = query.queryKey[0];
          return (
            predecessorQueries.has(query) &&
            typeof key === "string" &&
            this.#providers.get(key) === successor.get(key)
          );
        };
        queryClient.removeQueries({
          predicate: (query) => {
            const key = query.queryKey[0];
            return ownedQuery(query) && typeof key === "string" && !successor.has(key);
          },
        });
        void queryClient.resetQueries({
          predicate: (query) => {
            const key = query.queryKey[0];
            return ownedQuery(query) && typeof key === "string" && successor.has(key);
          },
        });
      });
  }
}

const dataQueryOwner = new DataQueryOwner();

if (import.meta.hot) import.meta.hot.dispose(() => dataQueryOwner.dispose());

export function createDataQuery<T>(key: string): () => UseQueryResult<T> {
  return () =>
    useQuery({
      queryKey: [key],
      queryFn: ({ signal }) => dataQueryOwner.load<T, void>(key, undefined, signal),
      ...STATIC_QUERY_OPTIONS,
    });
}

export interface ParameterizedQueryOptions<T> {
  refetchInterval?: (data: T | undefined) => number | false;
}

export function createParameterizedDataQuery<P, T>(
  key: string,
  options?: ParameterizedQueryOptions<T>,
): (params: P | undefined) => UseQueryResult<T> {
  const interval = options?.refetchInterval;
  return (params) =>
    useQuery({
      queryKey: [key, params],
      queryFn: ({ signal }) => dataQueryOwner.load<T, P>(key, params, signal),
      enabled: params !== undefined,
      ...STATIC_QUERY_OPTIONS,
      refetchInterval: interval ? (query) => interval(query.state.data) : undefined,
    });
}
