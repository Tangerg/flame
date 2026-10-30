import { afterEach, describe, expect, it, vi } from "vitest";
import { userMCPServer } from "@/features/mcp/application/mcpServerQueries";
import {
  type MCPAuthorizationAttempt,
  type MCPServerGateway,
} from "@/features/mcp/application/ports/mcpServerGateway";
import {
  MCPServerMutationOwner,
  type MCPServerCommands,
} from "@/features/mcp/application/mcpServerMutationOwner";
const inertGateway = {
  create: vi.fn(),
  update: vi.fn(),
  delete: vi.fn(),
  setEnabled: vi.fn(),
  setToolExposure: vi.fn().mockResolvedValue(undefined),
  reconnect: vi.fn().mockResolvedValue(undefined),
  test: vi.fn(),
} as const;

let disposeGateway: () => void = () => undefined;

let commands: MCPServerCommands;

function installGateway(gateway: MCPServerGateway): void {
  const owner = new MCPServerMutationOwner(gateway);
  commands = owner.capture()!;
  disposeGateway = () => owner.dispose();
}

afterEach(() => {
  disposeGateway();
  disposeGateway = () => undefined;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("MCP authorization attempts", () => {
  it("polls the created resource until it succeeds", async () => {
    vi.useFakeTimers();
    const getAuthorizationAttempt = vi
      .fn<(id: string) => Promise<MCPAuthorizationAttempt>>()
      .mockResolvedValueOnce({ id: "mcpauth_1", status: "pending" })
      .mockResolvedValueOnce({ id: "mcpauth_1", status: "succeeded" });
    installGateway({
      ...inertGateway,
      createAuthorizationAttempt: vi.fn().mockResolvedValue({ id: "mcpauth_1", status: "pending" }),
      getAuthorizationAttempt,
    } as MCPServerGateway);

    const authorization = commands.authorize(userMCPServer("github"));
    await vi.advanceTimersByTimeAsync(1_000);
    await authorization;

    expect(getAuthorizationAttempt).toHaveBeenNthCalledWith(
      1,
      "mcpauth_1",
      expect.any(AbortSignal),
    );
    expect(getAuthorizationAttempt).toHaveBeenNthCalledWith(
      2,
      "mcpauth_1",
      expect.any(AbortSignal),
    );
  });

  it("surfaces a failed terminal attempt without parsing transport errors", async () => {
    installGateway({
      ...inertGateway,
      createAuthorizationAttempt: vi.fn().mockResolvedValue({
        id: "mcpauth_2",
        status: "failed",
        error: "Sign-in didn't complete. Try again.",
      }),
      getAuthorizationAttempt: vi.fn(),
    } as MCPServerGateway);

    await expect(commands.authorize(userMCPServer("github"))).rejects.toThrow(
      "Sign-in didn't complete. Try again.",
    );
  });
});
