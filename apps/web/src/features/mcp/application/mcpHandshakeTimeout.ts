export type MCPHandshakeTimeout =
  { readonly type: "unbounded" } | { readonly type: "bounded"; readonly seconds: number };

export const UNBOUNDED_MCP_HANDSHAKE: MCPHandshakeTimeout = Object.freeze({
  type: "unbounded",
});

export function boundedMCPHandshakeTimeout(seconds: number): MCPHandshakeTimeout {
  if (!Number.isSafeInteger(seconds) || seconds <= 0) {
    throw new Error("MCP handshake timeout must be a positive integer");
  }
  return { type: "bounded", seconds };
}

export function mcpHandshakeTimeoutFromOptionalSeconds(
  seconds: number | undefined,
): MCPHandshakeTimeout {
  return seconds === undefined ? UNBOUNDED_MCP_HANDSHAKE : boundedMCPHandshakeTimeout(seconds);
}

export function mcpHandshakeTimeoutSeconds(timeout: MCPHandshakeTimeout): number | undefined {
  return timeout.type === "bounded" ? timeout.seconds : undefined;
}
