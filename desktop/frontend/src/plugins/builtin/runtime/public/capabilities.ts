import { runtimeCapabilities, type RuntimeCapabilityPort } from "../application/ports/capabilities";

export { negotiatedCapabilities } from "../application/ports/capabilities";

export const useRuntimeCapability: RuntimeCapabilityPort["useCapability"] = (capability) =>
  runtimeCapabilities().useCapability(capability);

export const runtimeCapability: RuntimeCapabilityPort["hasCapability"] = (capability) =>
  runtimeCapabilities().hasCapability(capability);

export function runtimeSupportsStreamingMethod(method: string): boolean {
  return runtimeCapabilities().supportsStreamingMethod(method);
}

export const runtimeSupportsTopic: RuntimeCapabilityPort["supportsRuntimeTopic"] = (topic) =>
  runtimeCapabilities().supportsRuntimeTopic(topic);
