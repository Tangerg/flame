import { t } from "@/lib/i18n";
import type {
  MCPServerCandidate,
  MCPHandshakeTimeout as WireMCPHandshakeTimeout,
  MCPAuthorizationChange,
  MCPAuthorizationAttempt,
  MCPConnectionInput,
  MCPEnvironmentChange,
  MCPHeadersChange,
  FlameClient,
  MCPServerID,
  UpdateMCPServerRequest,
} from "@flame/runtime-contract/client";
import type { MCPTestResult } from "@flame/runtime-contract/wire";
import type { MCPServerInput } from "../application/mcpServerInput";
import type { MCPHandshakeTimeout } from "../application/mcpHandshakeTimeout";
import { mcpServerSettings } from "./runtimeMcpServerProjection";
import { mcpStatusText } from "./mcpStatusText";
import {
  type MCPAuthorizationAttempt as AuthorizationAttempt,
  type MCPServerGateway,
} from "../application/ports/mcpServerGateway";
import { MCPServerMutationOwner } from "../application/mcpServerMutationOwner";

function authorizationChange(value: string | null | undefined): MCPAuthorizationChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function headersChange(
  value: Record<string, string> | null | undefined,
): MCPHeadersChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function environmentChange(
  value: Record<string, string> | null | undefined,
): MCPEnvironmentChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

function connectionInput(input: MCPServerInput): MCPConnectionInput {
  if (input.transport === "stdio") {
    return {
      type: "stdio",
      command: input.command ?? "",
      args: input.args,
      env: environmentChange(input.env),
      dir: input.dir,
    };
  }
  return {
    type: "streamableHttp",
    url: input.url ?? "",
    authorization: authorizationChange(input.authorization),
    headers: headersChange(input.headers),
  };
}

function candidate(input: MCPServerInput): MCPServerCandidate {
  return {
    name: input.name,
    enabled: input.enabled,
    description: input.description,
    connection: connectionInput(input),
    handshakeTimeout: wireHandshakeTimeout(input.handshakeTimeout),
  };
}

function updateRequest(server: MCPServerID, input: MCPServerInput): UpdateMCPServerRequest {
  return {
    server,
    description: input.description ?? "",
    connection: connectionInput(input),
    handshakeTimeout: wireHandshakeTimeout(input.handshakeTimeout),
  };
}

function wireHandshakeTimeout(timeout: MCPHandshakeTimeout): WireMCPHandshakeTimeout {
  return timeout.type === "bounded"
    ? { type: "bounded", seconds: timeout.seconds }
    : { type: "unbounded" };
}

function authorizationAttempt(attempt: MCPAuthorizationAttempt): AuthorizationAttempt {
  switch (attempt.status.type) {
    case "pending":
    case "succeeded":
    case "canceled":
      return { id: attempt.id, status: attempt.status.type };
    case "failed":
      return {
        id: attempt.id,
        status: "failed",
        error: mcpStatusText(attempt.status.error.type),
      };
  }
}

function testOutcome(result: MCPTestResult): { ok: boolean; error?: string } {
  switch (result.outcome) {
    case "reachable":
      return { ok: true };
    case "authorizationRequired":
      return { ok: false, error: mcpStatusText("mcp_authorization_required") };
    case "timedOut":
      return { ok: false, error: t("mcp.testOutcome.timedOut") };
    case "failed":
      return { ok: false, error: t("mcp.testOutcome.failed") };
  }
}

function runtimeMCPServerGateway(client: FlameClient): MCPServerGateway {
  return {
    async create(input) {
      const saved = await client.mcp.create(candidate(input));
      return mcpServerSettings(saved);
    },
    async update(server, input) {
      const saved = await client.mcp.update(updateRequest(server, input));
      return mcpServerSettings(saved);
    },
    async delete(server) {
      await client.mcp.delete(server);
    },
    async setEnabled(server, enabled) {
      const saved = await client.mcp.update({ server, enabled });
      return mcpServerSettings(saved);
    },
    async setToolExposure(server, name, disabled) {
      await client.mcp.setToolExposure({ server, name, disabled });
    },
    async reconnect(server) {
      await client.mcp.reconnect(server);
    },
    async createAuthorizationAttempt(server, signal) {
      const attempt = await client.mcp.authorizationAttempts.create(server, signal);
      return authorizationAttempt(attempt);
    },
    async getAuthorizationAttempt(id, signal) {
      const attempt = await client.mcp.authorizationAttempts.get(id, signal);
      return authorizationAttempt(attempt);
    },
    async test(input) {
      const result = await client.mcp.test(candidate(input));
      return testOutcome(result);
    },
  };
}

export function installMCPServerGateway(runtimeClient: () => FlameClient) {
  const gateway = runtimeMCPServerGateway(runtimeClient());
  const mutationOwner = MCPServerMutationOwner.install(gateway);
  return {
    replaceRuntimeGeneration: () =>
      mutationOwner.replaceRuntimeGeneration(() => runtimeMCPServerGateway(runtimeClient())),
    dispose() {
      mutationOwner.dispose();
    },
  };
}
