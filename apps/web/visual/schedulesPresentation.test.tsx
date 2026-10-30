import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { mockSelectLayout, mockSelectViewport } from "@/test/selectLayout";
let testClient: TestClient;
prepareTestClient(() => {
  mockSelectLayout();
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient } from "@flame/runtime-client";
import { Schedules } from "../../../plugins/schedules/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES, VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
import { VISUAL_SCHEDULES_BINDING, VISUAL_SCHEDULES_PAGE } from "./componentSnapshots";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { REVIEW_DIFF } from "./diffSnapshot";
const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

async function mountSchedules() {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const call = vi.spyOn(runtime.plugins, "call");
  const context: ComponentContext = {
    component: { ...VISUAL_SCHEDULES_BINDING, componentId: "schedules" },
    binding: VISUAL_SCHEDULES_BINDING,
    runtime,
    workspace: { ref: { path: VISUAL_WORKSPACE_PATH } },
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose: () => null,
  };
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  testClient.render(
    <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
      <Schedules context={context} />
    </div>,
  );
  mockSelectViewport(screen.getByTestId("viewport"));
  await screen.findByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title });
  await waitFor(() => expect(screen.queryByText("Reading schedules…")).toBeNull());
  return { call, runtime };
}

it("starts a changed page query at its first result without moving search focus or reading again", async () => {
  const { call } = await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  const viewport = screen.getByTestId("viewport");
  const issued = call.mock.calls.length;
  query.focus();
  viewport.scrollTop = 280;

  fireEvent.change(query, { target: { value: "nightly" } });

  expect(viewport.scrollTop).toBe(0);
  expect(screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title })).toBeTruthy();
  expect(screen.queryByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title })).toBeNull();
  expect(document.activeElement).toBe(query);
  viewport.scrollTop = 125;
  fireEvent.change(query, { target: { value: "nightly" } });
  expect(viewport.scrollTop).toBe(125);
  expect(call.mock.calls.length).toBe(issued);
});

it.each(["typing", "button", "Escape"] as const)(
  "reveals the page beginning when clearing search with %s",
  async (method) => {
    const { call } = await mountSchedules();
    const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
    fireEvent.change(query, { target: { value: "nightly" } });
    query.focus();
    const viewport = screen.getByTestId("viewport");
    viewport.scrollTop = 125;
    const issued = call.mock.calls.length;

    let unhandledEscape = false;
    if (method === "typing") fireEvent.change(query, { target: { value: "" } });
    else if (method === "button")
      fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    else unhandledEscape = fireEvent.keyDown(query, { key: "Escape" });

    expect(unhandledEscape).toBe(false);
    expect(viewport.scrollTop).toBe(0);
    expect(query.value).toBe("");
    expect(
      screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title }),
    ).toBeTruthy();
    expect(document.activeElement).toBe(query);
    expect(call.mock.calls.length).toBe(issued);
  },
);

it("starts a changed status filter at its first result and retains the query", async () => {
  const { call } = await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(query, { target: { value: "workspace" } });
  const status = screen.getByRole("combobox", { name: "Schedule status" });
  status.focus();
  fireEvent.click(status);
  const option = await screen.findByRole("option", { name: "Enabled" });
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 280;
  const issued = call.mock.calls.length;

  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);

  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(viewport.scrollTop).toBe(0);
  expect(query.value).toBe("workspace");
  expect(status.textContent).toContain("Enabled");
  expect(document.activeElement).toBe(status);
  expect(screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title })).toBeTruthy();
  expect(screen.queryByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title })).toBeNull();
  expect(call.mock.calls.length).toBe(issued);
});

it("opens a bounded editor and returns to the retained catalog on Escape", async () => {
  await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(query, { target: { value: "nightly" } });
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;
  const add = screen.getByRole("button", { name: "Add schedule" });
  add.focus();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  const title = within(editor).getByRole<HTMLInputElement>("textbox", { name: "Title" });
  await waitFor(() => expect(document.activeElement).toBe(title));
  fireEvent.change(title, { target: { value: "Retained task context" } });
  expect(within(editor).getByRole("button", { name: "Cancel" })).toBeTruthy();
  expect(within(editor).getByRole("button", { name: "Save schedule" })).toBeTruthy();
  fireEvent.keyDown(title, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
  expect(query.value).toBe("nightly");
  expect(viewport.scrollTop).toBe(125);
  expect(screen.queryByRole("heading", { name: "Weekly maintenance notes" })).toBeNull();
});

it("clears a filtered empty page back to its records and search without another Runtime call", async () => {
  const { call } = await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  const status = screen.getByRole("combobox", { name: "Schedule status" });
  fireEvent.click(status);
  const option = await screen.findByRole("option", { name: "Disabled" });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(status.textContent).toContain("Disabled");
  fireEvent.change(query, { target: { value: "release notes" } });
  expect(screen.getByText("No matching schedules on this page.")).toBeTruthy();
  expect(screen.queryByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title })).toBeNull();
  expect(screen.queryByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title })).toBeNull();

  const issued = call.mock.calls.length;
  const clear = screen.getByRole("button", { name: "Clear filters" });
  clear.focus();
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;
  fireEvent.click(clear);
  expect(viewport.scrollTop).toBe(0);
  expect(query.value).toBe("");
  expect(status.textContent).toContain("All statuses");
  expect(screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title })).toBeTruthy();
  expect(screen.getByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[1]!.Title })).toBeTruthy();
  expect(screen.queryByText("No matching schedules on this page.")).toBeNull();
  expect(screen.getByRole("searchbox", { name: "Search this page" })).toBe(query);
  expect(document.activeElement).toBe(query);
  expect(call.mock.calls.length).toBe(issued);
});

it("cancels a bounded deletion decision back to its record without issuing a command", async () => {
  const { call } = await mountSchedules();
  const trigger = screen.getByRole("button", { name: "More actions for Nightly dependency audit" });
  trigger.focus();
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
  const confirmation = await screen.findByRole("alertdialog", {
    name: "Delete Nightly dependency audit?",
  });
  const cancel = within(confirmation).getByRole("button", { name: "Cancel" });
  await waitFor(() => expect(document.activeElement).toBe(cancel));
  fireEvent.keyDown(cancel, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(call.mock.calls.some(([request]) => request.method === "flame.schedules/delete.v1")).toBe(
    false,
  );
});

it.each(["Cancel", "Close schedule editor"])("%s returns to the retained catalog", async (name) => {
  await mountSchedules();
  const add = screen.getByRole("button", { name: "Add schedule" });
  add.focus();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  fireEvent.click(within(editor).getByRole("button", { name }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
});

it("closes model choices before allowing Escape to cancel the editor", async () => {
  await mountSchedules();
  const add = screen.getByRole("button", { name: "Add schedule" });
  add.focus();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  const model = within(editor).getByRole("combobox", { name: "Model" });
  fireEvent.click(model);
  const option = await screen.findByRole("option", { name: "Runtime default model" });
  fireEvent.keyDown(option, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  expect(screen.getByRole("dialog", { name: "New schedule" })).toBe(editor);
  await waitFor(() => expect(document.activeElement).toBe(model));
  fireEvent.keyDown(model, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
});

it("returns an idle edit to its available record opener", async () => {
  await mountSchedules();
  const trigger = screen.getByRole("button", { name: "More actions for Nightly dependency audit" });
  trigger.focus();
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Edit" }));
  const editor = await screen.findByRole("dialog", { name: "Edit Nightly dependency audit" });
  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});
