import { getConfig } from "@/extensions/config";
import { type ObservabilityTeardown } from "@/features/observability/observabilityLifecycle";
export async function initFrontendObservability(service: {
  name: string;
  version: string;
}): Promise<ObservabilityTeardown> {
  const { setupObservability, teardownObservability } = await import("@/lib/observability/setup");
  const configuredEndpoint = getConfig("otel.endpoint");
  await setupObservability({
    serviceName: service.name,
    serviceVersion: service.version,
    otlpEndpoint: typeof configuredEndpoint === "string" ? configuredEndpoint : undefined,
  });
  return teardownObservability;
}
