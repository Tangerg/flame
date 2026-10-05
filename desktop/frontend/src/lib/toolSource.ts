import type { MCPServerID, ToolRef } from "@flame/runtime-contract/wire";

export function mcpServerLabel(server: MCPServerID): string {
  return server.origin.type === "installation"
    ? `${server.origin.installationId}/${server.name}`
    : server.name;
}

export function toolSourceLabel(ref: ToolRef): string {
  switch (ref.type) {
    case "builtIn":
      return `builtIn/${ref.name}`;
    case "mcp":
      return `mcp/${mcpServerLabel(ref.server)}/${ref.name}`;
    case "a2a":
      return `a2a/${ref.endpoint}`;
  }
}
