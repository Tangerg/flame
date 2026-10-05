import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { MCPServerMutationOwner } from "./mcpServerMutationOwner";
import type { MCPServerGateway } from "./ports/mcpServerGateway";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  type MCPServerSettings,
  userMCPServer,
  type MCPServerID,
} from "./mcpServerQueries";
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
      .mockResolvedValueOnce(server({ status: "connected", tools: 2 }));
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

  it("does not turn failed cache repair into an accepted server command failure", async () => {
    const saved = server({ status: "connected", tools: 2 });
    owner = MCPServerMutationOwner.install({
      setEnabled: vi.fn().mockResolvedValue(saved),
    } as unknown as MCPServerGateway);
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(owner.setEnabled(userMCPServer("cloud"), true)).resolves.toEqual(saved);
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([saved]);
  });

  it("coalesces overlapping reconnect admissions and orders settings behind the same server", async () => {
    const admitted = Promise.withResolvers<void>();
    const reconnect = vi.fn(() => admitted.promise);
    const setEnabled = vi.fn().mockResolvedValue(server({ status: "disabled", enabled: false }));
    owner = MCPServerMutationOwner.install({
      reconnect,
      setEnabled,
    } as unknown as MCPServerGateway);

    const first = owner.reconnect(userMCPServer("cloud"));
    const duplicate = owner.reconnect(userMCPServer("cloud"));
    const settings = owner.setEnabled(userMCPServer("cloud"), false);
    await vi.waitFor(() => expect(reconnect).toHaveBeenCalledOnce());
    expect(setEnabled).not.toHaveBeenCalled();

    admitted.resolve();
    await expect(Promise.all([first, duplicate])).resolves.toEqual([undefined, undefined]);
    await expect(settings).resolves.toMatchObject({ enabled: false });
    expect(reconnect).toHaveBeenCalledOnce();
    expect(setEnabled).toHaveBeenCalledOnce();
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
