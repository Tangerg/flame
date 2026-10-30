import { type MCPTransport } from "./model";
import { type MCPHandshakeTimeout } from "./timeout";
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
