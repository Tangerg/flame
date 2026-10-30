import { type MCPTransport } from "./model";
import { type MCPHandshakeTimeout } from "./timeout";
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
