import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";
import type { MCPHandshakeTimeout } from "./mcpHandshakeTimeout";

export type MCPTransport = "stdio" | "streamableHttp";
type MCPServerStatus =
  "disabled" | "disconnected" | "connecting" | "connected" | "failed" | "needsAuth";

// MCPServerSettings is the frontend's unified MCP resource. Workspace and
// settings views intentionally consume the same read model instead of joining
// separate configuration and live-status caches.
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

// Keyed by the WIRE name, which the protocol constrains to
// `^[a-z0-9][a-z0-9._-]{0,31}$` — lowercase, no spaces. Display-cased keys can
// never match a name the runtime is able to send, which is how every server was
// falling back to the generic glyph.
// A Map, not an object: that regex admits `constructor`, so a server named it would
// read an inherited member out of an object literal and return a function where an
// icon name belongs.
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
