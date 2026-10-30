import { useCallback, useSyncExternalStore } from "react";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { type MCPServerID } from "@/features/mcp/application/mcpServerQueries";
import { type MCPServerCommands } from "@/features/mcp/application/mcpServerMutationOwner";
import { useMCPServerAccess } from "@/features/mcp/public/servers";
export type { MCPTransport, MCPServerSettings } from "@/features/mcp/application/mcpServerQueries";

export function useMCPServerMutationMaterialGeneration(): bigint {
  const owner = useMCPServerAccess();
  return useSyncExternalStore(
    owner.subscribeMaterialGeneration,
    owner.materialGeneration,
    owner.materialGeneration,
  );
}

export function useMCPServerCommands(): MCPServerCommands | null {
  const owner = useMCPServerAccess();
  return useSyncExternalStore(owner.subscribeMaterialGeneration, owner.capture, owner.capture);
}

function requireCommands(commands: MCPServerCommands | null): MCPServerCommands {
  if (!commands) throw new GenerationRetiredError("mcp_server_mutation_generation");
  return commands;
}

export function useSetMCPServerEnabled(): (server: MCPServerID, enabled: boolean) => Promise<void> {
  const commands = useMCPServerCommands();
  return useCallback(
    (server, enabled) =>
      requireCommands(commands)
        .setEnabled(server, enabled)
        .then(() => undefined),
    [commands],
  );
}

export function useAuthorizeMCPServer(): (
  server: MCPServerID,
  signal?: AbortSignal,
) => Promise<void> {
  const commands = useMCPServerCommands();
  return useCallback(
    (server, signal) => requireCommands(commands).authorize(server, signal),
    [commands],
  );
}

export function useSetMCPToolExposure(): (
  server: MCPServerID,
  name: string,
  disabled: boolean,
) => Promise<void> {
  const commands = useMCPServerCommands();
  return useCallback(
    (server, name, disabled) => requireCommands(commands).setToolExposure(server, name, disabled),
    [commands],
  );
}

export function useReconnectMCPServer(): (server: MCPServerID) => Promise<void> {
  const commands = useMCPServerCommands();
  return useCallback((server) => requireCommands(commands).reconnect(server), [commands]);
}
