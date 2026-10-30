import { createContext, useContext, type ReactNode } from "react";
import { type MCPServerMutationAccess } from "@/features/mcp/application/mcpServerMutationOwner";

const MCPServersContext = createContext<MCPServerMutationAccess | null>(null);

export function MCPServersProvider({
  value,
  children,
}: {
  value: MCPServerMutationAccess;
  children: ReactNode;
}) {
  return <MCPServersContext value={value}>{children}</MCPServersContext>;
}

export function useMCPServerAccess(): MCPServerMutationAccess {
  const value = useContext(MCPServersContext);
  if (!value) throw new Error("MCPServersProvider is missing");
  return value;
}
