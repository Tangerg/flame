import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { MCP_SERVERS_PANE } from "../kit/panes";
import { installMCPServerGateway } from "./adapters/runtimeMcpServerGateway";
import { registerMCPDataProviders } from "./adapters/runtimeMcpDataProviders";
import {
  RUNTIME_STREAM_PORTS,
  followRuntimeGeneration,
} from "@/plugins/builtin/runtime/public/ports";

const McpServersPane = lazy(() =>
  import("./ui/McpServersPane").then(({ McpServersPane }) => ({ default: McpServersPane })),
);

export default definePlugin({
  name: "flame.builtin.mcp-servers-pane",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installMCPServerGateway();
    const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, () =>
      gateway.replaceRuntimeGeneration(),
    );
    registerMCPDataProviders(ctx);
    registerSettingsPane(ctx, {
      id: MCP_SERVERS_PANE,
      label: "settings.pane.mcpServers",
      group: "integrations",
      icon: "tool",
      order: 56,
      component: McpServersPane,
    });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
