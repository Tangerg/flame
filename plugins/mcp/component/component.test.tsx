// @vitest-environment happy-dom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { RpcTransportError } from "@flame/runtime-client";
import { MCPServers } from "./component";
import { fixture, mutation, server } from "./testing";
import { de } from "./locales/de";
import { en } from "./locales/en";
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(100, 100, 200, 32),
  );
  vi.spyOn(document.documentElement, "clientWidth", "get").mockReturnValue(
    1024,
  );
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(
    768,
  );
});
const fixtures: ReturnType<typeof fixture>[] = [];
function create(initial = [server]) {
  const f = fixture(initial);
  fixtures.push(f);
  const Component = MCPServers.create({
    runtimeIdentity: f.context.runtime,
    component: f.context.component,
    signal: f.source.signal,
  });
  return { ...f, Component };
}
afterEach(() => {
  cleanup();
  for (const f of fixtures.splice(0)) f.dispose();
  vi.restoreAllMocks();
});
async function mount(f = create()) {
  const view = render(<f.Component context={f.context} />);
  await screen.findByText("docs");
  return { f, view };
}
async function add() {
  fireEvent.click(screen.getAllByRole("button", { name: "Add server" })[0]!);
  const field = screen.getByLabelText<HTMLInputElement>("Server name");
  fireEvent.change(field, { target: { value: "new-server" } });
  fireEvent.change(screen.getByLabelText("Command"), {
    target: { value: "npx" },
  });
  return field;
}
const reason =
  "workspace-tools refused the read\n  publisher: policy store unavailable";
const literal = { normalizer: (text: string) => text };
function readFailure(title: string) {
  return screen
    .queryAllByRole("alert")
    .find((alert) => within(alert).queryByText(title) !== null);
}
const visibilityTitle = en["mcp.error.loadToolVisibility"]!;
const rulesTitle = en["mcp.error.loadAutoApproveRules"]!;
const catalog = {
  data: [
    {
      server: server.id,
      name: "read",
      description: "Read documentation",
      modelName: "read",
      nameConflicts: [],
    },
  ],
};
function readCalls(f: ReturnType<typeof create>) {
  return [f.tools, f.exposure, f.listRules].map(
    (read) => read.mock.calls.length,
  );
}
function expectFocusedPending(action: HTMLElement) {
  expect(screen.getByRole("button", { name: "Loading…" })).toBe(action);
  expect(action.getAttribute("aria-disabled")).toBe("true");
  expect(document.activeElement).toBe(action);
}
function activateAgain(action: HTMLElement) {
  fireEvent.pointerDown(action);
  fireEvent.click(action);
  fireEvent.keyDown(action, { key: "Enter" });
  fireEvent.keyDown(action, { key: " " });
  fireEvent.keyUp(action, { key: " " });
}
function expectNoWrites(f: ReturnType<typeof create>) {
  for (const write of [
    f.create,
    f.update,
    f.remove,
    f.setExposure,
    f.setRule,
    f.forgetRule,
  ])
    expect(write).not.toHaveBeenCalled();
}
it("reads the canonical server and exposes tools without opening its configuration editor", async () => {
  const { f } = await mount();
  fireEvent.click(screen.getByRole("button", { name: "Tools for docs" }));
  expect(await screen.findByText("Read documentation")).toBeTruthy();
  expect(screen.queryByLabelText("Server URL")).toBeNull();
  const on = screen.getByRole("switch", { name: "Auto-approve read" });
  await waitFor(() => expect(on.hasAttribute("disabled")).toBe(false));
  fireEvent.click(on);
  await waitFor(() =>
    expect(f.setRule).toHaveBeenCalledWith({
      tool: { type: "mcp", server: server.id, name: "read" },
      scope: "global",
      subject: { type: "all" },
      decision: "allow",
    }),
  );
});
it("keeps initial loading separate from an accepted empty server list", async () => {
  const f = create([]);
  const result = Promise.withResolvers<{ data: (typeof server)[] }>();
  f.list.mockReturnValue(result.promise);
  render(<f.Component context={f.context} />);
  expect(screen.queryByText("No MCP servers configured")).toBeNull();
  expect(screen.getAllByRole("status").length).toBeGreaterThan(0);
  await act(async () => result.resolve({ data: [] }));
  expect(f.list).toHaveBeenCalled();
  expect(screen.queryByRole("alert")).toBeNull();
});
it("keeps a failed list load visible alongside an add draft and offers Retry", async () => {
  const f = create();
  f.list.mockRejectedValue(new Error("registry unavailable"));
  render(<f.Component context={f.context} />);
  await screen.findByText("registry unavailable");
  await add();
  expect(screen.getByText("registry unavailable")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
});
it("retains an unsubmitted draft when the view closes, then clears it on source retirement", async () => {
  const { f, view } = await mount();
  await add();
  fireEvent.change(
    screen.getByRole("textbox", { name: "Environment (JSON object)" }),
    { target: { value: '{"TOKEN":"synthetic-secret"}' } },
  );
  view.unmount();
  f.view.abort();
  const next = f.openView();
  render(<f.Component context={next.context} />);
  await screen.findByText("docs");
  fireEvent.click(screen.getAllByRole("button", { name: "Add server" })[0]!);
  expect(
    screen.getByRole("textbox", { name: "Environment (JSON object)" }),
  ).toHaveProperty("value", '{"TOKEN":"synthetic-secret"}');
  expect(f.create).not.toHaveBeenCalled();
  f.source.abort();
});
it("focuses the editable field and prevents secret reuse after a changed HTTP origin", async () => {
  await mount();
  const opener = screen.getByRole("button", { name: "Edit docs" });
  fireEvent.click(opener);
  expect(document.activeElement).toBe(screen.getByLabelText("Server URL"));
  expect(screen.getByLabelText("Server name")).toHaveProperty("disabled", true);
  fireEvent.change(screen.getByLabelText("Server URL"), {
    target: { value: "https://another.example.test/mcp" },
  });
  expect(screen.getByRole("button", { name: "Save" })).toHaveProperty(
    "disabled",
    true,
  );
  const clear = screen.getByRole("switch", {
    name: "Remove stored authorization",
  });
  fireEvent.click(clear);
  expect(clear.getAttribute("aria-checked")).toBe("true");
  expect(screen.getByRole("button", { name: "Save" })).toHaveProperty(
    "disabled",
    false,
  );
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(document.activeElement).toBe(opener);
});
it("validates whole-second timeout locally and exposes its field description", async () => {
  await mount();
  await add();
  const field = screen.getByLabelText("Timeout (seconds) — optional");
  fireEvent.change(field, { target: { value: "0" } });
  expect(field.getAttribute("aria-invalid")).toBe("true");
  const id = field.getAttribute("aria-describedby")!;
  expect(document.getElementById(id)?.textContent).toBe(
    "Enter positive whole seconds, or leave blank for no time limit.",
  );
  expect(
    screen.getAllByRole("button", { name: "Add server" })[1],
  ).toHaveProperty("disabled", true);
  fireEvent.change(field, { target: { value: "" } });
  expect(field.getAttribute("aria-invalid")).not.toBe("true");
});
it("keeps an uncertain form submission open and reconciles its original command", async () => {
  const { f, view } = await mount();
  const retry = vi.fn().mockResolvedValue({
    ...server,
    id: { origin: { type: "user" }, name: "new-server" },
  });
  f.create.mockReturnValueOnce(
    mutation(
      Promise.reject(new RpcTransportError("synthetic disconnect")),
      retry,
    ),
  );
  await add();
  fireEvent.click(screen.getAllByRole("button", { name: "Add server" })[1]!);
  await screen.findByRole("button", { name: "Reconcile original command" });
  view.unmount();
  f.view.abort();
  const next = f.openView();
  render(<f.Component context={next.context} />);
  await screen.findByText("docs");
  const reopen = screen.getByRole("button", { name: "Add server" });
  expect(reopen).toHaveProperty("disabled", false);
  fireEvent.click(reopen);
  const reconcile = await screen.findByRole("button", {
    name: "Reconcile original command",
  });
  expect(screen.getByLabelText("Server name")).toHaveProperty(
    "value",
    "new-server",
  );
  fireEvent.click(reconcile);
  await waitFor(() =>
    expect(screen.queryByLabelText("Server name")).toBeNull(),
  );
  expect(f.create).toHaveBeenCalledTimes(1);
  expect(retry).toHaveBeenCalledTimes(1);
});
it("shows a connection probe outcome and keeps test separate from saving", async () => {
  const { f } = await mount();
  await add();
  f.test.mockResolvedValue({ outcome: "authorizationRequired" });
  fireEvent.click(screen.getByRole("button", { name: "Test" }));
  await screen.findByText(
    "This server needs you to sign in before it can be used.",
  );
  expect(f.create).not.toHaveBeenCalled();
  expect(f.update).not.toHaveBeenCalled();
  expect(f.test).toHaveBeenCalledWith(
    expect.objectContaining({
      name: "new-server",
      connection: { type: "stdio", command: "npx", args: [] },
    }),
    expect.any(AbortSignal),
  );
});
it("renders localized import issues with literal untrusted names and retires feedback after editing", async () => {
  const f = create();
  render(<f.Component context={{ ...f.context, locale: "de" }} />);
  fireEvent.click(screen.getByRole("button", { name: de["mcp.import"] }));
  const dialog = screen.getByRole("dialog");
  const field = within(dialog).getByLabelText(de["mcp.import.config"]!);
  const source = JSON.stringify({
    mcpServers: { "<b>{{server}}</b>": { type: "stdio" } },
  });
  fireEvent.change(field, { target: { value: source } });
  fireEvent.click(
    within(dialog).getByRole("button", { name: de["mcp.import.confirm"] }),
  );
  const alert = await within(dialog).findByRole("alert");
  expect(alert.textContent).toContain("<b>{{server}}</b>");
  expect(alert.querySelector("b")).toBeNull();
  expect(field.getAttribute("aria-invalid")).toBe("true");
  fireEvent.change(field, { target: { value: "{}" } });
  expect(within(dialog).queryByRole("alert")).toBeNull();
  expect(field.getAttribute("aria-invalid")).not.toBe("true");
  expect(f.create).not.toHaveBeenCalled();
});
it("keeps a partially failed import's progress and valid JSON without marking it invalid", async () => {
  const { f } = await mount();
  const first = f.create.getMockImplementation()!;
  f.create
    .mockImplementationOnce(first)
    .mockReturnValueOnce(mutation(Promise.reject(new Error("known refusal"))));
  fireEvent.click(screen.getByRole("button", { name: "Import from JSON" }));
  const dialog = screen.getByRole("dialog");
  const field = within(dialog).getByLabelText("JSON configuration");
  const source = JSON.stringify({
    mcpServers: { first: { command: "first" }, second: { command: "second" } },
  });
  fireEvent.change(field, { target: { value: source } });
  fireEvent.click(within(dialog).getByRole("button", { name: "Import" }));
  await within(dialog).findByText("known refusal");
  expect(within(dialog).getByText("Imported 1 of 2 servers.")).toBeTruthy();
  expect(field.getAttribute("aria-invalid")).not.toBe("true");
  expect(field).toHaveProperty("value", source);
  fireEvent.click(within(dialog).getByRole("button", { name: "Import" }));
  await within(dialog).findByText("Imported 2 servers");
  expect(f.create.mock.calls.map(([s]) => s.name)).toEqual([
    "first",
    "second",
    "second",
  ]);
});
it("names identical tool policy read failures and recovers each through its own Retry without disturbing the editor", async () => {
  const f = create();
  f.exposure.mockRejectedValue(new Error(reason));
  f.listRules.mockRejectedValue(new Error(reason));
  await mount(f);
  fireEvent.click(screen.getByRole("button", { name: "Tools for docs" }));
  expect(await screen.findByText("Read documentation")).toBeTruthy();
  await screen.findByText(visibilityTitle);
  const visibility = readFailure(visibilityTitle)!;
  const rules = readFailure(rulesTitle)!;
  expect(visibility).not.toBe(rules);
  expect(within(visibility).queryByText(rulesTitle)).toBeNull();
  for (const alert of [visibility, rules])
    expect(within(alert).getByText(reason, literal)).toBeTruthy();
  expect(readFailure(en["mcp.error.loadTools"]!)).toBeUndefined();
  expect(
    screen
      .getByRole("switch", { name: "Enable read" })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  expect(
    screen
      .getByRole("switch", { name: "Auto-approve read" })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("switch", { name: "Enable read" }));
  fireEvent.click(screen.getByRole("switch", { name: "Auto-approve read" }));
  expectNoWrites(f);

  const opener = screen.getByRole("button", { name: "Edit docs" });
  fireEvent.click(opener);
  const description = screen.getByLabelText<HTMLInputElement>("Description");
  fireEvent.change(description, { target: { value: "Team documentation" } });

  const toolReads = f.tools.mock.calls.length;
  const exposureReads = f.exposure.mock.calls.length;
  const ruleReads = f.listRules.mock.calls.length;
  const exposure =
    Promise.withResolvers<Awaited<ReturnType<typeof f.exposure>>>();
  f.exposure.mockReturnValueOnce(exposure.promise);
  fireEvent.click(within(visibility).getByRole("button", { name: "Retry" }));
  expect(f.tools).toHaveBeenCalledTimes(toolReads + 1);
  expect(f.exposure).toHaveBeenCalledTimes(exposureReads + 1);
  expect(readFailure(visibilityTitle)).toBeUndefined();
  description.focus();
  await act(async () =>
    exposure.resolve({ server: server.id, disabledTools: ["read"] }),
  );
  const enable = screen.getByRole("switch", { name: "Enable read" });
  expect(enable.getAttribute("aria-disabled")).not.toBe("true");
  expect(enable.getAttribute("aria-checked")).toBe("false");
  expect(
    screen
      .getByRole("switch", { name: "Auto-approve read" })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  expect(
    within(readFailure(rulesTitle)!).getByText(reason, literal),
  ).toBeTruthy();
  expect(f.listRules).toHaveBeenCalledTimes(ruleReads);
  expect(document.activeElement).toBe(description);
  expect(description.value).toBe("Team documentation");

  const accepted =
    Promise.withResolvers<Awaited<ReturnType<typeof f.listRules>>>();
  f.listRules.mockReturnValueOnce(accepted.promise);
  fireEvent.click(
    within(readFailure(rulesTitle)!).getByRole("button", { name: "Retry" }),
  );
  expect(f.listRules).toHaveBeenCalledTimes(ruleReads + 1);
  expect(f.tools).toHaveBeenCalledTimes(toolReads + 1);
  expect(f.exposure).toHaveBeenCalledTimes(exposureReads + 1);
  const cancel = screen.getByRole("button", { name: "Cancel" });
  cancel.focus();
  await act(async () => accepted.resolve({ rules: [] }));
  expect(readFailure(rulesTitle)).toBeUndefined();
  expect(readFailure(visibilityTitle)).toBeUndefined();
  const approve = screen.getByRole("switch", { name: "Auto-approve read" });
  expect(approve.getAttribute("aria-disabled")).not.toBe("true");
  expect(approve.getAttribute("aria-checked")).toBe("false");
  expect(
    screen
      .getByRole("switch", { name: "Enable read" })
      .getAttribute("aria-checked"),
  ).toBe("false");
  expect(screen.getByText("Read documentation")).toBeTruthy();
  expect(document.activeElement).toBe(cancel);
  expect(description.value).toBe("Team documentation");
  expectNoWrites(f);
  fireEvent.click(cancel);
  expect(document.activeElement).toBe(opener);
});
it.each([
  ["en", en],
  ["de", de],
] as const)(
  "names a failed %s tool catalog apart from an accepted empty catalog and recovers its descriptions",
  async (locale, catalog) => {
    const f = create();
    f.tools.mockRejectedValueOnce(new Error(reason));
    render(<f.Component context={{ ...f.context, locale }} />);
    await screen.findByText("docs");
    fireEvent.click(
      screen.getByRole("button", {
        name: catalog["mcp.tools.title"]!.replace("{{server}}", "docs"),
      }),
    );
    const title = catalog["mcp.error.loadTools"]!;
    await screen.findByText(title);
    const failure = readFailure(title)!;
    expect(within(failure).getByText(reason, literal)).toBeTruthy();
    expect(screen.getAllByRole("alert")).toEqual([failure]);
    expect(screen.queryByText(catalog["mcp.tools.empty"]!)).toBeNull();
    expect(screen.queryByText("Read documentation")).toBeNull();
    fireEvent.click(
      within(failure).getByRole("button", { name: catalog["retry"]! }),
    );
    expect(await screen.findByText("Read documentation")).toBeTruthy();
    expect(readFailure(title)).toBeUndefined();
    expect(screen.queryByText(catalog["mcp.tools.empty"]!)).toBeNull();
    expect(f.tools).toHaveBeenCalledTimes(2);
    expectNoWrites(f);
  },
);
it("adds no recovery actions to reads that are ready on arrival", async () => {
  const { f } = await mount();
  const toolbarRefresh = screen.getByRole("button", { name: "Refresh" });
  fireEvent.click(screen.getByRole("button", { name: "Tools for docs" }));
  expect(await screen.findByText("Read documentation")).toBeTruthy();
  await waitFor(() => expect(screen.queryByText("Loading…")).toBeNull());
  expect(screen.getAllByRole("button", { name: "Refresh" })).toEqual([
    toolbarRefresh,
  ]);
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();
  expectNoWrites(f);
});
it("keeps a focused visibility Retry as one pending action through its paired rereads until focus leaves", async () => {
  const f = create();
  f.exposure.mockRejectedValue(new Error(reason));
  await mount(f);
  const toolbarRefresh = screen.getByRole("button", { name: "Refresh" });
  fireEvent.click(screen.getByRole("button", { name: "Tools for docs" }));
  await screen.findByText(visibilityTitle);
  const retry = within(readFailure(visibilityTitle)!).getByRole("button", {
    name: "Retry",
  });
  act(() => retry.focus());
  const [tools, exposure, rules] = readCalls(f);

  const rejected =
    Promise.withResolvers<Awaited<ReturnType<typeof f.exposure>>>();
  f.exposure.mockReturnValueOnce(rejected.promise);
  fireEvent.click(retry);
  expect(readCalls(f)).toEqual([tools! + 1, exposure! + 1, rules]);
  expectFocusedPending(retry);
  expect(readFailure(visibilityTitle)).toBeUndefined();
  expect(screen.queryByText(reason, literal)).toBeNull();
  activateAgain(retry);
  expect(readCalls(f)).toEqual([tools! + 1, exposure! + 1, rules]);
  await act(async () => rejected.reject(new Error(reason)));
  const failure = readFailure(visibilityTitle)!;
  expect(within(failure).getByRole("button", { name: "Retry" })).toBe(retry);
  expect(within(failure).getByText(reason, literal)).toBeTruthy();
  expect(retry.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(retry);

  const accepted =
    Promise.withResolvers<Awaited<ReturnType<typeof f.exposure>>>();
  f.exposure.mockReturnValueOnce(accepted.promise);
  fireEvent.click(retry);
  expect(readCalls(f)).toEqual([tools! + 2, exposure! + 2, rules]);
  expectFocusedPending(retry);
  await act(async () =>
    accepted.resolve({ server: server.id, disabledTools: [] }),
  );
  expect(screen.getAllByRole("button", { name: "Refresh" })).toEqual([
    toolbarRefresh,
    retry,
  ]);
  expect(retry.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(retry);
  expect(readFailure(visibilityTitle)).toBeUndefined();
  expect(screen.queryByText(reason, literal)).toBeNull();

  const reread =
    Promise.withResolvers<Awaited<ReturnType<typeof f.exposure>>>();
  f.exposure.mockReturnValueOnce(reread.promise);
  fireEvent.click(retry);
  expect(readCalls(f)).toEqual([tools! + 3, exposure! + 3, rules]);
  expectFocusedPending(retry);
  activateAgain(retry);
  expect(readCalls(f)).toEqual([tools! + 3, exposure! + 3, rules]);
  await act(async () =>
    reread.resolve({ server: server.id, disabledTools: ["read"] }),
  );
  expect(screen.getAllByRole("button", { name: "Refresh" })).toEqual([
    toolbarRefresh,
    retry,
  ]);
  expect(document.activeElement).toBe(retry);
  expect(
    screen
      .getByRole("switch", { name: "Enable read" })
      .getAttribute("aria-checked"),
  ).toBe("false");

  const opener = screen.getByRole("button", { name: "Edit docs" });
  fireEvent.click(opener);
  const description = screen.getByLabelText<HTMLInputElement>("Description");
  act(() => description.focus());
  expect(retry.isConnected).toBe(false);
  expect(screen.getAllByRole("button", { name: "Refresh" })).toEqual([
    toolbarRefresh,
  ]);
  expect(document.activeElement).toBe(description);
  const cancel = screen.getByRole("button", { name: "Cancel" });
  act(() => cancel.focus());
  expect(document.activeElement).toBe(cancel);
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  expect(readCalls(f)).toEqual([tools! + 3, exposure! + 3, rules]);
  expectNoWrites(f);
});
it("rereads only rules from a focused rules Retry and leaves a later editor selection in place", async () => {
  const f = create();
  f.listRules.mockRejectedValue(new Error(reason));
  await mount(f);
  fireEvent.click(screen.getByRole("button", { name: "Tools for docs" }));
  expect(await screen.findByText("Read documentation")).toBeTruthy();
  await screen.findByText(rulesTitle);
  await waitFor(() => expect(screen.queryByText("Loading…")).toBeNull());
  const retry = within(readFailure(rulesTitle)!).getByRole("button", {
    name: "Retry",
  });
  act(() => retry.focus());
  const [tools, exposure, rules] = readCalls(f);
  const accepted =
    Promise.withResolvers<Awaited<ReturnType<typeof f.listRules>>>();
  f.listRules.mockReturnValueOnce(accepted.promise);
  fireEvent.click(retry);
  expect(readCalls(f)).toEqual([tools, exposure, rules! + 1]);
  expectFocusedPending(retry);
  expect(readFailure(rulesTitle)).toBeUndefined();
  activateAgain(retry);
  expect(readCalls(f)).toEqual([tools, exposure, rules! + 1]);

  fireEvent.click(screen.getByRole("button", { name: "Edit docs" }));
  const description = screen.getByLabelText<HTMLInputElement>("Description");
  fireEvent.change(description, { target: { value: "Team documentation" } });
  act(() => {
    description.focus();
    description.setSelectionRange(5, 18);
  });
  expect(retry.isConnected).toBe(false);
  expect(screen.getByText("Loading…").getAttribute("role")).toBe("status");

  await act(async () => accepted.resolve({ rules: [] }));
  expect(document.activeElement).toBe(description);
  expect(description.value).toBe("Team documentation");
  expect([description.selectionStart, description.selectionEnd]).toEqual([
    5, 18,
  ]);
  expect(readFailure(rulesTitle)).toBeUndefined();
  expect(screen.queryByText("Loading…")).toBeNull();
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  expect(
    screen
      .getByRole("switch", { name: "Auto-approve read" })
      .getAttribute("aria-disabled"),
  ).not.toBe("true");
  expect(readCalls(f)).toEqual([tools, exposure, rules! + 1]);
  expectNoWrites(f);
});
it("keeps a focused tool catalog Retry through acceptance as Refresh and retains the catalog when focus leaves a reread", async () => {
  const f = create();
  f.tools.mockRejectedValueOnce(new Error(reason));
  await mount(f);
  const toolbarRefresh = screen.getByRole("button", { name: "Refresh" });
  const toggle = screen.getByRole("button", { name: "Tools for docs" });
  fireEvent.click(toggle);
  const title = en["mcp.error.loadTools"]!;
  await screen.findByText(title);
  const retry = within(readFailure(title)!).getByRole("button", {
    name: "Retry",
  });
  act(() => retry.focus());
  const [tools, exposure, rules] = readCalls(f);

  const accepted = Promise.withResolvers<Awaited<ReturnType<typeof f.tools>>>();
  f.tools.mockReturnValueOnce(accepted.promise);
  fireEvent.click(retry);
  expect(readCalls(f)).toEqual([tools! + 1, exposure! + 1, rules]);
  expectFocusedPending(retry);
  expect(screen.queryByText("Read documentation")).toBeNull();
  activateAgain(retry);
  expect(readCalls(f)).toEqual([tools! + 1, exposure! + 1, rules]);
  await act(async () => accepted.resolve(catalog));
  expect(screen.getByText("Read documentation")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "Refresh" })).toEqual([
    toolbarRefresh,
    retry,
  ]);
  expect(retry.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(retry);
  expect(readFailure(title)).toBeUndefined();

  const reread = Promise.withResolvers<Awaited<ReturnType<typeof f.tools>>>();
  f.tools.mockReturnValueOnce(reread.promise);
  fireEvent.click(retry);
  expect(readCalls(f)).toEqual([tools! + 2, exposure! + 2, rules]);
  expectFocusedPending(retry);
  act(() => toggle.focus());
  expect(retry.isConnected).toBe(false);
  expect(screen.queryByText("Loading…")).toBeNull();
  expect(screen.getByText("Read documentation")).toBeTruthy();
  await act(async () => reread.resolve(catalog));
  expect(document.activeElement).toBe(toggle);
  expect(screen.getAllByRole("button", { name: "Refresh" })).toEqual([
    toolbarRefresh,
  ]);
  expect(readCalls(f)).toEqual([tools! + 2, exposure! + 2, rules]);
  expectNoWrites(f);
});
it.each(["view", "source"] as const)(
  "ignores a focused visibility reread completing after %s retirement",
  async (retired) => {
    const f = create();
    f.exposure.mockRejectedValue(new Error(reason));
    const { view } = await mount(f);
    fireEvent.click(screen.getByRole("button", { name: "Tools for docs" }));
    await screen.findByText(visibilityTitle);
    const retry = within(readFailure(visibilityTitle)!).getByRole("button", {
      name: "Retry",
    });
    act(() => retry.focus());
    const late =
      Promise.withResolvers<Awaited<ReturnType<typeof f.exposure>>>();
    f.exposure.mockReturnValueOnce(late.promise);
    fireEvent.click(retry);
    expectFocusedPending(retry);
    if (retired === "view") {
      view.unmount();
      f.view.abort();
    } else act(() => f.source.abort());
    expect(retry.isConnected).toBe(false);
    const settled = document.activeElement;
    await act(async () =>
      late.resolve({ server: server.id, disabledTools: [] }),
    );
    expect(document.activeElement).toBe(settled);
    expect(retry.isConnected).toBe(false);
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Loading…" })).toBeNull();
    expect(screen.queryByText(visibilityTitle)).toBeNull();
    expectNoWrites(f);
  },
);
