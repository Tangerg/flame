import type { MCPTransport } from "./mcpServerQueries";
import type { MCPHandshakeTimeout } from "./mcpHandshakeTimeout";

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
  disabledTools?: string[];
  autoApproveTools?: string[];
}
