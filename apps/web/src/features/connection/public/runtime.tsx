import { createContext, useContext, type ReactNode } from "react";
import {
  type RuntimeCapabilityPort,
  type RuntimeServiceStatusPort,
} from "@/features/connection/application/runtimeService";
import { type RuntimeEndpoint } from "@/features/connection/application/runtimeEndpoint";
export interface RuntimeAccess {
  readonly capabilities: RuntimeCapabilityPort;
  readonly status: RuntimeServiceStatusPort;
  readonly endpoint: RuntimeEndpoint;
}
const RuntimeContext = createContext<RuntimeAccess | null>(null);
export function RuntimeProvider({
  value,
  children,
}: {
  value: RuntimeAccess;
  children: ReactNode;
}) {
  return <RuntimeContext value={value}>{children}</RuntimeContext>;
}
export function useRuntime(): RuntimeAccess {
  const value = useContext(RuntimeContext);
  if (!value) throw new Error("RuntimeProvider is missing");
  return value;
}
