import { toolSourceLabel } from "@/lib/toolSource";
import type { FlameClient } from "@flame/runtime-contract/client";
import { emptyListIfUngated } from "@/lib/rpcErrors";
import type { Contributor, DataProviderSpec } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  MCP_EXPOSURE_KEY,
  type McpToolsQuery,
} from "../application/mcpServerQueries";
import { mcpServerSettings } from "./runtimeMcpServerProjection";

function pageData<T>(request: Promise<{ data: T[] }>): Promise<T[]> {
  return request.then((page) => page.data);
}

function requiredQuery(params: unknown): McpToolsQuery {
  if (params === undefined) throw new Error(`Data provider "${MCP_TOOLS_KEY}" requires parameters`);
  return params as McpToolsQuery;
}

export function registerMCPDataProviders(ctx: Contributor, runtimeClient: () => FlameClient): void {
  const contribute = (provider: DataProviderSpec): void => {
    ctx.contribute(DATA_PROVIDER, provider);
  };
  contribute({
    key: MCP_SERVERS_KEY,
    fetcher: async () =>
      (await pageData(runtimeClient().mcp.list()).catch(emptyListIfUngated)).map(mcpServerSettings),
  });
  contribute({
    key: MCP_TOOLS_KEY,
    fetcher: async (params) =>
      (
        await pageData(runtimeClient().mcp.listTools(requiredQuery(params).server)).catch(
          emptyListIfUngated,
        )
      ).map((tool) => ({
        name: tool.name,
        description: tool.description ?? "",
        modelName: tool.modelName,
        nameConflicts: tool.nameConflicts.map(toolSourceLabel),
      })),
  });
  contribute({
    key: MCP_EXPOSURE_KEY,
    fetcher: async (params, signal) =>
      (await runtimeClient().mcp.toolExposure(requiredQuery(params).server, signal)).disabledTools,
  });
}
