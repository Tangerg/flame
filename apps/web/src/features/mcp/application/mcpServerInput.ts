import { type MCPTransport } from "@/features/mcp/application/mcpServerQueries";
import { type MCPHandshakeTimeout } from "@/features/mcp/application/mcpHandshakeTimeout";
export interface MCPConnectionInput {
  transport: MCPTransport;
  command?: string;
  args?: string[];
  env?: Record<string, string> | null;
  dir?: string;
  url?: string;
  authorization?: string | null;
  headers?: Record<string, string> | null;
}

export interface MCPServerInput extends MCPConnectionInput {
  name: string;
  enabled: boolean;
  description?: string;
  handshakeTimeout: MCPHandshakeTimeout;
}

export interface MCPServerUpdateInput {
  connection?: MCPConnectionInput;
  description?: string;
  handshakeTimeout?: MCPHandshakeTimeout;
}
