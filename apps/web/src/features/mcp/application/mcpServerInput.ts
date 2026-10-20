import { type MCPTransport } from "@/features/mcp/application/mcpServerQueries";
import { type MCPHandshakeTimeout } from "@/features/mcp/application/mcpHandshakeTimeout";
export interface MCPServerInput {
  name: string;
  transport: MCPTransport;
  enabled: boolean;
  description?: string;
  command?: string;
  args?: string[];
  env?: Record<string, string> | null;
  dir?: string;
  url?: string;
  authorization?: string | null;
  headers?: Record<string, string> | null;
  handshakeTimeout: MCPHandshakeTimeout;
}
