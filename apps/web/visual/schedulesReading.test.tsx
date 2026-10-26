import { mutationResult } from "@/test/runtimeClient";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import {
  cleanup,
  fireEvent,
  getDefaultNormalizer,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient, RpcError } from "@flame/runtime-client";
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

function mountSchedules(subscribeError?: Error) {
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
  let subscriptionSettled = false;
  const subscribe = runtime.runtimeEvents.subscribe.bind(runtime.runtimeEvents);
  vi.spyOn(runtime.runtimeEvents, "subscribe").mockImplementation(async (...args) => {
    if (subscribeError) {
      subscriptionSettled = true;
      throw subscribeError;
    }
    const subscription = await subscribe(...args);
    subscriptionSettled = true;
    return subscription;
  });
  let commands = 0;
  const call = vi.spyOn(runtime.plugins, "call").mockImplementation((request, options) => {
    if (request.method === "flame.schedules/list.v1") {
      const read = heldCall(options?.signal);
      reads.push(read);
      return read.promise;
    }
    return mutationResult(
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
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation,
    compose: () => null,
  };
  testClient.render(
    <div data-testid="viewport" style={{ overflowY: "auto", height: 400 }}>
      <Schedules context={context} />
    </div>,
  );
  let taken = 0;
  async function nextRead() {
    await waitFor(() => expect(subscriptionSettled && reads.length > taken).toBe(true));
    taken = reads.length;
    return reads.at(-1)!;
  }
  const listCalls = () =>
    call.mock.calls.filter(([request]) => request.method === "flame.schedules/list.v1").length;
  const pluginCalls = () => call.mock.calls.length;
  return { nextRead, listCalls, pluginCalls };
}

const clearFiltersButton = () => screen.queryByRole("button", { name: "Clear filters" });

const reply = (page: typeof VISUAL_SCHEDULES_PAGE) => ({ output: JSON.stringify(page) });

const refreshButton = () => screen.getByRole<HTMLButtonElement>("button", { name: "Refresh" });

const alertText = async () => (await screen.findByRole("alert")).textContent;

const backendOffline = (detail = "schedules backend offline") =>
  new RpcError({
    code: -32603,
    message: "internal_error",
    data: { type: "internal_error", detail },
  });

const sentenceDetail = "The schedules source is unavailable.\nReconnect it and refresh.";

const literal = { normalizer: getDefaultNormalizer({ trim: false, collapseWhitespace: false }) };

async function expectFailureAlert(summary: string, detail: string) {
  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText(summary).textContent).toBe(summary);
  expect(within(alert).getByText(detail, literal).textContent).toBe(detail);
}

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

  initial.reject(backendOffline());
  await expectFailureAlert(
    "Schedules could not be read. Use Refresh to try again.",
    "schedules backend offline",
  );
  expect(screen.queryByText("No schedules saved.")).toBeNull();
  expect(screen.queryByText("No matching schedules on this page.")).toBeNull();
  expect(screen.queryByText(/enabled ·/)).toBeNull();
  expect(clearFiltersButton()).toBeNull();

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
  fireEvent.change(screen.getByRole("searchbox", { name: "Search this page" }), {
    target: { value: "nightly" },
  });
  expect(screen.getByText("Use Add schedule to save recurring work.")).toBeTruthy();
  expect(screen.queryByText("No matching schedules on this page.")).toBeNull();
  expect(clearFiltersButton()).toBeNull();

  fireEvent.click(refreshButton());
  (await nextRead()).reject(backendOffline());
  await expectFailureAlert(
    "Schedules could not be read. The last accepted page is retained. Use Refresh to try again.",
    "schedules backend offline",
  );
  expect(screen.getByText("No schedules saved.")).toBeTruthy();
});

it("keeps a complete published read detail separate from the localized summary", async () => {
  const { nextRead } = mountSchedules();
  (await nextRead()).reject(backendOffline(sentenceDetail));
  await expectFailureAlert(
    "Schedules could not be read. Use Refresh to try again.",
    sentenceDetail,
  );
  expect(screen.queryByText(/The last accepted page is retained/)).toBeNull();
  expect(screen.queryByText("No schedules saved.")).toBeNull();
  expect(screen.queryByText("No matching schedules on this page.")).toBeNull();
  expect(screen.queryByText(/enabled ·/)).toBeNull();
  expect(refreshButton().getAttribute("aria-disabled")).toBeNull();
});

it("keeps a failed live subscription's published detail separate while the accepted page stays refreshable", async () => {
  const { nextRead, listCalls } = mountSchedules(backendOffline(sentenceDetail));
  (await nextRead()).resolve(reply(VISUAL_SCHEDULES_PAGE));
  await screen.findByRole("heading", { name: nightly });
  await expectFailureAlert(
    "Live refresh is unavailable. Use Refresh to read the current schedules.",
    sentenceDetail,
  );
  expect(screen.getByRole("heading", { name: weekly })).toBeTruthy();
  expect(screen.queryByText(/could not be read/)).toBeNull();
  expect(refreshButton().getAttribute("aria-disabled")).toBeNull();

  const issued = listCalls();
  fireEvent.click(refreshButton());
  (await nextRead()).resolve(reply(VISUAL_SCHEDULES_PAGE));
  await waitFor(() => expect(screen.queryByText("Reading schedules…")).toBeNull());
  expect(listCalls()).toBe(issued + 1);
  expect(screen.getByRole("heading", { name: nightly })).toBeTruthy();
  expect(screen.getByRole("heading", { name: weekly })).toBeTruthy();
});

it("retains accepted and filtered records through a failed refresh and its retry", async () => {
  const { nextRead, pluginCalls } = mountSchedules();
  (await nextRead()).resolve(reply(VISUAL_SCHEDULES_PAGE));
  await screen.findByRole("heading", { name: nightly });
  const query = screen.getByRole<HTMLInputElement>("searchbox", { name: "Search this page" });
  fireEvent.change(query, { target: { value: "nightly" } });
  expect(screen.queryByRole("heading", { name: weekly })).toBeNull();
  const details = screen.getByRole("button", { name: "Schedule details" });
  fireEvent.click(details);
  expect(details.getAttribute("aria-expanded")).toBe("true");

  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;
  fireEvent.click(refreshButton());
  expect(viewport.scrollTop).toBe(125);
  (await nextRead()).reject(new Error("schedules backend offline"));
  expect(await alertText()).toContain("The last accepted page is retained.");
  expect(screen.getByRole("heading", { name: nightly })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Schedule details" })).toBe(details);
  expect(details.getAttribute("aria-expanded")).toBe("true");
  expect(viewport.scrollTop).toBe(125);
  expect(screen.getByText(/1 enabled · 1 disabled on this/)).toBeTruthy();
  fireEvent.change(query, { target: { value: "release notes" } });
  expect(screen.getByText("No matching schedules on this page.")).toBeTruthy();
  expect(screen.queryByText("No schedules saved.")).toBeNull();
  const clearLocally = () => {
    const issued = pluginCalls();
    const clear = screen.getByRole("button", { name: "Clear filters" });
    clear.focus();
    fireEvent.click(clear);
    expect(query.value).toBe("");
    expect(document.activeElement).toBe(query);
    expect(screen.getByRole("heading", { name: nightly })).toBeTruthy();
    expect(screen.getByRole("heading", { name: weekly })).toBeTruthy();
    expect(screen.queryByText("No matching schedules on this page.")).toBeNull();
    expect(pluginCalls()).toBe(issued);
    fireEvent.change(query, { target: { value: "release notes" } });
  };
  clearLocally();
  expect(screen.getByRole("alert").textContent).toContain("The last accepted page is retained.");

  fireEvent.click(refreshButton());
  const retry = await nextRead();
  expect(screen.getByText("Reading schedules…")).toBeTruthy();
  clearLocally();
  retry.resolve(reply(VISUAL_SCHEDULES_PAGE));
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
  const viewport = screen.getByTestId("viewport");
  viewport.scrollTop = 125;
  const refresh = refreshButton();
  refresh.focus();
  fireEvent.click(refresh);
  const held = await nextRead();
  expect(viewport.scrollTop).toBe(125);
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
  expect(viewport.scrollTop).toBe(125);

  fireEvent.click(refresh);
  const later = await nextRead();
  const query = screen.getByRole("searchbox", { name: "Search this page" });
  query.focus();
  viewport.scrollTop = 280;
  later.resolve(reply(VISUAL_SCHEDULES_PAGE));
  await waitFor(() => expect(screen.queryByText("Reading schedules…")).toBeNull());
  expect(document.activeElement).toBe(query);
  expect(viewport.scrollTop).toBe(280);

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
  expect(clearFiltersButton()).toBeNull();
});
