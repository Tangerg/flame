import { createContext, useContext, useMemo, type ReactNode } from "react";
import { type AgentDependencies } from "./agentDependencies";
import { type AgentAccess } from "./agentAccess";

interface AgentContextValue {
  dependencies: AgentDependencies;
  access: AgentAccess;
}
const AgentContext = createContext<AgentContextValue | null>(null);

export function AgentScope({
  dependencies,
  access,
  children,
}: AgentContextValue & { children?: ReactNode }) {
  const value = useMemo(() => ({ dependencies, access }), [dependencies, access]);
  return <AgentContext value={value}>{children}</AgentContext>;
}
function useAgentContext(): AgentContextValue {
  const value = useContext(AgentContext);
  if (!value) throw new Error("Agent scope is missing");
  return value;
}
export function useAgentDependencies(): AgentDependencies {
  return useAgentContext().dependencies;
}
export function useAgentAccess(): AgentAccess {
  return useAgentContext().access;
}
