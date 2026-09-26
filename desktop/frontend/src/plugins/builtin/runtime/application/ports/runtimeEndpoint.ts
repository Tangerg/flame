import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface RuntimeEndpointTarget {
  endpoint: string;
  localToken?: string;
}

interface RuntimeEndpointConfiguration {
  read(): RuntimeEndpointTarget;
  defaultTarget(): RuntimeEndpointTarget;
  replace(target: RuntimeEndpointTarget): void;
}

const port = createSingletonPort<RuntimeEndpointConfiguration>(
  "Runtime endpoint configuration is not installed",
);

export const configureRuntimeEndpoint = port.configure;
export const runtimeEndpointConfiguration = port.get;
export const configuredRuntimeEndpoint = port.peek;
