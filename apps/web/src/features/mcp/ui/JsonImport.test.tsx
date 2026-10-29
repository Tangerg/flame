import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
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
    reconnect: unexpected,
    createAuthorizationAttempt: unexpected,
    getAuthorizationAttempt: unexpected,
    test: unexpected,
  };
  return { gateway, create };
}

async function openImport(gateway: MCPServerGateway) {
  queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], []);
  owner = MCPServerMutationOwner.install(gateway);
  testClient.render(<JsonImport />);
  const opener = screen.getByRole("button", { name: "Import from JSON" });
  act(() => opener.focus());
  fireEvent.click(opener);
  const source = await screen.findByRole<HTMLTextAreaElement>("textbox", {
    name: "JSON configuration",
  });
  await waitFor(() => expect(document.activeElement).toBe(source));
  return source;
}

const invalidNames = JSON.stringify(
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
  it.each([
    { text: "{", message: "Enter valid JSON." },
    { text: "{}", message: 'Use a valid MCP configuration with a "mcpServers" object.' },
    { text: '{"mcpServers":{}}', message: 'Add at least one server under "mcpServers".' },
    {
      text: '{"mcpServers":{"---":{"command":"npx"}}}',
      message: 'Server name "---" does not meet the Runtime naming rules; edit the key and retry.',
    },
    {
      text: '{"mcpServers":{"empty":{}}}',
      message: 'Provide exactly one command or URL for server "empty".',
    },
    {
      text: '{"mcpServers":{"slow":{"command":"npx","timeout":0}}}',
      message:
        'Set a positive whole-second timeout for server "slow", or omit it for no time limit.',
    },
  ])(
    "explains the static source problem in $text before any command",
    async ({ text, message }) => {
      const { gateway, create } = runtimeGateway();
      const source = await openImport(gateway);
      fireEvent.change(source, { target: { value: text } });
      fireEvent.click(screen.getByRole("button", { name: "Import" }));
      expect((await screen.findByRole("alert")).textContent).toBe(message);
      expect(source.getAttribute("aria-invalid")).toBe("true");
      expect(source.value).toBe(text);
      expect(create).not.toHaveBeenCalled();
    },
  );

  it("keeps a named source error literal instead of parsing markup or interpolation", async () => {
    const { gateway, create } = runtimeGateway();
    const source = await openImport(gateway);
    const text = JSON.stringify({ mcpServers: { "<b>{{server}}</b>": { type: "stdio" } } });
    fireEvent.change(source, { target: { value: text } });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(
      'Server name "<b>{{server}}</b>" does not meet the Runtime naming rules; edit the key and retry.',
    );
    expect(alert.querySelector("b")).toBeNull();
    expect(source.value).toBe(text);
    expect(create).not.toHaveBeenCalled();
  });

  it.each([
    { entry: { type: "stdio" }, field: "command" },
    { entry: { type: "streamableHttp" }, field: "URL" },
  ])(
    "rejects a later missing $field before creating the first server",
    async ({ entry, field }) => {
      const { gateway, create } = runtimeGateway();
      const source = await openImport(gateway);
      const text = JSON.stringify({
        mcpServers: { "safe-first": { command: "npx" }, "bad-second": entry },
      });
      fireEvent.change(source, { target: { value: text } });
      fireEvent.click(screen.getByRole("button", { name: "Import" }));
      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toContain(field);
      expect(alert.textContent).toContain("bad-second");
      expect(create).not.toHaveBeenCalled();
      expect(source.getAttribute("aria-invalid")).toBe("true");
      expect(source.value).toBe(text);
      const corrected = JSON.stringify({
        mcpServers: { "safe-first": { command: "npx" }, "bad-second": { command: "node" } },
      });
      fireEvent.change(source, { target: { value: corrected } });
      expect(screen.queryByRole("alert")).toBeNull();
      fireEvent.click(screen.getByRole("button", { name: "Import" }));
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(create.mock.calls.map(([input]) => input.name)).toEqual(["safe-first", "bad-second"]);
    },
  );

  it.each([
    {
      source: { type: "sse", url: "https://example.test" },
      message: 'Unsupported "type" for server "second". Use "stdio" or "streamableHttp".',
    },
    {
      source: {
        url: "https://example.test",
        authorization: "secret-one",
        headers: { AUTHORIZATION: "secret-two" },
      },
      message: 'Server "second" declares "authorization" more than once. Keep one declaration.',
    },
  ])(
    "preflights protocol and credential issues before any creation",
    async ({ source: invalid, message }) => {
      const { gateway, create } = runtimeGateway();
      const source = await openImport(gateway);
      const text = JSON.stringify({ mcpServers: { first: { command: "node" }, second: invalid } });
      fireEvent.change(source, { target: { value: text } });
      fireEvent.click(screen.getByRole("button", { name: "Import" }));
      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toBe(message);
      expect(alert.textContent).not.toContain("secret");
      expect(source.value).toBe(text);
      expect(create).not.toHaveBeenCalled();
    },
  );

  it("uses the labeled JSON example without a repeated introduction and keeps code input literal", async () => {
    const { gateway } = runtimeGateway();
    const source = await openImport(gateway);
    expect(screen.queryByText(/Paste an MCP client config block/)).toBeNull();
    expect(JSON.parse(source.placeholder)).toEqual({
      mcpServers: { "my-server": { type: "streamableHttp", url: "https://example.com/mcp" } },
    });
    expect(source.getAttribute("autocapitalize")).toBe("none");
    expect(source.getAttribute("autocorrect")).toBe("off");
    expect(source.getAttribute("spellcheck")).toBe("false");
  });

  it.each(["{", '{"mcpServers":{}}'])(
    "retires validation feedback after editing %s without replacing the draft",
    async (invalid) => {
      const { gateway, create } = runtimeGateway();
      const source = await openImport(gateway);
      fireEvent.change(source, { target: { value: invalid } });
      fireEvent.click(screen.getByRole("button", { name: "Import" }));
      await screen.findByRole("alert");
      expect(source.getAttribute("aria-invalid")).toBe("true");
      expect(source.getAttribute("aria-describedby")).toBeTruthy();
      expect(create).not.toHaveBeenCalled();

      const corrected = '{"mcpServers":{"native-json":{"command":"npx"}}}';
      act(() => source.focus());
      fireEvent.change(source, { target: { value: corrected } });
      expect(screen.queryByRole("alert")).toBeNull();
      expect(source.getAttribute("aria-invalid")).toBeNull();
      expect(source.getAttribute("aria-describedby")).toBeNull();
      expect(screen.getByRole("textbox", { name: "JSON configuration" })).toBe(source);
      expect(document.activeElement).toBe(source);
      expect(source.value).toBe(corrected);
      expect(create).not.toHaveBeenCalled();
      fireEvent.click(screen.getByRole("button", { name: "Import" }));
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(create).toHaveBeenCalledExactlyOnceWith(
        expect.objectContaining({ name: "native-json", command: "npx" }),
      );
    },
  );

  it("keeps partial command failure separate from JSON validity and preserves later focus", async () => {
    const { gateway, create } = runtimeGateway();
    const accepted = create.getMockImplementation()!;
    const blocked = Promise.withResolvers<MCPServerSettings>();
    create.mockImplementation((input) =>
      input.name === "second" ? blocked.promise : accepted(input),
    );
    const source = await openImport(gateway);
    const text = '{"mcpServers":{"first":{"command":"npx"},"second":{"command":"node"}}}';
    fireEvent.change(source, { target: { value: text } });
    const submit = screen.getByRole("button", { name: "Import" });
    act(() => submit.focus());
    fireEvent.click(submit);
    await waitFor(() => expect(create).toHaveBeenCalledTimes(2));
    expect(source.readOnly).toBe(true);
    const cancel = screen.getByRole("button", { name: "Cancel" });
    act(() => cancel.focus());
    await act(async () => blocked.reject(new Error("The connection was interrupted.")));
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Imported 1 of 2 servers. The connection was interrupted.",
    );
    expect(source.getAttribute("aria-invalid")).toBeNull();
    expect(source.getAttribute("aria-describedby")).toBeNull();
    expect(source.readOnly).toBe(false);
    expect(source.value).toBe(text);
    expect(document.activeElement).toBe(cancel);
    expect(
      queryClient
        .getQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY])
        ?.map((server) => server.id.name),
    ).toEqual(["first"]);
    act(() => source.focus());
    fireEvent.change(source, { target: { value: text.replace('"second"', '"remaining"') } });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(document.activeElement).toBe(source);
    expect(create).toHaveBeenCalledTimes(2);
    fireEvent.click(cancel);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Import from JSON" })),
    );
  });

  it("reports an unexpected import rejection without marking valid source invalid", async () => {
    const { gateway, create } = runtimeGateway();
    const source = await openImport(gateway);
    const commands = MCPServerMutationOwner.capture()!;
    vi.spyOn(commands, "importServers").mockRejectedValueOnce(
      new Error("The import request failed."),
    );
    const text = '{"mcpServers":{"native-json":{"command":"npx"}}}';
    fireEvent.change(source, { target: { value: text } });
    const submit = screen.getByRole("button", { name: "Import" });
    act(() => submit.focus());
    fireEvent.click(submit);
    expect((await screen.findByRole("alert")).textContent).toBe("The import request failed.");
    expect(source.getAttribute("aria-invalid")).toBeNull();
    expect(source.getAttribute("aria-describedby")).toBeNull();
    expect(source.value).toBe(text);
    expect(document.activeElement).toBe(submit);
    expect(create).not.toHaveBeenCalled();
  });

  it("clears an unsubmitted JSON draft when its captured Runtime retires", async () => {
    const original = runtimeGateway();
    const source = await openImport(original.gateway);
    fireEvent.change(source, { target: { value: invalidNames } });
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

  it("rejects invalid names before creating any server", async () => {
    const { gateway, create } = runtimeGateway();
    const source = await openImport(gateway);

    fireEvent.change(source, { target: { value: invalidNames } });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    const message =
      'Server name "Docs Tools" does not meet the Runtime naming rules; edit the key and retry.';
    expect((await screen.findByRole("alert")).textContent).toBe(message);
    expect(create).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(source.value).toBe(invalidNames);
    expect(source.readOnly).toBe(false);
    const describedBy = source.getAttribute("aria-describedby");
    expect(describedBy && document.getElementById(describedBy)?.textContent).toBe(message);
    expect(document.activeElement).toBe(source);
    expect(useNotificationStore.getState().log).toEqual([]);
  });

  it("renders invalid source names literally inside the catalog message", async () => {
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
      'Server name "<b>{{second}}</b>" does not meet the Runtime naming rules; edit the key and retry.',
    );
    expect(message.querySelector("b")).toBeNull();
    expect(create).not.toHaveBeenCalled();
  });

  it("imports every server in order once the source names are valid", async () => {
    const { gateway, create } = runtimeGateway();
    const source = await openImport(gateway);
    fireEvent.change(source, { target: { value: invalidNames } });
    fireEvent.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain('Server name "Docs Tools"'),
    );

    fireEvent.change(source, {
      target: {
        value: invalidNames
          .replace('"docs-tools"', '"docs-web"')
          .replace('"Docs Tools"', '"docs-tools"'),
      },
    });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(source.getAttribute("aria-invalid")).toBeNull();
    expect(source.getAttribute("aria-describedby")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(create.mock.calls.map(([input]) => input.name)).toEqual([
      "safe-first",
      "docs-tools",
      "docs-web",
    ]);
    expect(useNotificationStore.getState().log.map((entry) => entry.message)).toEqual([
      "Imported 3 servers",
    ]);
  });
});
