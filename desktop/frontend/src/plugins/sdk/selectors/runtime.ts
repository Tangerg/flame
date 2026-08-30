import type {
  AgentRunStartOptions,
  AgentRunOptionsProviderSpec,
  AgentSourceSpec,
  PluginErrorFallbackSpec,
} from "../types";
import { AGENT_RUN_OPTIONS, AGENT_SOURCE, DATA_PROVIDER, ERROR_FALLBACK } from "../kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "./extensions";

/** Highest priority wins, ties broken by insertion order. */
export function pickAgentSource(): AgentSourceSpec | undefined {
  const sources = lookupExtensionPoint(AGENT_SOURCE);
  if (sources.length === 0) return undefined;
  return sources.reduce((best, cur) => ((cur.priority ?? 0) > (best.priority ?? 0) ? cur : best));
}

function pickAgentRunOptionsProvider(): AgentRunOptionsProviderSpec | undefined {
  const providers = lookupExtensionPoint(AGENT_RUN_OPTIONS);
  if (providers.length === 0) return undefined;
  return providers.reduce((best, cur) =>
    (cur.priority ?? 0) >= (best.priority ?? 0) ? cur : best,
  );
}

export function resolveAgentRunStartOptions(): AgentRunStartOptions {
  return pickAgentRunOptionsProvider()?.resolve() ?? {};
}

/** The type is ERASED so every provider fits one map; callers cast on the way out. */
export function lookupDataProvider<T = unknown, P = unknown>(
  key: string,
): ((params?: P, signal?: AbortSignal) => Promise<T>) | undefined {
  const spec = lookupExtensionByKey(DATA_PROVIDER, key);
  return spec ? (spec.fetcher as (params?: P, signal?: AbortSignal) => Promise<T>) : undefined;
}

/** Ties resolve by insertion order, so the later registration wins. */
export function pickPluginErrorFallback(): PluginErrorFallbackSpec | undefined {
  const specs = lookupExtensionPoint(ERROR_FALLBACK);
  if (specs.length === 0) return undefined;
  return specs.reduce((best, cur) => ((cur.priority ?? 0) >= (best.priority ?? 0) ? cur : best));
}
