import { createSingletonPort } from "@/lib/ports/singletonPort";

interface RuntimeEndpointConfiguration {
  read(): string | undefined;
  replace(endpoint: string): void;
}

const port = createSingletonPort<RuntimeEndpointConfiguration>(
  "Runtime endpoint configuration is not installed",
);

export const configureRuntimeEndpoint = port.configure;
export const runtimeEndpointConfiguration = port.get;
export const configuredRuntimeEndpoint = port.peek;
