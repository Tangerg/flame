import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-contract/client";
import { queryClient } from "@/lib/queryClient";
import {
  authorizeMCPServer,
  createMCPServer,
  reconnectMCPServer,
  setMCPServerEnabled,
  setMCPToolExposure,
} from "../application/mcpServerConfig";
import {
  MCP_SERVERS_KEY,
  type MCPServerSettings,
  userMCPServer,
} from "../application/mcpServerQueries";
import { validateWire } from "@flame/runtime-contract/validate";
import { installMCPServerGateway } from "./runtimeMcpServerGateway";
import { MCPServerMutationOwner } from "../application/mcpServerMutationOwner";
import { rejected } from "@/test/rejected";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;

  queryClient.removeQueries({ queryKey: [MCP_SERVERS_KEY] });
  vi.useRealTimers();
});

describe("runtimeMcpServerGateway", () => {
  it.each([
    ["reachable", { ok: true }],
    [
      "authorizationRequired",
      { ok: false, error: "This server needs you to sign in before it can be used." },
    ],
    [
      "timedOut",
      {
        ok: false,
        error: "The server didn't respond in time — check the command or URL and retry.",
      },
    ],
    [
      "failed",
      {
        ok: false,
        error:
          "The server did not connect or did not offer a valid tool list. Check its settings and retry.",
      },
    ],
  ])("renders the closed %s test outcome locally", async (outcome, expected) => {
    const test = vi.fn().mockResolvedValue({ outcome });
    uninstall = installMCPServerGateway(
      () => ({ mcp: { test } }) as unknown as FlameClient,
    ).dispose;
    await expect(
      MCPServerMutationOwner.current().test({
        name: "probe",
        transport: "stdio",
        enabled: true,
        handshakeTimeout: { type: "unbounded" },
        command: "tool-server",
      }),
    ).resolves.toEqual(expected);
  });

  it("changes exposure through its own operation without updating the server", async () => {
    const setToolExposure = vi.fn().mockResolvedValue(undefined);
    const update = vi.fn();
    const client = { mcp: { setToolExposure, update } } as unknown as FlameClient;
    uninstall = installMCPServerGateway(() => client).dispose;

    await setMCPToolExposure(userMCPServer("docs"), "read", true);

    expect(setToolExposure).toHaveBeenCalledWith({
      server: userMCPServer("docs"),
      name: "read",
      disabled: true,
    });
    expect(update).not.toHaveBeenCalled();
  });

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
      id: { origin: { type: "user" }, name: input.name },
      connection:
        input.transport === "stdio"
          ? { type: "stdio", command: "tool-server", args: [] }
          : { type: "streamableHttp", url: "https://mcp.example/sse" },
      handshakeTimeout: { type: "unbounded" },
      status: { type: "connected", toolCount: 0 },
    });
    const runtimeClient = () => ({ mcp: { create } }) as unknown as FlameClient;
    uninstall = installMCPServerGateway(() => runtimeClient()).dispose;

    await createMCPServer(input);

    expect(validateWire("MCPServerCandidate", create.mock.calls[0]?.[0])).toEqual([]);
  });

  it("maps the complete server returned by create", async () => {
    const create = vi.fn().mockResolvedValue({
      id: { origin: { type: "user" }, name: "local-tools" },
      description: "Local tools",
      connection: { type: "stdio", command: "tool-server", args: ["--stdio"] },
      handshakeTimeout: { type: "bounded", seconds: 15 },
      status: { type: "connected", toolCount: 3 },
    });
    const runtimeClient = () => ({ mcp: { create } }) as unknown as FlameClient;
    uninstall = installMCPServerGateway(() => runtimeClient()).dispose;

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
      id: userMCPServer("local-tools"),
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
      id: { origin: { type: "user" }, name: "cloud" },
      connection: { type: "streamableHttp", url: "https://example.test/mcp" },
      handshakeTimeout: { type: "unbounded" },
      status: { type: "disabled" },
    });
    const runtimeClient = () => ({ mcp: { update } }) as unknown as FlameClient;
    uninstall = installMCPServerGateway(() => runtimeClient()).dispose;

    await expect(setMCPServerEnabled(userMCPServer("cloud"), false)).resolves.toMatchObject({
      id: userMCPServer("cloud"),
      status: "disabled",
      enabled: false,
      type: "streamableHttp",
    });
    expect(update).toHaveBeenCalledWith({ server: userMCPServer("cloud"), enabled: false });
  });

  it("retires in-flight and queued server commands before installing a successor", async () => {
    const retiredUpdate = Promise.withResolvers<ReturnType<typeof runtimeServer>>();
    const updateRetired = vi.fn(() => retiredUpdate.promise);
    const updateSuccessor = vi
      .fn()
      .mockResolvedValue(runtimeServer({ status: { type: "connected", toolCount: 2 } }));
    let runtimeClient = () => ({ mcp: { update: updateRetired } }) as unknown as FlameClient;
    const retiredInstallation = installMCPServerGateway(() => runtimeClient());
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);

    const inFlight = setMCPServerEnabled(userMCPServer("cloud"), false);
    const queued = setMCPServerEnabled(userMCPServer("cloud"), true);
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateRetired).toHaveBeenCalledOnce());

    runtimeClient = () => ({ mcp: { update: updateSuccessor } }) as unknown as FlameClient;
    const successorInstallation = installMCPServerGateway(() => runtimeClient());
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    queryClient.setQueryData([MCP_SERVERS_KEY], [server({ status: "connected", toolCount: 2 })]);

    retiredUpdate.resolve(runtimeServer({ status: { type: "disabled" } }));
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(updateSuccessor).not.toHaveBeenCalled();
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([
      server({ status: "connected", toolCount: 2 }),
    ]);
  });

  it("does not continue an authorization attempt through a successor Runtime", async () => {
    vi.useFakeTimers();
    const createRetired = vi.fn().mockResolvedValue({
      id: "mcpauth_retired",
      status: { type: "pending" },
    });
    let runtimeClient = () =>
      ({
        mcp: { authorizationAttempts: { create: createRetired } },
      }) as unknown as FlameClient;
    const retiredInstallation = installMCPServerGateway(() => runtimeClient());
    const authorization = rejected(authorizeMCPServer(userMCPServer("github")));
    await vi.waitFor(() => expect(createRetired).toHaveBeenCalledOnce());

    const getSuccessor = vi.fn().mockResolvedValue({
      id: "mcpauth_retired",
      status: { type: "succeeded" },
    });
    runtimeClient = () =>
      ({ mcp: { authorizationAttempts: { get: getSuccessor } } }) as unknown as FlameClient;
    const successorInstallation = installMCPServerGateway(() => runtimeClient());
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
    let runtimeClient = () => ({ mcp: { reconnect: reconnectRetired } }) as unknown as FlameClient;
    uninstall = installMCPServerGateway(() => runtimeClient()).dispose;

    const reconnectSuccessor = vi.fn().mockResolvedValue(undefined);
    runtimeClient = () => ({ mcp: { reconnect: reconnectSuccessor } }) as unknown as FlameClient;

    await reconnectMCPServer(userMCPServer("cloud"));

    expect(reconnectRetired).toHaveBeenCalledWith(userMCPServer("cloud"));
    expect(reconnectSuccessor).not.toHaveBeenCalled();
  });

  it("retires an admitted reconnect when a successor Host takes ownership", async () => {
    const retired = Promise.withResolvers<void>();
    const reconnectRetired = vi.fn(() => retired.promise);
    let runtimeClient = () => ({ mcp: { reconnect: reconnectRetired } }) as unknown as FlameClient;
    const retiredInstallation = installMCPServerGateway(() => runtimeClient());
    const reconnect = rejected(reconnectMCPServer(userMCPServer("cloud")));
    await vi.waitFor(() => expect(reconnectRetired).toHaveBeenCalledOnce());

    const reconnectSuccessor = vi.fn().mockResolvedValue(undefined);
    runtimeClient = () => ({ mcp: { reconnect: reconnectSuccessor } }) as unknown as FlameClient;
    const successorInstallation = installMCPServerGateway(() => runtimeClient());
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
    id: { origin: { type: "user" }, name: "cloud" },
    connection: { type: "streamableHttp" as const, url: "https://example.test/mcp" },
    handshakeTimeout: { type: "unbounded" as const },
    status: { type: "disconnected" as const },
    ...overrides,
  };
}

function server(overrides: Partial<MCPServerSettings> = {}): MCPServerSettings {
  return {
    id: userMCPServer("cloud"),
    status: "disconnected",
    type: "streamableHttp",
    enabled: true,
    handshakeTimeout: { type: "unbounded" },
    url: "https://example.test/mcp",
    ...overrides,
  };
}
