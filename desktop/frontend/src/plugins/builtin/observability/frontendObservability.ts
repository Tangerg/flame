import { CLIENT_VERSION } from "@/main/config";
import { getConfig } from "@/plugins/sdk/config";
import type { ObservabilityTeardown } from "./observabilityLifecycle";

export async function initFrontendObservability(): Promise<ObservabilityTeardown> {
  const { setupObservability, teardownObservability } = await import("@/lib/observability/setup");
  const configuredEndpoint = getConfig("otel.endpoint");
  await setupObservability({
    serviceName: "flame-frontend",
    serviceVersion: CLIENT_VERSION,
    otlpEndpoint: typeof configuredEndpoint === "string" ? configuredEndpoint : undefined,
  });
  return teardownObservability;
}
