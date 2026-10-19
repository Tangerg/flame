import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ComponentContext } from "@flame/plugin-sdk";
import { createFlameClient } from "@flame/runtime-contract/client";
import { Schedules } from "../../../plugins/schedules/src/component";
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
    signal: owner.signal,
    track: (operation) => operation,
    compose: () => null,
  };
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  render(<Schedules context={context} />);
  await screen.findByRole("heading", { name: VISUAL_SCHEDULES_PAGE.Rows[0]!.Title });
  await waitFor(() => expect(screen.queryByText("Reading schedules…")).toBeNull());
  return { call, runtime };
}

it("opens a bounded editor and returns to the retained catalog on Escape", async () => {
  await mountSchedules();
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(query, { target: { value: "nightly" } });
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
  expect(screen.queryByRole("heading", { name: "Weekly maintenance notes" })).toBeNull();
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
