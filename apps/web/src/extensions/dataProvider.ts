import { defineExtensionPoint } from "@/extensions/contracts";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
export interface DataProviderSpec<T = unknown, P = unknown> {
  key: string;
  fetcher: (params?: P, signal?: AbortSignal) => Promise<T>;
}

export const DATA_PROVIDER = defineExtensionPoint<DataProviderSpec>({
  id: "flame.data.provider",
  keying: "single",
  keyOf: (s) => s.key,
});

export function lookupDataProvider<T = unknown, P = unknown>(
  key: string,
): ((params?: P, signal?: AbortSignal) => Promise<T>) | undefined {
  const spec = lookupExtensionByKey(DATA_PROVIDER, key);
  return spec ? (spec.fetcher as (params?: P, signal?: AbortSignal) => Promise<T>) : undefined;
}
