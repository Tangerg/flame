import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { RuntimeAgentDriver } from "./runtimeDriver";
export interface RuntimeOpeningRetries {
  retry(key: string, signal: AbortSignal): ReturnType<RuntimeAgentDriver["resume"]>;
}
export const { get: runtimeOpeningRetries, configure: configureRuntimeOpeningRetries } =
  createSingletonPort<RuntimeOpeningRetries>("agent runtime opening retries");
