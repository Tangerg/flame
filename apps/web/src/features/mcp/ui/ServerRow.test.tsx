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
import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { contributeForTest } from "@/test/kernel";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import {
  MCP_EXPOSURE_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  userMCPServer,
  type McpToolsQuery,
  type MCPServerID,
  type MCPServerSettings,
  type MCPToolSummary,
} from "@/features/mcp/application/mcpServerQueries";
import { type MCPServerGateway } from "@/features/mcp/application/ports/mcpServerGateway";
import { ServerRow } from "@/features/mcp/ui/ServerRow";
let owner: MCPServerMutationOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
  queryClient.clear();
});

const userDocs = userMCPServer("docs");
const installedDocs: MCPServerID = {
  origin: { type: "installation", installationId: "eeb329cd-c7ce-40c9-bd90-6821fef06d30" },
  name: "docs",
};

const tool = (name: string, description: string): MCPToolSummary => ({
  name,
  description,
  modelName: `docs_${name}`,
  nameConflicts: [],
});

function server(overrides: Partial<MCPServerSettings> = {}): MCPServerSettings {
  return {
    id: userDocs,
    status: "failed",
    errorDetail: "Couldn't reach this server — check the command or URL and retry.",
    type: "stdio",
    enabled: true,
    command: "docs-server",
    handshakeTimeout: { type: "unbounded" },
    ...overrides,
  };
}

function gatewayDouble(overrides: Record<string, unknown> = {}) {
  return {
    setEnabled: vi.fn(),
    setToolExposure: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    createAuthorizationAttempt: vi.fn(),
    getAuthorizationAttempt: vi.fn(),
    reconnect: vi.fn(),
    ...overrides,
  };
}

function Row({ server }: { server: MCPServerSettings }) {
  return (
    <QueryClientProvider client={queryClient}>
      <ServerRow server={server} />
    </QueryClientProvider>
  );
}

async function mount(initial: MCPServerSettings, gateway = gatewayDouble()) {
  const toolsRead = vi.fn(async (params?: unknown) => {
    const query = params as McpToolsQuery | undefined;
    return query?.server.origin.type === "installation"
      ? [tool("render", "Renders plugin documentation.")]
      : [tool("read-page", "Reads documentation pages.")];
  });
  await contributeForTest((ctx) => {
    ctx.contribute(DATA_PROVIDER, { key: MCP_TOOLS_KEY, fetcher: toolsRead });
    for (const key of [MCP_EXPOSURE_KEY, "approval-rules"]) {
      ctx.contribute(DATA_PROVIDER, { key, fetcher: async () => [] });
    }
  });
  queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], [initial]);
  owner = MCPServerMutationOwner.install(gateway as unknown as MCPServerGateway);
  const view = testClient.render(<Row server={initial} />);
  const canonical = (next: MCPServerSettings) => {
    queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], [next]);
    view.rerender(<Row server={next} />);
  };
  return { gateway, toolsRead, canonical };
}

const toolsDisclosure = () => screen.getByRole("button", { name: "Tools for docs" });
const reconnectButton = () => screen.getByRole("button", { name: "Reconnect docs" });

function expectNoWrites(gateway: ReturnType<typeof gatewayDouble>) {
  for (const command of [
    gateway.setEnabled,
    gateway.setToolExposure,
    gateway.update,
    gateway.delete,
    gateway.reconnect,
  ]) {
    expect(command).not.toHaveBeenCalled();
  }
}

function expectNoEditor() {
  expect(screen.queryByRole("textbox", { name: "Command" })).toBeNull();
  expect(screen.queryByRole("textbox", { name: "Server name" })).toBeNull();
}

describe("MCP server row tools", () => {
  it.each([
    { origin: "user", id: userDocs, name: "read-page", description: "Reads documentation pages." },
    {
      origin: "installation",
      id: installedDocs,
      name: "render",
      description: "Renders plugin documentation.",
    },
  ])(
    "lists the $origin server's own tools directly from the row",
    async ({ id, name, description }) => {
      const { gateway, toolsRead } = await mount(
        server({ id, status: "connected", errorDetail: undefined, toolCount: 1 }),
      );
      const disclosure = toolsDisclosure();
      expect(disclosure.getAttribute("aria-expanded")).toBe("false");

      fireEvent.click(disclosure);

      expect(await screen.findByText(name)).toBeTruthy();
      expect(screen.getByText(description)).toBeTruthy();
      expect(disclosure.getAttribute("aria-expanded")).toBe("true");
      expect(toolsRead).toHaveBeenCalledWith({ server: id }, expect.anything());
      for (const [query] of toolsRead.mock.calls) expect(query).toEqual({ server: id });
      expectNoEditor();
      expectNoWrites(gateway);
    },
  );

  it("keeps expanded tools with the row while configuration is edited", async () => {
    await mount(server({ status: "connected", errorDetail: undefined, toolCount: 1 }));
    fireEvent.click(toolsDisclosure());
    await screen.findByText("read-page");

    const edit = screen.getByRole("button", { name: "Edit docs" });
    expect(edit.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(edit);

    expect(await screen.findByRole("textbox", { name: "Command" })).toBeTruthy();
    expect(toolsDisclosure().getAttribute("aria-expanded")).toBe("true");
    expect(screen.getAllByText("read-page")).toHaveLength(1);
    expect(screen.queryByText("Manage tools")).toBeNull();
  });
});

describe("MCP server row connection task", () => {
  it("reconnects the exact server once and keeps the same focused control through canonical states", async () => {
    const acknowledged = Promise.withResolvers<void>();
    const reconnect = vi.fn((_id: MCPServerID) => acknowledged.promise);
    const { gateway, canonical } = await mount(server(), gatewayDouble({ reconnect }));
    fireEvent.click(toolsDisclosure());
    await screen.findByText("read-page");
    expect(screen.getByText("Connection failed")).toBeTruthy();

    const button = reconnectButton();
    expect(button.textContent).toBe("Reconnect");
    button.focus();
    fireEvent.click(button);
    await waitFor(() => expect(reconnect).toHaveBeenCalledOnce());
    expect(reconnect).toHaveBeenCalledWith(userDocs);
    expect(button.getAttribute("aria-disabled")).toBe("true");
    await act(async () => fireEvent.click(button));
    expect(reconnect).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(button);

    canonical(server({ status: "connecting", errorDetail: undefined }));
    expect(reconnectButton()).toBe(button);
    expect(screen.getByText("Connecting…")).toBeTruthy();
    await act(async () => acknowledged.resolve());
    await waitFor(() => expect(queryClient.isFetching()).toBe(0));
    expect(screen.queryByText("Connected")).toBeNull();
    expect(button.getAttribute("aria-disabled")).toBe("true");
    await act(async () => fireEvent.click(button));
    expect(reconnect).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(button);

    canonical(server());
    expect(reconnectButton()).toBe(button);
    expect(screen.getByText("Connection failed")).toBeTruthy();
    expect(button.getAttribute("aria-disabled")).toBeNull();
    expect(document.activeElement).toBe(button);

    const laterFocus = screen.getByRole("switch", { name: "Enable docs" });
    laterFocus.focus();
    canonical(server({ status: "connected", errorDetail: undefined, toolCount: 1 }));
    expect(reconnectButton()).toBe(button);
    expect(screen.getByText("Connected")).toBeTruthy();
    expect(document.activeElement).toBe(laterFocus);
    expect(toolsDisclosure().getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("read-page")).toBeTruthy();
    expect(reconnect).toHaveBeenCalledOnce();
    expect(gateway.setEnabled).not.toHaveBeenCalled();
    expect(gateway.update).not.toHaveBeenCalled();
  });

  it("offers no reconnection for a disabled server", async () => {
    const { gateway } = await mount(
      server({ status: "disabled", enabled: false, errorDetail: undefined }),
    );
    expect(screen.getByText("Disabled")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /reconnect/i })).toBeNull();
    expectNoWrites(gateway);
  });

  it("uses the existing sign-in action for a server that needs authorization", async () => {
    const createAuthorizationAttempt = vi.fn((_id: MCPServerID) =>
      Promise.resolve({ id: "mcpauth_1", status: "succeeded" as const }),
    );
    const { gateway } = await mount(
      server({ status: "needsAuth", errorDetail: undefined }),
      gatewayDouble({ createAuthorizationAttempt }),
    );
    expect(screen.getByText("Sign in required")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", {
        name: (name) => name.startsWith("Sign in") && name.includes("docs"),
      }),
    );

    await waitFor(() => expect(createAuthorizationAttempt).toHaveBeenCalledOnce());
    expect(createAuthorizationAttempt).toHaveBeenCalledWith(userDocs, expect.anything());
    expect(gateway.reconnect).not.toHaveBeenCalled();
  });
});
