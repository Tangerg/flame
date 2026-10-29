import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { contributeForTest } from "@/test/kernel";
import { MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import {
  MCP_EXPOSURE_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  userMCPServer,
  type MCPServerSettings,
} from "@/features/mcp/application/mcpServerQueries";
import {
  type MCPServerGateway,
  type MCPServerTestOutcome,
} from "@/features/mcp/application/ports/mcpServerGateway";
import { ServerForm } from "@/features/mcp/ui/ServerForm";
let owner: MCPServerMutationOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
  queryClient.clear();
});

const httpServer: MCPServerSettings = {
  id: userMCPServer("remote-tools"),
  status: "connected",
  type: "streamableHttp",
  enabled: true,
  url: "https://tools.example/mcp",
  authorizationMasked: "Bearer ****",
  headersMasked: { "X-Team": "****" },
  handshakeTimeout: { type: "bounded", seconds: 30 },
};

const stdioServer: MCPServerSettings = {
  id: userMCPServer("workspace-tools"),
  status: "connected",
  type: "stdio",
  enabled: true,
  command: "npx",
  args: ["-y", "@modelcontextprotocol/server-filesystem"],
  envMasked: { API_KEY: "****" },
  dir: "/work",
  handshakeTimeout: { type: "unbounded" },
};

function Harness({ server }: { server?: MCPServerSettings }) {
  const [opener, setOpener] = useState<HTMLButtonElement | null>(null);
  return (
    <>
      <button disabled={opener !== null} onClick={(event) => setOpener(event.currentTarget)}>
        Configure MCP server
      </button>
      {opener && (
        <ServerForm
          server={server}
          opener={opener}
          onDone={() => setOpener(null)}
          onCancel={() => setOpener(null)}
        />
      )}
    </>
  );
}

async function mount(gateway: Partial<MCPServerGateway>, server?: MCPServerSettings) {
  if (server) {
    await contributeForTest((ctx) => {
      for (const key of [MCP_TOOLS_KEY, MCP_EXPOSURE_KEY, "approval-rules"]) {
        ctx.contribute(DATA_PROVIDER, { key, fetcher: async () => [] });
      }
    });
  }
  queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], server ? [server] : []);
  owner = MCPServerMutationOwner.install(gateway as MCPServerGateway);
  testClient.render(
    <QueryClientProvider client={queryClient}>
      <Harness server={server} />
    </QueryClientProvider>,
  );
  const opener = screen.getByRole("button", { name: "Configure MCP server" });
  fireEvent.click(opener);
  await waitFor(() => expect(screen.getAllByRole("status")).toHaveLength(1));
  return opener;
}

function openDraft(test: MCPServerGateway["test"]) {
  owner = MCPServerMutationOwner.install({ test } as MCPServerGateway);
  testClient.render(<Harness />);
  const opener = screen.getByRole("button", { name: "Configure MCP server" });
  fireEvent.click(opener);
  const status = screen.getByRole("status");
  fireEvent.change(screen.getByRole("textbox", { name: "Server name" }), {
    target: { value: "workspace-tools" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "Command" }), {
    target: { value: "npx" },
  });
  const trigger = screen.getByRole("button", { name: "Test" });
  trigger.focus();
  return { opener, status, trigger };
}

async function settle() {
  await waitFor(() => expect(queryClient.isFetching()).toBe(0));
  await act(() => new Promise((resolve) => setTimeout(resolve, 0)));
}

function servers() {
  return queryClient.getQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY]);
}

function field(name: string) {
  return screen.getByLabelText<HTMLInputElement | HTMLTextAreaElement>(name, {
    selector: "input, textarea",
  });
}

function expectHeldFields(names: readonly string[], held: boolean) {
  const fields = names.map((name) => {
    const element = field(name);
    return {
      name,
      readOnly: element.readOnly,
      disabled: element.disabled,
      ariaDisabled: element.getAttribute("aria-disabled"),
    };
  });
  expect(fields).toEqual(
    names.map((name) => ({
      name,
      readOnly: held,
      disabled: false,
      ariaDisabled: held ? "true" : null,
    })),
  );
}

function expectDisabledControls(names: readonly string[], role: "switch" | "radio", held: boolean) {
  const controls = names.map((name) => ({
    name,
    disabled: screen.getByRole(role, { name }).hasAttribute("data-disabled"),
  }));
  expect(controls).toEqual(names.map((name) => ({ name, disabled: held })));
}

describe("MCP literal connection editing", () => {
  it("sends only a description change while retaining exact process fields", async () => {
    const stored = {
      ...stdioServer,
      command: "/tools/mcp ",
      dir: "/work/project ",
      args: ["", " value ", "first\nsecond"],
    };
    const update = vi.fn<MCPServerGateway["update"]>().mockResolvedValue(stored);
    await mount({ update }, stored);
    expect(field("Command").value).toBe(stored.command);
    expect(field("Working directory").value).toBe(stored.dir);
    expect(JSON.parse(field("Arguments (JSON array)").value)).toEqual(stored.args);
    fireEvent.change(field("Description"), { target: { value: "Changed description" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(stored.id, { description: "Changed description" }),
    );
  });

  it.each([
    {
      fieldName: "Arguments (JSON array)",
      invalid: "one\ntwo",
      valid: '["", " value ", "first\\nsecond"]',
      input: { args: ["", " value ", "first\nsecond"] },
    },
    {
      fieldName: "Environment (JSON object)",
      invalid: '{"EMPTY":1}',
      valid: JSON.stringify({ EMPTY: "", SPACES: " \t " }),
      input: { env: { EMPTY: "", SPACES: " \t " } },
    },
    {
      fieldName: "Headers (JSON object)",
      invalid: "X-Key=value",
      valid: '{"X-Key":"value"}',
      input: { headers: { "X-Key": "value" } },
    },
  ])(
    "explains invalid $fieldName JSON and preserves corrected material",
    async ({ fieldName, invalid, valid, input }) => {
      const test = vi.fn<MCPServerGateway["test"]>().mockResolvedValue({ ok: true });
      await mount(
        { test },
        fieldName.startsWith("Headers")
          ? httpServer
          : { ...stdioServer, args: [], envMasked: undefined },
      );
      const control = field(fieldName);
      control.focus();
      fireEvent.change(control, { target: { value: invalid } });
      expect(control.getAttribute("aria-invalid")).toBe("true");
      expect(
        document.getElementById(control.getAttribute("aria-describedby")!)?.textContent,
      ).toContain("JSON");
      expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(true);
      expect(test).not.toHaveBeenCalled();
      fireEvent.change(control, { target: { value: valid } });
      expect(control.hasAttribute("aria-invalid")).toBe(false);
      expect(control.hasAttribute("aria-describedby")).toBe(false);
      expect(document.activeElement).toBe(control);
      fireEvent.click(screen.getByRole("button", { name: "Test" }));
      await waitFor(() => expect(test).toHaveBeenCalledWith(expect.objectContaining(input)));
    },
  );
});

describe("MCP server test feedback", () => {
  it.each(["0", "-1", "1.5", "9007199254740992"])(
    "explains an invalid timeout %s and clears the error when corrected",
    (value) => {
      const test = vi.fn<MCPServerGateway["test"]>();
      const { trigger } = openDraft(test);
      const timeout = screen.getByRole<HTMLInputElement>("spinbutton");
      timeout.focus();
      fireEvent.change(timeout, { target: { value } });

      expect(timeout.getAttribute("aria-invalid")).toBe("true");
      const errorId = timeout.getAttribute("aria-describedby");
      expect(errorId).toBeTruthy();
      expect(document.getElementById(errorId!)?.textContent).toContain(
        "Enter positive whole seconds, or leave blank for no time limit.",
      );
      expect(timeout.min).toBe("1");
      expect(trigger.hasAttribute("disabled")).toBe(true);
      expect(screen.getByRole("button", { name: "Add server" }).hasAttribute("disabled")).toBe(
        true,
      );
      expect(document.activeElement).toBe(timeout);
      expect(test).not.toHaveBeenCalled();

      fireEvent.change(timeout, { target: { value: "15" } });
      expect(timeout.hasAttribute("aria-invalid")).toBe(false);
      expect(timeout.hasAttribute("aria-describedby")).toBe(false);
      expect(screen.queryByText(/Enter positive whole seconds/)).toBeNull();
      expect(trigger.hasAttribute("disabled")).toBe(false);
      expect(document.activeElement).toBe(timeout);
      expect(field("Server name").value).toBe("workspace-tools");
      expect(field("Command").value).toBe("npx");
    },
  );

  it("keeps a blank timeout unbounded when the corrected draft is saved", async () => {
    const saved: MCPServerSettings = {
      ...stdioServer,
      args: [],
      envMasked: undefined,
      handshakeTimeout: { type: "unbounded" },
    };
    const create = vi.fn<MCPServerGateway["create"]>().mockResolvedValue(saved);
    const opener = await mount({ create });
    fireEvent.change(field("Server name"), { target: { value: "workspace-tools" } });
    fireEvent.change(field("Command"), { target: { value: "npx" } });
    const timeout = screen.getByRole<HTMLInputElement>("spinbutton");
    fireEvent.change(timeout, { target: { value: "0" } });
    fireEvent.change(timeout, { target: { value: "" } });
    expect(timeout.hasAttribute("aria-invalid")).toBe(false);
    const save = screen.getByRole("button", { name: "Add server" });
    save.focus();
    fireEvent.click(save);

    await waitFor(() => expect(create).toHaveBeenCalledOnce());
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "workspace-tools",
        command: "npx",
        enabled: true,
        handshakeTimeout: { type: "unbounded" },
      }),
    );
    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(servers()).toEqual([saved]);
  });

  it("marks active required fields and avoids repeating their labels in placeholders", async () => {
    await mount({});
    expect(field("Server name").required).toBe(true);
    expect(field("Command").required).toBe(true);
    expect(field("Server name").placeholder).toBe("");
    expect(field("Command").placeholder).toBe("npx");
    expect(field("Working directory").placeholder).toBe("");
    expect(field("Description").placeholder).toBe("");
    expect(screen.queryByText(/Saving starts a background connection/)).toBeNull();

    fireEvent.click(screen.getByRole("radio", { name: "Streamable HTTP" }));
    expect(field("Server URL").required).toBe(true);
    expect(screen.queryByRole("textbox", { name: "Command" })).toBeNull();
  });

  it("keeps configuration identifiers, commands and connection values literal", async () => {
    await mount({});
    const expectLiteral = (names: string[]) => {
      for (const name of names) {
        const input = field(name);
        expect({
          name,
          capitalize: input.getAttribute("autocapitalize"),
          correct: input.getAttribute("autocorrect"),
          spelling: input.getAttribute("spellcheck"),
        }).toEqual({ name, capitalize: "none", correct: "off", spelling: "false" });
      }
    };
    expectLiteral([
      "Server name",
      "Command",
      "Arguments (JSON array)",
      "Environment (JSON object)",
      "Working directory",
    ]);
    fireEvent.click(screen.getByRole("radio", { name: "Streamable HTTP" }));
    expectLiteral(["Server name", "Server URL", "Authorization token", "Headers (JSON object)"]);
  });

  it("closes an authored draft on Runtime replacement and never submits it to the successor", async () => {
    const create = vi.fn<MCPServerGateway["create"]>();
    const opener = await mount({ create });
    fireEvent.change(field("Server name"), { target: { value: "old-draft" } });
    fireEvent.change(field("Command"), { target: { value: "old-command" } });
    const oldSubmit = screen.getByRole("button", { name: "Add server" });
    const successorCreate = vi.fn<MCPServerGateway["create"]>();
    await act(async () =>
      owner!.replaceRuntimeGeneration(
        () => ({ create: successorCreate }) as unknown as MCPServerGateway,
      ),
    );
    fireEvent.click(oldSubmit);
    expect(create).not.toHaveBeenCalled();
    expect(successorCreate).not.toHaveBeenCalled();
    expect(screen.queryByRole("textbox", { name: "Server name" })).toBeNull();
    fireEvent.click(opener);
    expect(field("Server name").value).toBe("");
    expect(field("Command").value).toBe("");
  });

  it("updates its existing polite region for repeated successful tests without moving focus", async () => {
    const first = Promise.withResolvers<MCPServerTestOutcome>();
    const repeated = Promise.withResolvers<MCPServerTestOutcome>();
    const test = vi
      .fn<MCPServerGateway["test"]>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(repeated.promise);
    const { opener, status, trigger } = openDraft(test);
    expect(status.textContent).toBe("");
    fireEvent.click(trigger);
    await waitFor(() => expect(test).toHaveBeenCalledOnce());
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    await act(async () => first.resolve({ ok: true }));
    await waitFor(() => expect(status.textContent).toContain("Connection OK"));
    expect(screen.getByRole("status")).toBe(status);
    expect(document.activeElement).toBe(trigger);

    fireEvent.click(trigger);
    await waitFor(() => expect(test).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    await act(async () => repeated.resolve({ ok: true }));
    await waitFor(() => expect(status.textContent).toContain("Connection OK"));
    expect(screen.getByRole("status")).toBe(status);
    expect(document.activeElement).toBe(trigger);
    const cancel = screen.getByRole("button", { name: "Cancel" });
    cancel.focus();
    fireEvent.click(cancel);
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it.each([
    {
      name: "unsuccessful connection",
      result: { ok: false, error: "The server rejected the connection." },
      text: "The server rejected the connection.",
    },
    {
      name: "failed request",
      error: new Error("The Runtime connection was interrupted."),
      text: "The Runtime connection was interrupted.",
    },
  ])("announces a $name without moving focus", async ({ result, error, text }) => {
    const pending = Promise.withResolvers<MCPServerTestOutcome>();
    const test = vi.fn(() => pending.promise);
    const { opener, status, trigger } = openDraft(test);
    expect(status.textContent).toBe("");
    fireEvent.click(trigger);
    await waitFor(() => expect(test).toHaveBeenCalledOnce());
    expect(test).toHaveBeenCalledWith(
      expect.objectContaining({ name: "workspace-tools", transport: "stdio", command: "npx" }),
    );
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    expect(screen.queryByRole("alert")).toBeNull();

    await act(async () => {
      if (error) pending.reject(error);
      else pending.resolve(result!);
    });

    expect((await screen.findByRole("alert")).textContent).toContain(text);
    expect(document.activeElement).toBe(trigger);
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    const cancel = screen.getByRole("button", { name: "Cancel" });
    cancel.focus();
    fireEvent.click(cancel);
    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(screen.queryByRole("textbox", { name: "Server name" })).toBeNull();
  });

  it("retires a completed connection result when the draft changes", async () => {
    const test = vi.fn<MCPServerGateway["test"]>().mockResolvedValue({ ok: true });
    const { status, trigger } = openDraft(test);
    fireEvent.click(trigger);
    await waitFor(() => expect(status.textContent).toContain("Connection OK"));

    const command = field("Command");
    command.focus();
    fireEvent.change(command, { target: { value: "uvx" } });

    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    expect(document.activeElement).toBe(command);
  });

  it.each([
    {
      name: "success",
      settle: (p: PromiseWithResolvers<MCPServerTestOutcome>) => p.resolve({ ok: true }),
    },
    {
      name: "unsuccessful connection",
      settle: (p: PromiseWithResolvers<MCPServerTestOutcome>) =>
        p.resolve({ ok: false, error: "The server rejected the connection." }),
    },
    {
      name: "failed request",
      settle: (p: PromiseWithResolvers<MCPServerTestOutcome>) =>
        p.reject(new Error("The Runtime connection was interrupted.")),
    },
  ])("does not publish an in-flight $name onto a changed draft", async ({ settle: finish }) => {
    const pending = Promise.withResolvers<MCPServerTestOutcome>();
    const test = vi.fn(() => pending.promise);
    const { status, trigger } = openDraft(test);
    fireEvent.click(trigger);
    await waitFor(() => expect(test).toHaveBeenCalledOnce());

    fireEvent.change(field("Command"), { target: { value: "uvx" } });
    await act(async () => finish(pending));

    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.getByRole("button", { name: "Test" })).toBe(trigger);
  });
});

describe("MCP server form presentation lifetime", () => {
  it("lets a save admitted by a canceled form finish without closing the reopened draft", async () => {
    const created = Promise.withResolvers<MCPServerSettings>();
    const create = vi.fn<MCPServerGateway["create"]>(() => created.promise);
    const opener = await mount({ create });
    fireEvent.change(field("Server name"), { target: { value: "workspace-tools" } });
    fireEvent.change(field("Command"), { target: { value: "npx" } });
    fireEvent.click(screen.getByRole("button", { name: "Add server" }));
    await waitFor(() => expect(create).toHaveBeenCalledOnce());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("textbox", { name: "Server name" })).toBeNull();
    fireEvent.click(opener);
    const name = field("Server name");
    expect(document.activeElement).toBe(name);
    fireEvent.change(name, { target: { value: "another-draft" } });

    const saved: MCPServerSettings = { ...stdioServer, args: [], envMasked: undefined };
    await act(async () => created.resolve(saved));
    await waitFor(() => expect(servers()).toEqual([saved]));
    await settle();

    expect(field("Server name")).toBe(name);
    expect(name.value).toBe("another-draft");
    expect(document.activeElement).toBe(name);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("lets a delete admitted by a canceled form finish without closing the reopened draft", async () => {
    const deleted = Promise.withResolvers<void>();
    const remove = vi.fn<MCPServerGateway["delete"]>(() => deleted.promise);
    const opener = await mount({ delete: remove }, stdioServer);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(
      within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }),
    );
    await waitFor(() => expect(remove).toHaveBeenCalledWith(stdioServer.id));

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("textbox", { name: "Command" })).toBeNull();
    fireEvent.click(opener);
    const command = field("Command");
    expect(document.activeElement).toBe(command);
    fireEvent.change(command, { target: { value: "uvx" } });

    await act(async () => deleted.resolve());
    await waitFor(() => expect(servers()).toEqual([]));
    await settle();

    expect(field("Command")).toBe(command);
    expect(command.value).toBe("uvx");
    expect(document.activeElement).toBe(command);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("MCP server form while a mutation is pending", () => {
  it("holds HTTP configuration during save and releases the exact draft on failure", async () => {
    const updated = Promise.withResolvers<MCPServerSettings>();
    const update = vi.fn<MCPServerGateway["update"]>(() => updated.promise);
    const test = vi.fn<MCPServerGateway["test"]>().mockResolvedValue({ ok: true });
    await mount({ update, test }, httpServer);
    const textFields = [
      "Server URL",
      "Authorization token",
      "Headers (JSON object)",
      "Description",
    ];
    const switches = ["Remove stored authorization", "Remove stored headers"];
    const tabs = ["stdio", "Streamable HTTP"];
    fireEvent.change(field("Server URL"), { target: { value: "https://tools.example/v2" } });
    fireEvent.change(field("Description"), { target: { value: "Team tools" } });
    fireEvent.click(screen.getByText("Remove stored headers"));

    const save = screen.getByRole("button", { name: "Save" });
    save.focus();
    fireEvent.click(save);
    await waitFor(() => expect(update).toHaveBeenCalledOnce());
    expect(update).toHaveBeenCalledWith(
      httpServer.id,
      expect.objectContaining({
        description: "Team tools",
        connection: expect.objectContaining({ url: "https://tools.example/v2", headers: null }),
      }),
    );

    expectHeldFields(textFields, true);
    expect((screen.getByRole("spinbutton") as HTMLInputElement).readOnly).toBe(true);
    expectDisabledControls(switches, "switch", true);
    expectDisabledControls(tabs, "radio", true);
    const trigger = screen.getByRole<HTMLButtonElement>("button", { name: "Test" });
    expect(trigger.disabled).toBe(false);
    expect(trigger.getAttribute("aria-disabled")).toBe("true");
    trigger.focus();
    await act(async () => fireEvent.click(trigger));
    expect(test).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(trigger);
    save.focus();
    expect((screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled).toBe(
      false,
    );

    await act(async () => updated.reject(new Error("The Runtime rejected the update.")));

    expect((await screen.findByRole("alert")).textContent).toContain(
      "The Runtime rejected the update.",
    );
    expectHeldFields(textFields, false);
    expect((screen.getByRole("spinbutton") as HTMLInputElement).readOnly).toBe(false);
    expectDisabledControls(switches, "switch", false);
    expectDisabledControls(tabs, "radio", false);
    expect(field("Server URL").value).toBe("https://tools.example/v2");
    expect(field("Description").value).toBe("Team tools");
    expect(
      screen.getByRole("switch", { name: "Remove stored headers" }).hasAttribute("data-checked"),
    ).toBe(true);
    expect(document.activeElement).toBe(save);
  });

  it("holds stdio configuration during delete and retires earlier test feedback", async () => {
    const deleted = Promise.withResolvers<void>();
    const remove = vi.fn<MCPServerGateway["delete"]>(() => deleted.promise);
    const test = vi.fn<MCPServerGateway["test"]>().mockResolvedValue({ ok: true });
    await mount({ delete: remove, test }, stdioServer);
    const textFields = [
      "Command",
      "Arguments (JSON array)",
      "Environment (JSON object)",
      "Working directory",
      "Description",
    ];
    const status = screen.getByRole("status");
    fireEvent.change(field("Description"), { target: { value: "Local files" } });
    fireEvent.click(screen.getByRole("button", { name: "Test" }));
    await waitFor(() => expect(status.textContent).toContain("Connection OK"));

    const destroy = screen.getByRole("button", { name: "Delete" });
    destroy.focus();
    fireEvent.click(destroy);
    fireEvent.click(
      within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }),
    );
    await waitFor(() => expect(remove).toHaveBeenCalledOnce());

    expect(status.textContent).toBe("");
    expectHeldFields(textFields, true);
    expectDisabledControls(["Remove stored environment"], "switch", true);
    expectDisabledControls(["stdio", "Streamable HTTP"], "radio", true);
    const trigger = screen.getByRole<HTMLButtonElement>("button", { name: "Test" });
    expect(trigger.disabled).toBe(false);
    expect(trigger.getAttribute("aria-disabled")).toBe("true");
    await act(async () => fireEvent.click(trigger));
    expect(test).toHaveBeenCalledOnce();
    expect((screen.getByRole("button", { name: "Cancel" }) as HTMLButtonElement).disabled).toBe(
      false,
    );

    await act(async () => deleted.reject(new Error("The Runtime rejected the removal.")));

    expect((await screen.findByRole("alert")).textContent).toContain(
      "The Runtime rejected the removal.",
    );
    expectHeldFields(textFields, false);
    expectDisabledControls(["Remove stored environment"], "switch", false);
    expectDisabledControls(["stdio", "Streamable HTTP"], "radio", false);
    expect(field("Description").value).toBe("Local files");
    expect(servers()).toEqual([stdioServer]);
    expect(document.activeElement).toBe(destroy);
  });
});
