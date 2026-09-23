import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";
import type { MCPHandshakeTimeout } from "./mcpHandshakeTimeout";

export type MCPTransport = "stdio" | "streamableHttp";
type MCPServerStatus =
  "disabled" | "disconnected" | "connecting" | "connected" | "failed" | "needsAuth";

export interface MCPServerSettings {
  id: string;
  name: string;
  desc: string;
  tools: number;
  status: MCPServerStatus;
  errorDetail?: string;
  icon: string;
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
  disabledTools?: string[];
  autoApproveTools?: string[];
  toolCount?: number;
}

export interface MCPToolSummary {
  name: string;
  description: string;
}

export interface McpToolsQuery {
  server: string;
}

export const MCP_SERVERS_KEY = "mcp-servers";
export const MCP_TOOLS_KEY = "mcp-tools";

const MCP_ICON = new Map([
  ["filesystem", "folder"],
  ["git", "branch"],
  ["github", "git"],
  ["linear", "list"],
  ["shell", "terminal"],
  ["slack", "chat"],
  ["web-search", "globe"],
  ["websearch", "globe"],
]);

export function mcpServerIcon(name: string): string {
  return MCP_ICON.get(name.toLowerCase()) ?? "tool";
}

export const useMCPServers = createDataQuery<MCPServerSettings[]>(MCP_SERVERS_KEY);
export const useMCPTools = createParameterizedDataQuery<McpToolsQuery, MCPToolSummary[]>(
  MCP_TOOLS_KEY,
);
