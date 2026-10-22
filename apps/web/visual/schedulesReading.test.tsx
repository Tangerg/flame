import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient, createMutationPromise } from "@flame/runtime-client";
import { type CallPluginResponse } from "@flame/runtime-client/wire";
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

const nightly = VISUAL_SCHEDULES_PAGE.Rows[0]!.Title;

const weekly = VISUAL_SCHEDULES_PAGE.Rows[1]!.Title;

function heldCall(signal?: AbortSignal) {
  const read = Promise.withResolvers<CallPluginResponse>();
  const abort = () => read.reject(signal?.reason);
  if (signal?.aborted) abort();
  else signal?.addEventListener("abort", abort, { once: true });
  return {
    ...read,
    promise: read.promise.finally(() => signal?.removeEventListener("abort", abort)),
  };
}

function mountSchedules() {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  const reads: Array<PromiseWithResolvers<CallPluginResponse>> = [];
  let subscribed = false;
  const subscribe = runtime.runtimeEvents.subscribe.bind(runtime.runtimeEvents);
  vi.spyOn(runtime.runtimeEvents, "subscribe").mockImplementation(async (...args) => {
    const subscription = await subscribe(...args);
    subscribed = true;
    return subscription;
  });
  let commands = 0;
  const call = vi.spyOn(runtime.plugins, "call").mockImplementation((request, options) => {
    if (request.method === "flame.schedules/list.v1") {
      const read = heldCall(options?.signal);
      reads.push(read);
      return read.promise;
    }
    return createMutationPromise(
      (_, attempt) => heldCall(attempt.signal).promise,
      `schedules-command-${++commands}`,
      { signal: options?.signal },
    );
  });
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
  render(<Schedules context={context} />);
  let taken = 0;
  async function nextRead() {
    await waitFor(() => expect(subscribed && reads.length > taken).toBe(true));
    taken = reads.length;
    return reads.at(-1)!;
  }
  const listCalls = () =>
    call.mock.calls.filter(([request]) => request.method === "flame.schedules/list.v1").length;
  return { nextRead, listCalls };
}

const reply = (page: typeof VISUAL_SCHEDULES_PAGE) => ({ output: JSON.stringify(page) });

const refreshButton = () => screen.getByRole<HTMLButtonElement>("button", { name: "Refresh" });

const alertText = async () => (await screen.findByRole("alert")).textContent;

function activateRepeatedly(button: HTMLElement) {
  fireEvent.pointerDown(button, { pointerType: "mouse" });
  fireEvent.click(button);
  for (const key of ["Enter", " "]) {
    fireEvent.keyDown(button, { key });
    fireEvent.keyUp(button, { key });
    fireEvent.click(button);
  }
}

it("names a failed initial read without claiming saved or retained schedules, then recovers", async () => {
  const { nextRead } = mountSchedules();
  const initial = await nextRead();
  expect(screen.getByText("Reading schedules…")).toBeTruthy();
  expect(screen.queryByText("No schedules saved.")).toBeNull();
  expect(refreshButton().disabled).toBe(false);
  expect(refreshButton().getAttribute("aria-disabled")).toBe("true");

  initial.reject(new Error("schedules backend offline"));
  expect(await alertText()).toBe(
    "Schedules could not be read: schedules backend offline. Use Refresh to try again.",
  );
  expect(screen.queryByText("No schedules saved.")).toBeNull();
  expect(screen.queryByText("No matching schedules on this page.")).toBeNull();
  expect(screen.queryByText(/enabled ·/)).toBeNull();

  fireEvent.click(refreshButton());
  expect(screen.queryByRole("alert")).toBeNull();
  (await nextRead()).resolve(reply(VISUAL_SCHEDULES_PAGE));
  await screen.findByRole("heading", { name: nightly });
  expect(screen.getByRole("heading", { name: weekly })).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
});

it("keeps an accepted empty catalog distinct from its failed refresh", async () => {
  const { nextRead } = mountSchedules();
  (await nextRead()).resolve(reply({ Rows: [], NextCursor: "" }));
  await screen.findByText("No schedules saved.");
  expect(screen.queryByRole("alert")).toBeNull();

  fireEvent.click(refreshButton());
  (await nextRead()).reject(new Error("schedules backend offline"));
  expect(await alertText()).toBe(
    "Schedules could not be read: schedules backend offline. The last accepted page is retained. Use Refresh to try again.",
  );
  expect(screen.getByText("No schedules saved.")).toBeTruthy();
});

it("retains accepted and filtered records through a failed refresh and its retry", async () => {
  const { nextRead } = mountSchedules();
  (await nextRead()).resolve(reply(VISUAL_SCHEDULES_PAGE));
  await screen.findByRole("heading", { name: nightly });
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(query, { target: { value: "nightly" } });
  expect(screen.queryByRole("heading", { name: weekly })).toBeNull();
  const details = screen.getByRole("button", { name: "Schedule details" });
  fireEvent.click(details);
  expect(details.getAttribute("aria-expanded")).toBe("true");

  fireEvent.click(refreshButton());
  (await nextRead()).reject(new Error("schedules backend offline"));
  expect(await alertText()).toContain("The last accepted page is retained.");
  expect(screen.getByRole("heading", { name: nightly })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Schedule details" })).toBe(details);
  expect(details.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByText(/1 enabled · 1 disabled on this/)).toBeTruthy();
  fireEvent.change(query, { target: { value: "release notes" } });
  expect(screen.getByText("No matching schedules on this page.")).toBeTruthy();
  expect(screen.queryByText("No schedules saved.")).toBeNull();

  fireEvent.click(refreshButton());
  (await nextRead()).resolve(reply(VISUAL_SCHEDULES_PAGE));
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  expect(screen.getByText("No matching schedules on this page.")).toBeTruthy();
  expect(query.value).toBe("release notes");
  fireEvent.change(query, { target: { value: "" } });
  expect(screen.getByRole("heading", { name: nightly })).toBeTruthy();
  expect(screen.getByRole("heading", { name: weekly })).toBeTruthy();
});

it("keeps a held Refresh focused and single, yields to a later choice and disables for commands", async () => {
  const { nextRead, listCalls } = mountSchedules();
  (await nextRead()).resolve(reply(VISUAL_SCHEDULES_PAGE));
  await screen.findByRole("heading", { name: nightly });
  const refresh = refreshButton();
  refresh.focus();
  fireEvent.click(refresh);
  const held = await nextRead();
  const issued = listCalls();
  activateRepeatedly(refresh);
  expect(listCalls()).toBe(issued);
  expect(document.activeElement).toBe(refresh);
  expect(refresh.disabled).toBe(false);
  expect(refresh.getAttribute("aria-disabled")).toBe("true");
  held.resolve(reply(VISUAL_SCHEDULES_PAGE));
  await waitFor(() => expect(screen.queryByText("Reading schedules…")).toBeNull());
  expect(document.activeElement).toBe(refresh);
  expect(refresh.getAttribute("aria-disabled")).toBeNull();

  fireEvent.click(refresh);
  const later = await nextRead();
  const query = screen.getByRole("searchbox", { name: "Search this page" });
  query.focus();
  later.resolve(reply(VISUAL_SCHEDULES_PAGE));
  await waitFor(() => expect(screen.queryByText("Reading schedules…")).toBeNull());
  expect(document.activeElement).toBe(query);

  fireEvent.click(screen.getAllByRole("button", { name: /^More actions for/ })[0]!);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Run now" }));
  await screen.findByText("Run schedule now…");
  expect(refresh.disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Add schedule" }).disabled).toBe(
    true,
  );
});

it("opens and cancels a new schedule from an unavailable catalog with the search retained", async () => {
  const { nextRead } = mountSchedules();
  (await nextRead()).reject(new Error("schedules backend offline"));
  await screen.findByRole("alert");
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(query, { target: { value: "nightly" } });
  const add = screen.getByRole("button", { name: "Add schedule" });
  add.focus();
  fireEvent.click(add);
  const editor = await screen.findByRole("dialog", { name: "New schedule" });
  fireEvent.click(within(editor).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(add));
  expect(query.value).toBe("nightly");
  expect(screen.queryByText("No matching schedules on this page.")).toBeNull();
});
