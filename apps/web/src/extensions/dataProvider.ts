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

export function requiredParams<P>(key: string, params: unknown): P {
  if (params === undefined) throw new Error(`Data provider "${key}" requires parameters`);
  return params as P;
}

export function pageData<T>(request: Promise<{ data: T[] }>): Promise<T[]> {
  return request.then((page) => page.data);
}
