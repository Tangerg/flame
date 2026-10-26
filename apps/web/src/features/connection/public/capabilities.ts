import { type RuntimeAccess, useRuntime } from "@/features/connection/public/runtime";
export function useRuntimeCapability(
  capability: Parameters<RuntimeAccess["capabilities"]["hasCapability"]>[0],
): boolean {
  return useRuntime().capabilities.useCapability(capability);
}
export function runtimeCapability(
  runtime: RuntimeAccess,
  capability: Parameters<RuntimeAccess["capabilities"]["hasCapability"]>[0],
): boolean {
  return runtime.capabilities.hasCapability(capability);
}
export function runtimeSupportsStreamingMethod(runtime: RuntimeAccess, method: string): boolean {
  return runtime.capabilities.supportsStreamingMethod(method);
}
export function runtimeSupportsTopic(runtime: RuntimeAccess, topic: string): boolean {
  return runtime.capabilities.supportsRuntimeTopic(topic);
}
