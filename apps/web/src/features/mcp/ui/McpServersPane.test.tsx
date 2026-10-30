import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { onlineManager, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { contributeForTest } from "@/test/kernel";
import { queryClient } from "@/lib/queryClient";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import {
  MCP_EXPOSURE_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  userMCPServer,
  type MCPServerSettings,
  type MCPToolSummary,
} from "@/features/mcp/application/mcpServerQueries";
import { APPROVAL_RULES_KEY } from "@/features/agent/public/approvalPolicy";
import { type MCPServerGateway } from "@/features/mcp/application/ports/mcpServerGateway";
import { McpServersPane } from "@/features/mcp/ui/McpServersPane";

let owner: MCPServerMutationOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
  queryClient.clear();
});

const docs: MCPServerSettings = {
  id: userMCPServer("docs"),
  status: "connected",
  type: "stdio",
  enabled: true,
  command: "docs-server",
  handshakeTimeout: { type: "unbounded" },
  toolCount: 1,
};
const read: MCPToolSummary = {
  name: "read",
  modelName: "docs_read",
  description: "Reads documentation pages.",
  nameConflicts: [],
};

async function mount({
  failure,
  servers = [],
  tools = [],
}: {
  failure?: Error;
  servers?: MCPServerSettings[];
  tools?: MCPToolSummary[];
} = {}) {
  const list = vi.fn(async () => {
    if (failure) throw failure;
    return servers;
  });
  const loadTools = vi.fn(async () => tools);
  await contributeForTest((ctx) => {
    ctx.contribute(DATA_PROVIDER, { key: MCP_SERVERS_KEY, fetcher: list });
    ctx.contribute(DATA_PROVIDER, { key: MCP_TOOLS_KEY, fetcher: loadTools });
    for (const key of [MCP_EXPOSURE_KEY, APPROVAL_RULES_KEY])
      ctx.contribute(DATA_PROVIDER, { key, fetcher: async () => [] });
  });
  owner = testClient.mcpServers;
  owner.replaceRuntimeGeneration(() => ({}) as MCPServerGateway);
  testClient.render(
    <QueryClientProvider client={queryClient}>
      <McpServersPane />
    </QueryClientProvider>,
  );
  return { list, loadTools };
}

function pauseQueries() {
  const wasOnline = onlineManager.isOnline();
  onlineManager.setOnline(false);
  finishTestClient(() => onlineManager.setOnline(wasOnline));
}

it.each([
  { name: "empty", servers: [] as MCPServerSettings[], visibleText: "No MCP servers" },
  { name: "nonempty", servers: [docs], visibleText: "docs" },
])("waits for the first paused $name server result", async ({ servers, visibleText }) => {
  pauseQueries();
  const { list } = await mount({ servers });
  await waitFor(() =>
    expect(queryClient.getQueryState([MCP_SERVERS_KEY])).toMatchObject({
      status: "pending",
      fetchStatus: "paused",
      data: undefined,
    }),
  );
  expect(list).not.toHaveBeenCalled();
  expect(screen.getByRole("status").textContent).toBe("Loading");
  expect(screen.queryByText("No MCP servers")).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: "Add server" }));
  const name = screen.getByRole("textbox", { name: "Server name" });
  fireEvent.change(name, { target: { value: "Keep this draft" } });
  await act(async () => onlineManager.setOnline(true));
  await waitFor(() => expect(queryClient.getQueryState([MCP_SERVERS_KEY])?.status).toBe("success"));
  expect(document.activeElement).toBe(name);
  expect(name).toHaveProperty("value", "Keep this draft");
  expect(screen.queryByText("Loading")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(screen.getByText(visibleText)).toBeTruthy();
});

it.each([
  { name: "empty", tools: [] as MCPToolSummary[], visibleText: "This server exposes no tools" },
  { name: "nonempty", tools: [read], visibleText: "Reads documentation pages." },
])(
  "waits for the expanded server's first paused $name tools result",
  async ({ tools, visibleText }) => {
    const { loadTools } = await mount({ servers: [docs], tools });
    const disclosure = await screen.findByRole("button", { name: "Tools for docs" });
    pauseQueries();
    act(() => disclosure.focus());
    fireEvent.click(disclosure);
    await waitFor(() =>
      expect(queryClient.getQueryState([MCP_TOOLS_KEY, { server: docs.id }])).toMatchObject({
        status: "pending",
        fetchStatus: "paused",
        data: undefined,
      }),
    );
    expect(loadTools).not.toHaveBeenCalled();
    expect(screen.getByRole("status").textContent).toBe("Loading");
    expect(screen.queryByText("This server exposes no tools")).toBeNull();
    expect(screen.getByText("docs")).toBeTruthy();
    expect(document.activeElement).toBe(disclosure);

    await act(async () => onlineManager.setOnline(true));
    expect(await screen.findByText(visibleText)).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
    expect(document.activeElement).toBe(disclosure);
  },
);

it.each([
  { name: "empty", servers: [] as MCPServerSettings[], visibleText: "No MCP servers" },
  { name: "nonempty", servers: [docs], visibleText: "docs" },
])("keeps accepted $name servers through a paused reread", async ({ servers, visibleText }) => {
  const { list } = await mount({ servers });
  await screen.findByText(visibleText);
  list.mockClear();
  pauseQueries();
  act(() => void queryClient.refetchQueries({ queryKey: [MCP_SERVERS_KEY] }));
  await waitFor(() =>
    expect(queryClient.getQueryState([MCP_SERVERS_KEY])).toMatchObject({
      status: "success",
      fetchStatus: "paused",
      data: servers,
    }),
  );
  expect(list).not.toHaveBeenCalled();
  expect(screen.getByText(visibleText)).toBeTruthy();
  expect(screen.queryByRole("status")).toBeNull();
});

it.each([
  { name: "empty", tools: [] as MCPToolSummary[], visibleText: "This server exposes no tools" },
  { name: "nonempty", tools: [read], visibleText: "Reads documentation pages." },
])("keeps accepted $name tools through a paused reread", async ({ tools, visibleText }) => {
  const { loadTools } = await mount({ servers: [docs], tools });
  const disclosure = await screen.findByRole("button", { name: "Tools for docs" });
  fireEvent.click(disclosure);
  await screen.findByText(visibleText);
  loadTools.mockClear();
  pauseQueries();
  act(() => void queryClient.refetchQueries({ queryKey: [MCP_TOOLS_KEY] }));
  await waitFor(() =>
    expect(queryClient.getQueryState([MCP_TOOLS_KEY, { server: docs.id }])).toMatchObject({
      status: "success",
      fetchStatus: "paused",
      data: tools,
    }),
  );
  expect(loadTools).not.toHaveBeenCalled();
  expect(screen.getByText(visibleText)).toBeTruthy();
  expect(screen.queryByRole("status")).toBeNull();
  expect(disclosure.getAttribute("aria-expanded")).toBe("true");
});

it("shows the current editor without an empty-state prompt and restores the empty list on Cancel", async () => {
  await mount();
  await screen.findByText("No MCP servers");
  expect(screen.queryByText(/Add a server or paste a config/)).toBeNull();
  const opener = screen.getByRole("button", { name: "Add server" });
  fireEvent.click(opener);
  const name = screen.getByRole("textbox", { name: "Server name" });
  expect(document.activeElement).toBe(name);
  expect(screen.queryByText("No MCP servers")).toBeNull();
  const cancel = screen.getByRole("button", { name: "Cancel" });
  act(() => cancel.focus());
  fireEvent.click(cancel);
  await waitFor(() => expect(document.activeElement).toBe(opener));
  expect(screen.getByText("No MCP servers")).toBeTruthy();
  expect(screen.queryByRole("textbox", { name: "Server name" })).toBeNull();
});

it("keeps read failures and Retry visible while an Add draft is open", async () => {
  await mount({ failure: new Error("Private MCP list unavailable") });
  await screen.findByText("Couldn't load", {}, { timeout: 3000 });
  fireEvent.click(screen.getByRole("button", { name: "Add server" }));
  expect(screen.getByRole("textbox", { name: "Server name" })).toBeTruthy();
  expect(screen.getByText("Couldn't load")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
});
