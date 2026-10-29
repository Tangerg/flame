import {
  type MCPServerInput,
  type MCPServerUpdateInput,
} from "@/features/mcp/application/mcpServerInput";
import {
  type MCPServerID,
  type MCPServerSettings,
} from "@/features/mcp/application/mcpServerQueries";
export interface MCPServerTestOutcome {
  ok: boolean;
  error?: string;
}

export type MCPAuthorizationAttempt =
  | { id: string; status: "pending" }
  | { id: string; status: "succeeded" }
  | { id: string; status: "failed"; error: string }
  | { id: string; status: "canceled" };

export interface MCPServerGateway {
  create(input: MCPServerInput): Promise<MCPServerSettings>;
  update(server: MCPServerID, input: MCPServerUpdateInput): Promise<MCPServerSettings>;
  delete(server: MCPServerID): Promise<void>;
  setEnabled(server: MCPServerID, enabled: boolean): Promise<MCPServerSettings>;
  setToolExposure(server: MCPServerID, name: string, disabled: boolean): Promise<void>;
  reconnect(server: MCPServerID): Promise<void>;
  createAuthorizationAttempt(
    server: MCPServerID,
    signal?: AbortSignal,
  ): Promise<MCPAuthorizationAttempt>;
  getAuthorizationAttempt(id: string, signal?: AbortSignal): Promise<MCPAuthorizationAttempt>;
  test(input: MCPServerInput): Promise<MCPServerTestOutcome>;
}
