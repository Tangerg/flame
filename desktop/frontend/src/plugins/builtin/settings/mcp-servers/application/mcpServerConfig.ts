import { useCallback, useSyncExternalStore } from "react";
import { type MCPServerID, type MCPServerSettings } from "./mcpServerQueries";
import type { MCPServerInput } from "./mcpServerInput";
import type { MCPServerTestOutcome } from "./ports/mcpServerGateway";
import { MCPServerMutationOwner } from "./mcpServerMutationOwner";

export type { MCPServerInput } from "./mcpServerInput";
export type { MCPTransport } from "./mcpServerQueries";
export type { MCPServerTestOutcome } from "./ports/mcpServerGateway";
export type { MCPServerSettings };

export function useMCPServerMutationMaterialGeneration(): bigint {
  return useSyncExternalStore(
    MCPServerMutationOwner.subscribeMaterialGeneration,
    MCPServerMutationOwner.materialGeneration,
    MCPServerMutationOwner.materialGeneration,
  );
}

export function createMCPServer(input: MCPServerInput): Promise<MCPServerSettings> {
  return MCPServerMutationOwner.current().create(input);
}

function updateMCPServer(server: MCPServerID, input: MCPServerInput): Promise<MCPServerSettings> {
  return MCPServerMutationOwner.current().update(server, input);
}

export function setMCPServerEnabled(
  server: MCPServerID,
  enabled: boolean,
): Promise<MCPServerSettings> {
  return MCPServerMutationOwner.current().setEnabled(server, enabled);
}

export function deleteMCPServer(server: MCPServerID): Promise<void> {
  return MCPServerMutationOwner.current().delete(server);
}

export function reconnectMCPServer(server: MCPServerID): Promise<void> {
  return MCPServerMutationOwner.current().reconnect(server);
}

export function useCreateMCPServer(): (input: MCPServerInput) => Promise<void> {
  return useCallback((input) => createMCPServer(input).then(() => undefined), []);
}

export function useUpdateMCPServer(): (
  server: MCPServerID,
  input: MCPServerInput,
) => Promise<void> {
  return useCallback((server, input) => updateMCPServer(server, input).then(() => undefined), []);
}

export function useDeleteMCPServer(): (server: MCPServerID) => Promise<void> {
  return useCallback((server) => deleteMCPServer(server), []);
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

export function useTestMCPServer(): (input: MCPServerInput) => Promise<MCPServerTestOutcome> {
  return useCallback((input) => MCPServerMutationOwner.current().test(input), []);
}

export function setMCPToolExposure(
  server: MCPServerID,
  name: string,
  disabled: boolean,
): Promise<void> {
  return MCPServerMutationOwner.current().setToolExposure(server, name, disabled);
}
