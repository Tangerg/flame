import { useCallback, useSyncExternalStore } from "react";
import {
  type MCPServerID,
  type MCPServerSettings,
} from "@/features/mcp/application/mcpServerQueries";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import { type MCPServerCommands } from "@/features/mcp/application/mcpServerMutationOwner";
export type { MCPTransport } from "@/features/mcp/application/mcpServerQueries";

export type { MCPServerSettings };

export function useMCPServerMutationMaterialGeneration(): bigint {
  return useSyncExternalStore(
    MCPServerMutationOwner.subscribeMaterialGeneration,
    MCPServerMutationOwner.materialGeneration,
    MCPServerMutationOwner.materialGeneration,
  );
}

export function setMCPServerEnabled(
  server: MCPServerID,
  enabled: boolean,
): Promise<MCPServerSettings> {
  return MCPServerMutationOwner.current().setEnabled(server, enabled);
}

export function useMCPServerCommands(): MCPServerCommands | null {
  return useSyncExternalStore(
    MCPServerMutationOwner.subscribeMaterialGeneration,
    MCPServerMutationOwner.capture,
    MCPServerMutationOwner.capture,
  );
}

export function useSetMCPServerEnabled(): (server: MCPServerID, enabled: boolean) => Promise<void> {
  return useCallback(
    (server, enabled) => setMCPServerEnabled(server, enabled).then(() => undefined),
    [],
  );
}

export function useAuthorizeMCPServer(): (
  server: MCPServerID,
  signal?: AbortSignal,
) => Promise<void> {
  return useCallback((server, signal) => authorizeMCPServer(server, signal), []);
}

export async function authorizeMCPServer(server: MCPServerID, signal?: AbortSignal): Promise<void> {
  await MCPServerMutationOwner.current().authorize(server, signal);
}

export function setMCPToolExposure(
  server: MCPServerID,
  name: string,
  disabled: boolean,
): Promise<void> {
  return MCPServerMutationOwner.current().setToolExposure(server, name, disabled);
}
