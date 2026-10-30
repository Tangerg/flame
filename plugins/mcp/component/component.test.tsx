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
    screen.getByLabelText("Environment — KEY=value, one per line"),
    { target: { value: "TOKEN=synthetic-secret" } },
  );
  view.unmount();
  f.view.abort();
  const next = f.openView();
  render(<f.Component context={next.context} />);
  await screen.findByText("docs");
  fireEvent.click(screen.getAllByRole("button", { name: "Add server" })[0]!);
  expect(
    screen.getByLabelText("Environment — KEY=value, one per line"),
  ).toHaveProperty("value", "TOKEN=synthetic-secret");
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
      connection: { type: "stdio", command: "npx" },
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
