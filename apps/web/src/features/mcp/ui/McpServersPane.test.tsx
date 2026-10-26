import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { contributeForTest } from "@/test/kernel";
import { queryClient } from "@/lib/queryClient";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import { MCP_SERVERS_KEY } from "@/features/mcp/application/mcpServerQueries";
import { type MCPServerGateway } from "@/features/mcp/application/ports/mcpServerGateway";
import { McpServersPane } from "@/features/mcp/ui/McpServersPane";

let owner: MCPServerMutationOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
  queryClient.clear();
});

async function mount(failure?: Error) {
  await contributeForTest((ctx) => {
    ctx.contribute(DATA_PROVIDER, {
      key: MCP_SERVERS_KEY,
      fetcher: async () => {
        if (failure) throw failure;
        return [];
      },
    });
  });
  owner = MCPServerMutationOwner.install({} as MCPServerGateway);
  testClient.render(
    <QueryClientProvider client={queryClient}>
      <McpServersPane />
    </QueryClientProvider>,
  );
}

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
  await mount(new Error("Private MCP list unavailable"));
  await screen.findByText("Couldn't load", {}, { timeout: 3000 });
  fireEvent.click(screen.getByRole("button", { name: "Add server" }));
  expect(screen.getByRole("textbox", { name: "Server name" })).toBeTruthy();
  expect(screen.getByText("Couldn't load")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
});
