import { createContext, useContext, type ReactNode } from "react";
import { type AgentAccess } from "@/features/agent/application/agentAccess";
export type { AgentAccess } from "@/features/agent/application/agentAccess";
const AgentContext = createContext<AgentAccess | null>(null);
export function AgentProvider({ value, children }: { value: AgentAccess; children: ReactNode }) {
  return <AgentContext value={value}>{children}</AgentContext>;
}
export function useAgent(): AgentAccess {
  const value = useContext(AgentContext);
  if (!value) throw new Error("AgentProvider is missing");
  return value;
}
