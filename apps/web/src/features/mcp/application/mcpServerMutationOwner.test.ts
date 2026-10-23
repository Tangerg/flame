import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import { type MCPServerGateway } from "@/features/mcp/application/ports/mcpServerGateway";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  type MCPServerSettings,
  userMCPServer,
  type MCPServerID,
} from "@/features/mcp/application/mcpServerQueries";
import { rejected } from "@/test/rejected";
let owner: MCPServerMutationOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
  queryClient.removeQueries({ queryKey: [MCP_SERVERS_KEY] });
  queryClient.removeQueries({ queryKey: [MCP_TOOLS_KEY] });
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("MCPServerMutationOwner", () => {
  it("refuses commands captured before retirement even after installing a successor", async () => {
    const create = vi.fn();
    owner = MCPServerMutationOwner.install({ create } as unknown as MCPServerGateway);
    const captured = MCPServerMutationOwner.capture()!;
    const successor = vi.fn();
    owner.replaceRuntimeGeneration(() => ({ create: successor }) as unknown as MCPServerGateway);
    await expect(
      captured.importServers([
        {
          name: "old-draft",
          transport: "stdio",
          command: "old",
          enabled: true,
          handshakeTimeout: { type: "unbounded" },
        },
      ]),
    ).resolves.toEqual({ status: "retired", completed: 0, total: 1 });
    expect(create).not.toHaveBeenCalled();
    expect(successor).not.toHaveBeenCalled();
  });

  it("publishes one material generation for install, Runtime replacement, and final disposal", () => {
    const start = MCPServerMutationOwner.materialGeneration();
    const gateway = {} as MCPServerGateway;
    owner = MCPServerMutationOwner.install(gateway);
    expect(MCPServerMutationOwner.materialGeneration()).toBe(start + 1n);

    owner.replaceRuntimeGeneration(() => gateway);
    expect(MCPServerMutationOwner.materialGeneration()).toBe(start + 2n);

    owner.dispose();
    expect(MCPServerMutationOwner.materialGeneration()).toBe(start + 3n);
    owner = undefined;
  });

  it("retires one Runtime generation without draining commands through its successor", async () => {
    const retired = Promise.withResolvers<MCPServerSettings>();
    const setEnabled = vi
      .fn()
      .mockReturnValueOnce(retired.promise)
      .mockResolvedValueOnce(server({ status: "connected", toolCount: 2 }));
    const gateway = { setEnabled } as unknown as MCPServerGateway;
    owner = MCPServerMutationOwner.install(gateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);

    const inFlight = owner.setEnabled(userMCPServer("cloud"), false);
    const queued = owner.setEnabled(userMCPServer("cloud"), true);
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(setEnabled).toHaveBeenCalledOnce());

    owner.replaceRuntimeGeneration(() => gateway);
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(setEnabled).toHaveBeenCalledOnce();

    retired.resolve(server({ status: "disabled", enabled: false }));
    await Promise.resolve();
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([server()]);

    await expect(owner.setEnabled(userMCPServer("cloud"), true)).resolves.toMatchObject({
      status: "connected",
    });
    expect(setEnabled).toHaveBeenCalledTimes(2);
  });

  it("does not globally serialize unrelated MCP server resources", async () => {
    const first = Promise.withResolvers<MCPServerSettings>();
    const setEnabled = vi.fn((id: MCPServerID) =>
      id.name === "cloud" ? first.promise : Promise.resolve(server({ id })),
    );
    owner = MCPServerMutationOwner.install({ setEnabled } as unknown as MCPServerGateway);

    const blocked = owner.setEnabled(userMCPServer("cloud"), false);
    const independent = owner.setEnabled(userMCPServer("local"), true);
    await vi.waitFor(() => expect(setEnabled).toHaveBeenCalledTimes(2));
    await expect(independent).resolves.toMatchObject({ id: userMCPServer("local") });
    first.resolve(server({ status: "disabled", enabled: false }));
    await expect(blocked).resolves.toMatchObject({ enabled: false });
  });

  it("serializes reconnection with other commands for the same server", async () => {
    const enabling = Promise.withResolvers<MCPServerSettings>();
    const setEnabled = vi.fn(() => enabling.promise);
    const reconnect = vi.fn((_id: MCPServerID) => Promise.resolve());
    owner = MCPServerMutationOwner.install({
      setEnabled,
      reconnect,
    } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server({ status: "failed" })]);

    const enabled = owner.setEnabled(userMCPServer("cloud"), true);
    const reconnected = owner.reconnect(userMCPServer("cloud"));
    const independent = owner.reconnect(userMCPServer("local"));
    await vi.waitFor(() => expect(setEnabled).toHaveBeenCalledOnce());
    await expect(independent).resolves.toBeUndefined();
    expect(reconnect.mock.calls).toEqual([[userMCPServer("local")]]);

    enabling.resolve(server({ status: "connecting" }));
    await expect(enabled).resolves.toMatchObject({ status: "connecting" });
    await expect(reconnected).resolves.toBeUndefined();
    expect(reconnect.mock.calls).toEqual([[userMCPServer("local")], [userMCPServer("cloud")]]);
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([server({ status: "connecting" })]);
  });

  it("rereads canonical server facts after a reconnect acknowledgement without writing them", async () => {
    const failed = server({ status: "failed", errorDetail: "Couldn't reach this server." });
    owner = MCPServerMutationOwner.install({
      reconnect: vi.fn().mockResolvedValue(undefined),
    } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [failed]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    await expect(owner.reconnect(userMCPServer("cloud"))).resolves.toBeUndefined();

    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([failed]);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: [MCP_SERVERS_KEY] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: [MCP_TOOLS_KEY] });
  });

  it("retires pending and queued reconnection with its Runtime generation", async () => {
    const retired = Promise.withResolvers<void>();
    const reconnect = vi.fn(() => retired.promise);
    const successorReconnect = vi.fn().mockResolvedValue(undefined);
    owner = MCPServerMutationOwner.install({ reconnect } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server({ status: "failed" })]);

    const inFlight = rejected(owner.reconnect(userMCPServer("cloud")));
    const queued = rejected(owner.reconnect(userMCPServer("cloud")));
    await vi.waitFor(() => expect(reconnect).toHaveBeenCalledOnce());

    owner.replaceRuntimeGeneration(
      () => ({ reconnect: successorReconnect }) as unknown as MCPServerGateway,
    );
    await expect(inFlight).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    await expect(queued).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    const accepted = [server({ status: "connected", toolCount: 2 })];
    queryClient.setQueryData([MCP_SERVERS_KEY], accepted);
    const acceptedCache = queryClient.getQueryData([MCP_SERVERS_KEY]);
    expect(acceptedCache).toEqual(accepted);

    retired.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(reconnect).toHaveBeenCalledOnce();
    expect(successorReconnect).not.toHaveBeenCalled();
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toBe(acceptedCache);
  });

  it("does not turn failed cache repair into an accepted server command failure", async () => {
    const saved = server({ status: "connected", toolCount: 2 });
    owner = MCPServerMutationOwner.install({
      setEnabled: vi.fn().mockResolvedValue(saved),
    } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(owner.setEnabled(userMCPServer("cloud"), true)).resolves.toEqual(saved);
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([saved]);
  });

  it("retires authorization polling and clears its generation timer", async () => {
    vi.useFakeTimers();
    const getAuthorizationAttempt = vi.fn();
    const gateway = {
      createAuthorizationAttempt: vi.fn().mockResolvedValue({ id: "mcpauth_1", status: "pending" }),
      getAuthorizationAttempt,
    } as unknown as MCPServerGateway;
    owner = MCPServerMutationOwner.install(gateway);

    const authorization = rejected(owner.authorize(userMCPServer("github")));
    await vi.waitFor(() => expect(vi.getTimerCount()).toBe(1));
    owner.replaceRuntimeGeneration(() => gateway);

    await expect(authorization).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(vi.getTimerCount()).toBe(0);
    expect(getAuthorizationAttempt).not.toHaveBeenCalled();
  });
});

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
