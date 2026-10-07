import { mcpServerLabel } from "@/lib/toolSource";
import type { MCPServerID, MCPServerStateType, MCPTransport } from "@flame/runtime-contract/wire";
import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";
import type { MCPHandshakeTimeout } from "./mcpHandshakeTimeout";

export type { MCPServerID };

export type { MCPTransport };

export interface MCPServerSettings {
  id: MCPServerID;
  status: MCPServerStateType;
  errorDetail?: string;
  type: MCPTransport;
  enabled: boolean;
  description?: string;
  url?: string;
  authorizationMasked?: string;
  headersMasked?: Record<string, string>;
  command?: string;
  args?: string[];
  envMasked?: Record<string, string>;
  dir?: string;
  handshakeTimeout: MCPHandshakeTimeout;
  toolCount?: number;
}

export interface MCPToolSummary {
  modelName: string;
  nameConflicts: string[];
  name: string;
  description: string;
}

export interface McpToolsQuery {
  server: MCPServerID;
}

export function userMCPServer(name: string): MCPServerID {
  return { origin: { type: "user" }, name };
}

export function sameMCPServer(left: MCPServerID, right: MCPServerID): boolean {
  return mcpServerLabel(left) === mcpServerLabel(right);
}

export const MCP_SERVERS_KEY = "mcp-servers";
export const MCP_TOOLS_KEY = "mcp-tools";

export const useMCPServers = createDataQuery<MCPServerSettings[]>(MCP_SERVERS_KEY);
export const useMCPTools = createParameterizedDataQuery<McpToolsQuery, MCPToolSummary[]>(
  MCP_TOOLS_KEY,
);

export const MCP_EXPOSURE_KEY = "mcp-tool-exposure";
export const useMCPToolExposure = createParameterizedDataQuery<McpToolsQuery, string[]>(
  MCP_EXPOSURE_KEY,
);
