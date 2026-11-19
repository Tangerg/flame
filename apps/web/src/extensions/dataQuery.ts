import { useInfiniteQuery } from "@tanstack/react-query";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { useMemo, useSyncExternalStore } from "react";
import { type Query, type QueryCache, type UseQueryResult } from "@tanstack/react-query";
import { queryOptions, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Host } from "dougong";
import { queryClient } from "@/lib/queryClient";
import { type Contribution } from "@/extensions/contracts";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { contributionsTo, publishedKernel, subscribeContributions } from "@/extensions/kernel";
import { type DataProviderSpec } from "@/extensions/dataProvider";
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
    // Reset cancels reads and clears objects retained by continuous placeholders before eviction.
    void queryClient.resetQueries({ predicate: affectedQuery });
    queryClient.removeQueries({
      predicate: (query) => {
        const key = query.queryKey[0];
        // Mounted observers need their cache entry to receive a returning provider.
        return (
          affectedQuery(query) &&
          typeof key === "string" &&
          !successor.has(key) &&
          query.getObserversCount() === 0
        );
      },
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

export interface ParameterizedQueryOptions<P, T> {
  refetchInterval?: (data: T | undefined) => number | false;
  // Keep the prior value across a key change the predicate deems continuous (a
  // widened window of the same file), so the view holds its content and scroll
  // instead of flashing a loading state.
  keepWhenContinuous?: (previous: P, next: P) => boolean;
}

export function parameterizedQuery<P, T>(key: string, params: P | undefined) {
  return queryOptions({
    queryKey: [key, params],
    queryFn: ({ signal }) => dataQueryOwner.load<T, P>(key, params, signal),
    enabled: params !== undefined,
    ...STATIC_QUERY_OPTIONS,
  });
}

function useQueryCacheState<T>(cache: QueryCache, queryHash: string | undefined) {
  const readState = useMemo(
    () => ({
      subscribe: (onChange: () => void) => {
        if (queryHash === undefined) return () => undefined;
        return cache.subscribe((event) => {
          if (event.query.queryHash === queryHash) onChange();
        });
      },
      getSnapshot: () => (queryHash === undefined ? undefined : cache.get<T>(queryHash)?.state),
    }),
    [cache, queryHash],
  );
  return useSyncExternalStore(readState.subscribe, readState.getSnapshot);
}

export function useCachedParameterizedData<P, T>(
  key: string,
  params: P | undefined,
): T | undefined {
  const client = useQueryClient();
  const queryHash =
    params === undefined
      ? undefined
      : client.defaultQueryOptions(parameterizedQuery<P, T>(key, params)).queryHash;
  return useQueryCacheState<T>(client.getQueryCache(), queryHash)?.data;
}

export function createParameterizedDataQuery<P, T>(
  key: string,
  options?: ParameterizedQueryOptions<P, T>,
): (params: P | undefined, subscribed?: boolean) => UseQueryResult<T> {
  const interval = options?.refetchInterval;
  const keep = options?.keepWhenContinuous;
  const useData = (query: ReturnType<typeof parameterizedQuery<P, T>>, subscribed: boolean) =>
    useQuery({
      ...query,
      subscribed,
      refetchInterval: interval ? (query) => interval(query.state.data) : undefined,
    });
  if (!keep)
    return (params, subscribed = true) =>
      useData(parameterizedQuery<P, T>(key, params), subscribed);
  return (params, subscribed = true) => {
    const client = useQueryClient();
    const query = parameterizedQuery<P, T>(key, params);
    const queryHash = client.defaultQueryOptions(query).queryHash;
    const cache = client.getQueryCache();
    // Observer placeholders can outlive a reset while effects are disconnected.
    // Native cache state makes reconnection recompute continuity after reset or eviction.
    useQueryCacheState<T>(cache, queryHash);
    return useData(
      queryOptions({
        ...query,
        placeholderData: (_previous, previousQuery) => {
          if (!previousQuery) return undefined;
          const prior = previousQuery.queryKey[1] as P | undefined;
          // Observers can retain evicted Queries; continuity belongs to the current cache entry.
          return prior !== undefined && params !== undefined && keep(prior, params)
            ? cache.get<typeof _previous>(previousQuery.queryHash)?.state.data
            : undefined;
        },
      }),
      subscribed,
    );
  };
}

export function createPagedDataQuery<
  P extends { cursor?: string },
  T extends { nextCursor?: string },
>(key: string) {
  return (params: P | undefined, subscribed = true) =>
    useInfiniteQuery({
      queryKey: [key, params, "pages"],
      initialPageParam: undefined as string | undefined,
      queryFn: ({ pageParam, signal }) =>
        dataQueryOwner.load<T, P>(
          key,
          params === undefined ? undefined : { ...params, cursor: pageParam },
          signal,
        ),
      getNextPageParam: (page) => page.nextCursor,
      enabled: params !== undefined && subscribed,
      subscribed,
      ...STATIC_QUERY_OPTIONS,
    });
}
