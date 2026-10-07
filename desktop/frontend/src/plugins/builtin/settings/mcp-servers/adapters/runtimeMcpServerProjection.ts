import type { MCPServer } from "@flame/runtime-contract/client";
import type { MCPServerSettings } from "../application/mcpServerQueries";
import { mcpStatusText } from "./mcpStatusText";
import {
  boundedMCPHandshakeTimeout,
  UNBOUNDED_MCP_HANDSHAKE,
} from "../application/mcpHandshakeTimeout";

export function mcpServerSettings(server: MCPServer): MCPServerSettings {
  const connection = server.connection;
  const status = server.status;
  return {
    id: server.id,
    status: status.type,
    errorDetail: statusProblem(server),
    type: connection.type,
    enabled: status.type !== "disabled",
    description: server.description,
    url: connection.type === "streamableHttp" ? connection.url : undefined,
    authorizationMasked:
      connection.type === "streamableHttp" ? connection.authorizationMasked : undefined,
    headersMasked: connection.type === "streamableHttp" ? connection.headersMasked : undefined,
    command: connection.type === "stdio" ? connection.command : undefined,
    args: connection.type === "stdio" ? connection.args : undefined,
    envMasked: connection.type === "stdio" ? connection.envMasked : undefined,
    dir: connection.type === "stdio" ? connection.dir : undefined,
    handshakeTimeout:
      server.handshakeTimeout.type === "bounded"
        ? boundedMCPHandshakeTimeout(server.handshakeTimeout.seconds)
        : UNBOUNDED_MCP_HANDSHAKE,
    toolCount: status.type === "connected" ? status.toolCount : undefined,
  };
}

function statusProblem(server: MCPServer): string | undefined {
  const status = server.status;
  return status.type === "failed" || status.type === "needsAuth"
    ? mcpStatusText(status.error.type)
    : undefined;
}
