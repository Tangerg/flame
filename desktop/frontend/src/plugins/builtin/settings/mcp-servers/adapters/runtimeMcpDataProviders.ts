import { getContainer } from "@/main/container";
import { emptyListIfUngated } from "@/lib/rpcErrors";
import type { Contributor, DataProviderSpec } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
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

export function registerMCPDataProviders(ctx: Contributor): void {
  const client = () => getContainer().client();
  const contribute = (provider: DataProviderSpec): void => {
    ctx.contribute(DATA_PROVIDER, provider);
  };
  contribute({
    key: MCP_SERVERS_KEY,
    fetcher: async () =>
      (await pageData(client().mcp.list()).catch(emptyListIfUngated)).map(mcpServerSettings),
  });
  contribute({
    key: MCP_TOOLS_KEY,
    fetcher: async (params) =>
      (
        await pageData(client().mcp.listTools(requiredQuery(params).server)).catch(
          emptyListIfUngated,
        )
      ).map((tool) => ({ name: tool.name, description: tool.description ?? "" })),
  });
}
