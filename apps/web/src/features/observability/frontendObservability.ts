import { type ConfigService } from "@/extensions/config";
import { type ObservabilityTeardown } from "@/features/observability/observabilityLifecycle";
export async function initFrontendObservability(
  service: {
    name: string;
    version: string;
  },
  config: ConfigService,
): Promise<ObservabilityTeardown> {
  const { setupObservability } = await import("@/lib/observability/setup");
  const configuredEndpoint = config.get("otel.endpoint");
  return setupObservability({
    serviceName: service.name,
    serviceVersion: service.version,
    otlpEndpoint: typeof configuredEndpoint === "string" ? configuredEndpoint : undefined,
  });
}
