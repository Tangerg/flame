import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * Deliberately says nothing about Zustand, Host configuration or persistence — those are
 * adapter mechanisms, and the use case only reads and replaces one value.
 */
export interface RuntimeEndpointConfiguration {
  read(): string | undefined;
  /** Atomically replace the configured server scope and its live connection. */
  replace(endpoint: string): void;
}

const port = createSingletonPort<RuntimeEndpointConfiguration>(
  "Runtime endpoint configuration is not installed",
);

export const configureRuntimeEndpoint = port.configure;
export const runtimeEndpointConfiguration = port.get;
export const configuredRuntimeEndpoint = port.peek;
