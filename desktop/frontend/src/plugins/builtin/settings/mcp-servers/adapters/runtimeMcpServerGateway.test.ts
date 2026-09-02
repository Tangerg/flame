import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { FlameClient } from "@/rpc";
import { queryClient } from "@/lib/queryClient";
import {
  authorizeMCPServer,
  createMCPServer,
  reconnectMCPServer,
  setMCPServerEnabled,
} from "../application/mcpServerConfig";
import { MCP_SERVERS_KEY, type MCPServerSettings } from "../application/mcpServerQueries";
import { validateWire } from "@flame/runtime-contract/validate";
import { installMCPServerGateway } from "./runtimeMcpServerGateway";
import { rejected } from "@/test/rejected";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [MCP_SERVERS_KEY] });
  vi.useRealTimers();
});

describe("runtimeMcpServerGateway", () => {
  // The payload this builds is the one place the frontend decides which fields a transport
  // may carry, and the wire's answer is CONDITIONAL — a stdio connection must not carry a
  // url, an http one must not carry a command. The generated TS type cannot say that, so it
  // marks both optional and agrees with either. Only the validator disagrees, which is how a
  // request carrying a forbidden field reached a live Runtime once already.
  it.each([
    [
      "stdio",
      {
        name: "local-tools",
        transport: "stdio" as const,
        enabled: true,
        handshakeTimeout: { type: "unbounded" as const },
        command: "tool-server",
        args: ["--stdio"],
      },
    ],
    [
      "streamableHttp",
      {
        name: "cloud",
        transport: "streamableHttp" as const,
        enabled: true,
        handshakeTimeout: { type: "bounded" as const, seconds: 15 },
        url: "https://mcp.example/sse",
      },
    ],
  ])("sends a %s candidate the Runtime would accept", async (_transport, input) => {
    const create = vi.fn().mockResolvedValue({
      name: input.name,
      connection:
        input.transport === "stdio"
          ? { type: "stdio", command: "tool-server", args: [] }
          : { type: "streamableHttp", url: "https://mcp.example/sse" },
      handshakeTimeout: { type: "unbounded" },
      status: { type: "connected", toolCount: 0 },
    });
    setContainer({ client: () => ({ mcp: { create } }) as unknown as FlameClient });
    uninstall = installMCPServerGateway().dispose;

    await createMCPServer(input);

    expect(validateWire("MCPServerCandidate", create.mock.calls[0]?.[0])).toEqual([]);
  });

  it("maps the complete server returned by create", async () => {
    const create = vi.fn().mockResolvedValue({
      name: "local-tools",
      description: "Local tools",
      connection: { type: "stdio", command: "tool-server", args: ["--stdio"] },
      handshakeTimeout: { type: "bounded", seconds: 15 },
      status: { type: "connected", toolCount: 3 },
      disabledTools: ["delete"],
      autoApproveTools: ["read"],
    });
    setContainer({ client: () => ({ mcp: { create } }) as unknown as FlameClient });
    uninstall = installMCPServerGateway().dispose;

    await expect(
      createMCPServer({
        name: "local-tools",
        transport: "stdio",
        enabled: true,
        handshakeTimeout: { type: "unbounded" },
        command: "tool-server",
        args: ["--stdio"],
      }),
    ).resolves.toMatchObject({
      id: "local-tools",
      name: "local-tools",
      desc: "Local tools",
      tools: 3,
      status: "connected",
      type: "stdio",
      enabled: true,
      command: "tool-server",
      args: ["--stdio"],
      toolCount: 3,
    });
  });

  it("returns the stored server after an enablement change", async () => {
    const update = vi.fn().mockResolvedValue({
      name: "cloud",
      connection: { type: "streamableHttp", url: "https://example.test/mcp" },
      handshakeTimeout: { type: "unbounded" },
      status: { type: "disabled" },
    });
    setContainer({ client: () => ({ mcp: { update } }) as unknown as FlameClient });
    uninstall = installMCPServerGateway().dispose;

    await expect(setMCPServerEnabled("cloud", false)).resolves.toMatchObject({
      name: "cloud",
      status: "disabled",
      enabled: false,
      type: "streamableHttp",
    });
    expect(update).toHaveBeenCalledWith({ server: "cloud", enabled: false });
  });

  it("retires in-flight and queued server commands before installing a successor", async () => {
    const retiredUpdate = Promise.withResolvers<ReturnType<typeof runtimeServer>>();
    const updateRetired = vi.fn(() => retiredUpdate.promise);
    const updateSuccessor = vi
      .fn()
      .mockResolvedValue(runtimeServer({ status: { type: "connected", toolCount: 2 } }));
    setContainer({
      client: () => ({ mcp: { update: updateRetired } }) as unknown as FlameClient,
    });
    const retiredInstallation = installMCPServerGateway();
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);

    const inFlight = setMCPServerEnabled("cloud", false);
    const queued = setMCPServerEnabled("cloud", true);
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ mcp: { update: updateSuccessor } }) as unknown as FlameClient,
    });
    const successorInstallation = installMCPServerGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    queryClient.setQueryData([MCP_SERVERS_KEY], [server({ status: "connected", tools: 2 })]);

    retiredUpdate.resolve(runtimeServer({ status: { type: "disabled" } }));
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(updateSuccessor).not.toHaveBeenCalled();
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([
      server({ status: "connected", tools: 2 }),
    ]);
  });

  it("does not continue an authorization attempt through a successor Runtime", async () => {
    vi.useFakeTimers();
    const createRetired = vi.fn().mockResolvedValue({
      id: "mcpauth_retired",
      status: { type: "pending" },
    });
    setContainer({
      client: () =>
        ({
          mcp: { authorizationAttempts: { create: createRetired } },
        }) as unknown as FlameClient,
    });
    const retiredInstallation = installMCPServerGateway();
    const authorization = rejected(authorizeMCPServer("github"));
    await vi.waitFor(() => expect(createRetired).toHaveBeenCalledOnce());

    const getSuccessor = vi.fn().mockResolvedValue({
      id: "mcpauth_retired",
      status: { type: "succeeded" },
    });
    setContainer({
      client: () =>
        ({ mcp: { authorizationAttempts: { get: getSuccessor } } }) as unknown as FlameClient,
    });
    const successorInstallation = installMCPServerGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    await vi.advanceTimersByTimeAsync(500);

    await expect(authorization).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(getSuccessor).not.toHaveBeenCalled();
  });

  it("binds reconnect to the exact Runtime client captured by its installation", async () => {
    const reconnectRetired = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () => ({ mcp: { reconnect: reconnectRetired } }) as unknown as FlameClient,
    });
    uninstall = installMCPServerGateway().dispose;

    const reconnectSuccessor = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () => ({ mcp: { reconnect: reconnectSuccessor } }) as unknown as FlameClient,
    });

    await reconnectMCPServer("cloud");

    expect(reconnectRetired).toHaveBeenCalledWith("cloud");
    expect(reconnectSuccessor).not.toHaveBeenCalled();
  });

  it("retires an admitted reconnect when a successor Host takes ownership", async () => {
    const retired = Promise.withResolvers<void>();
    const reconnectRetired = vi.fn(() => retired.promise);
    setContainer({
      client: () => ({ mcp: { reconnect: reconnectRetired } }) as unknown as FlameClient,
    });
    const retiredInstallation = installMCPServerGateway();
    const reconnect = rejected(reconnectMCPServer("cloud"));
    await vi.waitFor(() => expect(reconnectRetired).toHaveBeenCalledOnce());

    const reconnectSuccessor = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () => ({ mcp: { reconnect: reconnectSuccessor } }) as unknown as FlameClient,
    });
    const successorInstallation = installMCPServerGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };

    retired.resolve();
    await expect(reconnect).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(reconnectSuccessor).not.toHaveBeenCalled();
  });
});

function runtimeServer(overrides: Record<string, unknown> = {}) {
  return {
    name: "cloud",
    connection: { type: "streamableHttp" as const, url: "https://example.test/mcp" },
    handshakeTimeout: { type: "unbounded" as const },
    status: { type: "disconnected" as const },
    ...overrides,
  };
}

function server(overrides: Partial<MCPServerSettings> = {}): MCPServerSettings {
  return {
    id: "cloud",
    name: "cloud",
    desc: "",
    tools: 0,
    status: "disconnected",
    icon: "tool",
    type: "streamableHttp",
    enabled: true,
    handshakeTimeout: { type: "unbounded" },
    url: "https://example.test/mcp",
    ...overrides,
  };
}
