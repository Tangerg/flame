import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { useNotificationStore } from "@/features/notifications/public/notifications";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import { type MCPServerInput } from "@/features/mcp/application/mcpServerInput";
import {
  MCP_SERVERS_KEY,
  userMCPServer,
  type MCPServerSettings,
} from "@/features/mcp/application/mcpServerQueries";
import { type MCPServerGateway } from "@/features/mcp/application/ports/mcpServerGateway";
import { JsonImport } from "@/features/mcp/ui/JsonImport";
let owner: MCPServerMutationOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
  queryClient.clear();
});

function unexpected(): never {
  throw new Error("unexpected MCP gateway call");
}

function runtimeGateway() {
  const accepted = new Set<string>();
  const create = vi.fn(async (input: MCPServerInput): Promise<MCPServerSettings> => {
    if (accepted.has(input.name)) throw new Error(`mcp server "${input.name}" already exists`);
    accepted.add(input.name);
    return {
      id: userMCPServer(input.name),
      status: "disconnected",
      type: input.transport,
      enabled: input.enabled,
      command: input.command,
      url: input.url,
      handshakeTimeout: input.handshakeTimeout,
    };
  });
  const gateway: MCPServerGateway = {
    create,
    update: unexpected,
    delete: unexpected,
    setEnabled: unexpected,
    setToolExposure: unexpected,
    createAuthorizationAttempt: unexpected,
    getAuthorizationAttempt: unexpected,
    test: unexpected,
  };
  return { gateway, create };
}

async function openImport(gateway: MCPServerGateway) {
  queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], []);
  owner = MCPServerMutationOwner.install(gateway);
  render(<JsonImport />);
  fireEvent.click(screen.getByRole("button", { name: "Import from JSON" }));
  const source = await screen.findByRole<HTMLTextAreaElement>("textbox", {
    name: "JSON configuration",
  });
  await waitFor(() => expect(document.activeElement).toBe(source));
  return source;
}

const colliding = JSON.stringify(
  {
    mcpServers: {
      "safe-first": { command: "safe" },
      "Docs Tools": { command: "docs" },
      "docs-tools": { url: "https://docs.example/mcp" },
    },
  },
  null,
  2,
);

describe("JsonImport", () => {
  it("clears an unsubmitted JSON draft when its captured Runtime retires", async () => {
    const original = runtimeGateway();
    const source = await openImport(original.gateway);
    fireEvent.change(source, { target: { value: colliding } });
    const successor = runtimeGateway();
    await act(async () => owner!.replaceRuntimeGeneration(() => successor.gateway));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Import from JSON" }));
    expect(
      (await screen.findByRole<HTMLTextAreaElement>("textbox", { name: "JSON configuration" }))
        .value,
    ).toBe("");
    expect(original.create).not.toHaveBeenCalled();
    expect(successor.create).not.toHaveBeenCalled();
  });

  it("reports a completed prefix when Runtime replacement interrupts the same import", async () => {
    const original = runtimeGateway();
    const source = await openImport(original.gateway);
    const successor = runtimeGateway();
    const commands = MCPServerMutationOwner.capture()!;
    const create = commands.create.bind(commands);
    vi.spyOn(commands, "create").mockImplementation(async (input) => {
      const result = await create(input);
      if (input.name === "first") owner!.replaceRuntimeGeneration(() => successor.gateway);
      return result;
    });
    fireEvent.change(source, {
      target: {
        value: JSON.stringify({
          mcpServers: { first: { command: "first" }, second: { command: "second" } },
        }),
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() =>
      expect(useNotificationStore.getState().log.map((entry) => entry.message)).toEqual([
        "Imported 1 of 2 servers before the Runtime connection changed.",
      ]),
    );
    expect(original.create.mock.calls.map(([input]) => input.name)).toEqual(["first"]);
    expect(successor.create).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("rejects names that collide after normalization before creating any server", async () => {
    const { gateway, create } = runtimeGateway();
    const source = await openImport(gateway);

    fireEvent.change(source, { target: { value: colliding } });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    const message =
      'Servers "Docs Tools" and "docs-tools" both become "docs-tools"; give them distinct names';
    expect((await screen.findByRole("alert")).textContent).toBe(message);
    expect(create).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(source.value).toBe(colliding);
    expect(source.readOnly).toBe(false);
    const describedBy = source.getAttribute("aria-describedby");
    expect(describedBy && document.getElementById(describedBy)?.textContent).toBe(message);
    expect(document.activeElement).toBe(source);
    expect(useNotificationStore.getState().log).toEqual([]);
  });

  it("renders colliding source names literally inside the catalog message", async () => {
    const { gateway, create } = runtimeGateway();
    const source = await openImport(gateway);
    const markup = JSON.stringify({
      mcpServers: {
        "<b>{{second}}</b>": { command: "first" },
        "b second b": { command: "second" },
      },
    });

    fireEvent.change(source, { target: { value: markup } });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    const message = await screen.findByRole("alert");
    expect(message.textContent).toBe(
      'Servers "<b>{{second}}</b>" and "b second b" both become "b-second-b"; give them distinct names',
    );
    expect(message.querySelector("b")).toBeNull();
    expect(create).not.toHaveBeenCalled();
  });

  it("imports every server in order once the source names are made distinct", async () => {
    const { gateway, create } = runtimeGateway();
    const source = await openImport(gateway);
    fireEvent.change(source, { target: { value: colliding } });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain('both become "docs-tools"'),
    );

    fireEvent.change(source, {
      target: { value: colliding.replace('"docs-tools"', '"docs-web"') },
    });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(create.mock.calls.map(([input]) => input.name)).toEqual([
      "safe-first",
      "docs-tools",
      "docs-web",
    ]);
    expect(useNotificationStore.getState().log.map((entry) => entry.message)).toEqual([
      "Imported 3 servers",
      "Renamed to match what the runtime accepts: Docs Tools → docs-tools",
    ]);
  });
});
