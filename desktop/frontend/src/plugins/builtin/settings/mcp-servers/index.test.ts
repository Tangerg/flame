import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import type { FlameClient } from "@flame/runtime-contract/client";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
} from "@/plugins/builtin/runtime/public/services";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { setMCPServerEnabled } from "./application/mcpServerConfig";
import {
  MCP_SERVERS_KEY,
  type MCPServerSettings,
  userMCPServer,
} from "./application/mcpServerQueries";
import { createMCPServersPlugin } from "./index";
import { rejected } from "@/test/rejected";

afterEach(async () => {
  await resetKernelForTest();

  queryClient.removeQueries({ queryKey: [MCP_SERVERS_KEY] });
});

describe("MCP servers plugin Runtime generation wiring", () => {
  it("retires an admitted command when the Runtime process generation changes", async () => {
    const retired = Promise.withResolvers<ReturnType<typeof runtimeServer>>();
    const update = vi.fn(() => retired.promise);
    let runtimeClient = () => ({ mcp: { update } }) as unknown as FlameClient;
    let generation = RuntimeConnectionGeneration.forProcess("runtime_1");
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.mcp-runtime-generation",
      provides: { stream: RUNTIME_STREAM },
      setup() {
        return {
          stream: {
            connectionGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            reportConnectionLoss: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(
      runtime,
      createMCPServersPlugin(() => runtimeClient()),
    );
    queryClient.setQueryData([MCP_SERVERS_KEY], [server()]);

    const command = rejected(setMCPServerEnabled(userMCPServer("cloud"), false));
    await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());

    const successorUpdate = vi
      .fn()
      .mockResolvedValue(runtimeServer({ status: { type: "disabled" } }));
    runtimeClient = () => ({ mcp: { update: successorUpdate } }) as unknown as FlameClient;
    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "mcp_server_mutation_generation_retired",
    });

    retired.resolve(runtimeServer({ status: { type: "disabled" } }));
    await Promise.resolve();
    expect(queryClient.getQueryData([MCP_SERVERS_KEY])).toEqual([server()]);
    await expect(setMCPServerEnabled(userMCPServer("cloud"), false)).resolves.toMatchObject({
      enabled: false,
    });
    expect(successorUpdate).toHaveBeenCalledExactlyOnceWith({
      server: userMCPServer("cloud"),
      enabled: false,
    });
    expect(update).toHaveBeenCalledOnce();
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
