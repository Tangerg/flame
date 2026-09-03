import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MCPServerSettings } from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
import { notifyError } from "@/plugins/sdk";
import { McpRow } from "./McpRow";

const mcp = vi.hoisted(() => ({
  reconnect: vi.fn<(server: string) => Promise<void>>(),
}));

// The real error, not a stand-in: the row's own guard is `wasGenerationRetired`, so a plain
// Error with the right message would prove nothing about the branch under test.
const retired = new GenerationRetiredError("mcp_server_mutation_generation");

vi.mock("@/plugins/builtin/settings/mcp-servers/public/serverCatalog", async (importOriginal) => ({
  ...(await importOriginal<
    typeof import("@/plugins/builtin/settings/mcp-servers/public/serverCatalog")
  >()),
  reconnectMCPServer: mcp.reconnect,
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  notifyError: vi.fn(),
}));

afterEach(() => {
  cleanup();
  mcp.reconnect.mockReset();
  vi.mocked(notifyError).mockClear();
});

describe("McpRow", () => {
  it("admits only one reconnect while the exact server command is unsettled", async () => {
    const admitted = Promise.withResolvers<void>();
    mcp.reconnect.mockImplementation(() => admitted.promise);
    render(<McpRow server={server()} />);

    const button = screen.getByRole("button", { name: "Reconnect" });
    fireEvent.click(button);
    fireEvent.click(button);

    await vi.waitFor(() => expect(mcp.reconnect).toHaveBeenCalledOnce());
    expect((button as HTMLButtonElement).disabled).toBe(true);

    await act(async () => admitted.resolve());
    await vi.waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    expect(notifyError).not.toHaveBeenCalled();
  });

  it("treats Runtime generation retirement as neutral settlement", async () => {
    const admitted = Promise.withResolvers<void>();
    mcp.reconnect.mockImplementation(() => admitted.promise);
    render(<McpRow server={server()} />);

    const button = screen.getByRole("button", { name: "Reconnect" });
    fireEvent.click(button);
    await vi.waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(true));

    await act(async () => admitted.reject(retired));

    await vi.waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    expect(notifyError).not.toHaveBeenCalled();
  });
});

function server(overrides: Partial<MCPServerSettings> = {}): MCPServerSettings {
  return {
    id: "cloud",
    name: "Cloud",
    desc: "Cloud tools",
    tools: 2,
    status: "disconnected",
    icon: "tool",
    type: "streamableHttp",
    enabled: true,
    handshakeTimeout: { type: "unbounded" },
    ...overrides,
  };
}
