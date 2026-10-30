import type { MCPHandshakeTimeout } from "@flame/runtime-client/wire";
export type { MCPHandshakeTimeout };

export const UNBOUNDED_MCP_HANDSHAKE: MCPHandshakeTimeout = Object.freeze({
  type: "unbounded",
});

export class MCPHandshakeTimeoutError extends Error {
  constructor() {
    super("invalid_mcp_handshake_timeout");
    this.name = "MCPHandshakeTimeoutError";
  }
}

export function boundedMCPHandshakeTimeout(
  seconds: number,
): MCPHandshakeTimeout {
  if (!Number.isSafeInteger(seconds) || seconds <= 0) {
    throw new MCPHandshakeTimeoutError();
  }
  return { type: "bounded", seconds };
}

export function mcpHandshakeTimeoutFromOptionalSeconds(
  seconds: number | undefined,
): MCPHandshakeTimeout {
  return seconds === undefined
    ? UNBOUNDED_MCP_HANDSHAKE
    : boundedMCPHandshakeTimeout(seconds);
}

export function mcpHandshakeTimeoutSeconds(
  timeout: MCPHandshakeTimeout,
): number | undefined {
  return timeout.type === "bounded" ? timeout.seconds : undefined;
}
