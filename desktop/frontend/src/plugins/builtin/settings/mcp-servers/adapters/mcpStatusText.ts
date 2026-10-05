import type { MCPStatusProblemType } from "@flame/runtime-contract/wire";
import { t } from "@/lib/i18n";

const MCP_STATUS_COPY: Record<MCPStatusProblemType, string> = {
  mcp_authorization_required: "mcpStatus.mcp_authorization_required",
  mcp_authorization_failed: "mcpStatus.mcp_authorization_failed",
  mcp_dial_failed: "mcpStatus.mcp_dial_failed",
  mcp_tool_discovery_failed: "mcpStatus.mcp_tool_discovery_failed",
  mcp_configuration_failed: "mcpStatus.mcp_configuration_failed",
  mcp_release_unavailable: "mcpStatus.mcp_release_unavailable",
  mcp_backend_unavailable: "mcpStatus.mcp_backend_unavailable",
};

export const MCP_STATUS_TYPES = Object.keys(MCP_STATUS_COPY) as MCPStatusProblemType[];

export function mcpStatusText(type: MCPStatusProblemType): string {
  return t(MCP_STATUS_COPY[type]);
}
