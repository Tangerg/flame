import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";
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
  it("rejects a captured enablement command instead of calling a successor gateway", async () => {
    const first = vi.fn(async () => server());
    const successor = vi.fn(async () => server());
    owner = new MCPServerMutationOwner({ setEnabled: first } as unknown as MCPServerGateway);
    const commands = owner.capture()!;
    const captured = commands.setEnabled.bind(commands);
    owner.replaceRuntimeGeneration(
      () => ({ setEnabled: successor }) as unknown as MCPServerGateway,
    );
    await expect(captured(userMCPServer("cloud"), false)).rejects.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
    expect(first).not.toHaveBeenCalled();
    expect(successor).not.toHaveBeenCalled();
  });

  it("refuses commands captured before retirement even after installing a successor", async () => {
    const create = vi.fn();
    owner = new MCPServerMutationOwner({ create } as unknown as MCPServerGateway);
    const captured = owner.capture()!;
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

  it("publishes material changes within its own instance lifecycle", () => {
    owner = new MCPServerMutationOwner(null);
    const start = owner.materialGeneration();
    const gateway = {} as MCPServerGateway;
    owner.replaceRuntimeGeneration(() => gateway);
    expect(owner.materialGeneration()).toBe(start + 1n);
    owner.replaceRuntimeGeneration(() => gateway);
    expect(owner.materialGeneration()).toBe(start + 2n);
    owner.dispose();
    expect(owner.materialGeneration()).toBe(start + 3n);
    owner = undefined;
  });

  it("keeps another instance's commands and subscribers live when retired", async () => {
    owner = new MCPServerMutationOwner(null);
    const reconnect = vi.fn().mockResolvedValue(undefined);
    const independent = new MCPServerMutationOwner({ reconnect } as unknown as MCPServerGateway);
    onTestFinished(() => independent.dispose());
    const listener = vi.fn();
    const unsubscribe = independent.subscribeMaterialGeneration(listener);
    onTestFinished(unsubscribe);
    const commands = independent.capture()!;
    const generation = independent.materialGeneration();

    owner.replaceRuntimeGeneration(() => ({}) as MCPServerGateway);
    owner.dispose();

    expect(independent.materialGeneration()).toBe(generation);
    expect(listener).not.toHaveBeenCalled();
    await expect(commands.reconnect(userMCPServer("cloud"))).resolves.toBeUndefined();
    expect(reconnect).toHaveBeenCalledExactlyOnceWith(userMCPServer("cloud"));
    independent.retireRuntimeGeneration();
    expect(listener).toHaveBeenCalledOnce();
    await expect(commands.reconnect(userMCPServer("cloud"))).rejects.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });
  });

  it("retires one Runtime generation without draining commands through its successor", async () => {
    const retired = Promise.withResolvers<MCPServerSettings>();
    const setEnabled = vi
      .fn()
      .mockReturnValueOnce(retired.promise)
      .mockResolvedValueOnce(server({ status: "connected", toolCount: 2 }));
    const gateway = { setEnabled } as unknown as MCPServerGateway;
    owner = new MCPServerMutationOwner(gateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);

    const inFlight = owner.capture()!.setEnabled(userMCPServer("cloud"), false);
    const queued = owner.capture()!.setEnabled(userMCPServer("cloud"), true);
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

    await expect(owner.capture()!.setEnabled(userMCPServer("cloud"), true)).resolves.toMatchObject({
      status: "connected",
    });
    expect(setEnabled).toHaveBeenCalledTimes(2);
  });

  it("does not globally serialize unrelated MCP server resources", async () => {
    const first = Promise.withResolvers<MCPServerSettings>();
    const setEnabled = vi.fn((id: MCPServerID) =>
      id.name === "cloud" ? first.promise : Promise.resolve(server({ id })),
    );
    owner = new MCPServerMutationOwner({ setEnabled } as unknown as MCPServerGateway);

    const blocked = owner.capture()!.setEnabled(userMCPServer("cloud"), false);
    const independent = owner.capture()!.setEnabled(userMCPServer("local"), true);
    await vi.waitFor(() => expect(setEnabled).toHaveBeenCalledTimes(2));
    await expect(independent).resolves.toMatchObject({ id: userMCPServer("local") });
    first.resolve(server({ status: "disabled", enabled: false }));
    await expect(blocked).resolves.toMatchObject({ enabled: false });
  });

  it("serializes reconnection with other commands for the same server", async () => {
    const enabling = Promise.withResolvers<MCPServerSettings>();
    const setEnabled = vi.fn(() => enabling.promise);
    const reconnect = vi.fn((_id: MCPServerID) => Promise.resolve());
    owner = new MCPServerMutationOwner({
      setEnabled,
      reconnect,
    } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server({ status: "failed" })]);

    const enabled = owner.capture()!.setEnabled(userMCPServer("cloud"), true);
    const reconnected = owner.capture()!.reconnect(userMCPServer("cloud"));
    const independent = owner.capture()!.reconnect(userMCPServer("local"));
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
    owner = new MCPServerMutationOwner({
      reconnect: vi.fn().mockResolvedValue(undefined),
    } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [failed]);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    await expect(owner.capture()!.reconnect(userMCPServer("cloud"))).resolves.toBeUndefined();

    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([failed]);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: [MCP_SERVERS_KEY] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: [MCP_TOOLS_KEY] });
  });

  it("retires pending and queued reconnection with its Runtime generation", async () => {
    const retired = Promise.withResolvers<void>();
    const reconnect = vi.fn(() => retired.promise);
    const successorReconnect = vi.fn().mockResolvedValue(undefined);
    owner = new MCPServerMutationOwner({ reconnect } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server({ status: "failed" })]);

    const inFlight = rejected(owner.capture()!.reconnect(userMCPServer("cloud")));
    const queued = rejected(owner.capture()!.reconnect(userMCPServer("cloud")));
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
    owner = new MCPServerMutationOwner({
      setEnabled: vi.fn().mockResolvedValue(saved),
    } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(owner.capture()!.setEnabled(userMCPServer("cloud"), true)).resolves.toEqual(saved);
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([saved]);
  });

  it("retires authorization polling and clears its generation timer", async () => {
    vi.useFakeTimers();
    const getAuthorizationAttempt = vi.fn();
    const gateway = {
      createAuthorizationAttempt: vi.fn().mockResolvedValue({ id: "mcpauth_1", status: "pending" }),
      getAuthorizationAttempt,
    } as unknown as MCPServerGateway;
    owner = new MCPServerMutationOwner(gateway);

    const authorization = rejected(owner.capture()!.authorize(userMCPServer("github")));
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
