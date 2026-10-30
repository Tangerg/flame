import { type MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import { type FlameClient } from "@flame/runtime-client";
import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { MCP_SERVERS_PANE } from "@/features/settings/public/paneIDs";
import { createRuntimeMCPServerGateway } from "@/features/mcp/adapters/runtimeMcpServerGateway";
import { registerMCPDataProviders } from "@/features/mcp/adapters/runtimeMcpDataProviders";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";
const McpServersPane = lazy(() =>
  import("@/features/mcp/ui/McpServersPane").then(({ McpServersPane }) => ({
    default: McpServersPane,
  })),
);

export function createMCPServersPlugin(
  runtimeClient: () => FlameClient,
  mutations: MCPServerMutationOwner,
) {
  return definePlugin({
    name: "flame.builtin.mcp-servers-pane",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      ctx.cleanup(() => mutations.retireRuntimeGeneration());
      const replace = () =>
        mutations.replaceRuntimeGeneration(() => createRuntimeMCPServerGateway(runtimeClient()));
      if (ctx.runtime.connectionGeneration() !== null) replace();
      ctx.cleanup(
        followRuntimeGeneration(ctx.runtime, (next) => {
          if (next === null) mutations.retireRuntimeGeneration();
          else replace();
        }),
      );
      registerMCPDataProviders(ctx, runtimeClient);
      registerSettingsPane(ctx, {
        id: MCP_SERVERS_PANE,
        label: "settings.pane.mcpServers",
        group: "integrations",
        icon: "server",
        order: 56,
        component: McpServersPane,
      });
    },
  });
}
