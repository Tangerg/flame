import type {
  MCPServer,
  MCPServerID,
  MCPStatusProblemType,
  MCPTransport,
  UpdateMCPServerRequest,
  MCPServerCandidate,
} from "@flame/runtime-client/wire";
import type { MCPHandshakeTimeout } from "./timeout";
import type { MCPServerInput } from "./input";
export type { MCPServerID, MCPTransport };
export interface MCPServerSettings {
  id: MCPServerID;
  status: MCPServer["status"]["type"];
  errorDetail?: MCPStatusProblemType;
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
export function serverKey(server: MCPServerID): string {
  return JSON.stringify(
    server.origin.type === "user"
      ? ["user", server.name]
      : ["installation", server.origin.installationId, server.name],
  );
}
export function userMCPServer(name: string): MCPServerID {
  return { origin: { type: "user" }, name };
}
export function sameMCPServer(left: MCPServerID, right: MCPServerID): boolean {
  return serverKey(left) === serverKey(right);
}
export function serverSettings(server: MCPServer): MCPServerSettings {
  const { connection, status } = server;
  return {
    id: server.id,
    status: status.type,
    errorDetail:
      status.type === "failed" || status.type === "needsAuth"
        ? status.error.type
        : undefined,
    type: connection.type,
    enabled: status.type !== "disabled",
    description: server.description,
    ...(connection.type === "stdio"
      ? {
          command: connection.command,
          args: connection.args,
          envMasked: connection.envMasked,
          dir: connection.dir,
        }
      : {
          url: connection.url,
          authorizationMasked: connection.authorizationMasked,
          headersMasked: connection.headersMasked,
        }),
    handshakeTimeout: server.handshakeTimeout,
    toolCount: status.type === "connected" ? status.toolCount : undefined,
  };
}
function change<T>(
  value: T | null | undefined,
): { type: "clear" } | { type: "set"; value: T } | undefined {
  return value === undefined
    ? undefined
    : value === null
      ? { type: "clear" }
      : { type: "set", value };
}
export function candidate(input: MCPServerInput): MCPServerCandidate {
  return {
    name: input.name,
    enabled: input.enabled,
    description: input.description,
    handshakeTimeout: input.handshakeTimeout,
    connection:
      input.transport === "stdio"
        ? {
            type: "stdio",
            command: input.command ?? "",
            args: input.args,
            dir: input.dir,
            env: change(input.env),
          }
        : {
            type: "streamableHttp",
            url: input.url ?? "",
            authorization: change(input.authorization),
            headers: change(input.headers),
          },
  };
}
export function updateRequest(
  server: MCPServerID,
  input: MCPServerInput,
): UpdateMCPServerRequest {
  const value = candidate(input);
  return {
    server,
    description: value.description ?? "",
    connection: value.connection,
    handshakeTimeout: value.handshakeTimeout,
  };
}
