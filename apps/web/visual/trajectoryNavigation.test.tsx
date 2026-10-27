import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient, type FlameClient } from "@flame/runtime-client";
import { type Page, type TrajectoryEntry } from "@flame/runtime-client/wire";
import { Trajectory } from "../../../plugins/trajectory/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS, VISUAL_SESSION_ID } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { REVIEW_DIFF } from "./diffSnapshot";
import { visualTrajectory } from "./trajectorySnapshot";

const ROOT_RUN = RUNTIME_AGENT_SESSION_SNAPSHOTS.idle.runs[0]!;
const RUN_ID = ROOT_RUN.id;
const ENTRIES = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle);
const PAGES = [
  { data: ENTRIES.slice(0, 2), nextCursor: "trajectory-page-2" },
  { data: ENTRIES.slice(2, 4), nextCursor: "trajectory-page-3" },
  { data: ENTRIES.slice(4) },
] as const satisfies readonly Page<TrajectoryEntry>[];
const [FIRST, SECOND, LAST] = PAGES;

const CHILD_RUN_ID = "run_child";
const CHILD: Page<TrajectoryEntry> = {
  data: [
    {
      type: "run",
      occurredAt: "2026-07-31T08:00:07.000Z",
      run: { ...ROOT_RUN, id: CHILD_RUN_ID, parentRunId: RUN_ID, rootRunId: RUN_ID },
    },
    {
      type: "model",
      occurredAt: "2026-07-31T08:00:08.000Z",
      model: {
        callId: "call_visual_child",
        runId: CHILD_RUN_ID,
        segmentId: "segment_visual_child",
        state: "completed",
        startedAt: "2026-07-31T08:00:08.000Z",
        settledAt: "2026-07-31T08:00:09.000Z",
      },
    },
  ],
  nextCursor: "trajectory-page-3",
};
const SESSION_ENDPOINT: TrajectoryEntry = {
  type: "endpoint",
  occurredAt: "2026-07-31T08:00:03.000Z",
  endpoint: {
    id: "endpoint_visual_title",
    sessionId: VISUAL_SESSION_ID,
    caller: {
      id: "invocation_visual_title",
      origin: "automatic",
      target: { scope: "session", reference: VISUAL_SESSION_ID },
    },
    kind: "chat",
    provider: ROOT_RUN.provider,
    model: ROOT_RUN.model,
    request: "{}",
    state: "completed",
    startedAt: "2026-07-31T08:00:03.000Z",
    settledAt: "2026-07-31T08:00:04.000Z",
  },
};

const TOOL_DATA: TrajectoryEntry = {
  type: "item",
  occurredAt: "2026-07-31T08:00:05.000Z",
  item: {
    type: "toolCall",
    id: "item_tool_data",
    runId: RUN_ID,
    status: "completed",
    startedAt: "2026-07-31T08:00:05.000Z",
    finishedAt: "2026-07-31T08:00:05.250Z",
    durationMillis: 250,
    tool: {
      name: "inspect_records",
      arguments: { query: { data: "ARGUMENT_PATH_DATA" } },
      result: { rows: [{ data: "RESULT_NESTED_DATA" }] },
    },
  },
};

type TrajectoryRead = ReturnType<FlameClient["sessions"]["trajectory"]>;

const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

async function mountTrajectory() {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const discover = vi.spyOn(runtime.runtime, "discover");
  const trajectory = vi.spyOn(runtime.sessions, "trajectory");
  const hold = () => {
    const read = Promise.withResolvers<Page<TrajectoryEntry>>();
    trajectory.mockImplementationOnce(() => read.promise as TrajectoryRead);
    return read;
  };
  const initial = hold();
  const context: ComponentContext = {
    component: { installationId: "trajectory", digest: "c".repeat(64), componentId: "trajectory" },
    runtime,
    sessionId: VISUAL_SESSION_ID,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    compose: () => null,
  };
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  const view = testClient.render(
    <>
      <button>Outside action</button>
      <Trajectory context={context} />
    </>,
  );
  await waitFor(() => expect(trajectory).toHaveBeenCalledOnce());
  await act(async () => initial.resolve(FIRST));
  await screen.findByText("Page 1 · 2 records");
  return { ...view, context, discover, trajectory, hold };
}

function query(trajectory: { mock: { calls: unknown[][] } }, call: number) {
  return trajectory.mock.calls[call - 1]![0];
}

function signalOf(trajectory: { mock: { calls: unknown[][] } }, call: number) {
  return trajectory.mock.calls[call - 1]![1] as AbortSignal;
}

const records = () =>
  Array.from(document.querySelectorAll("[data-trajectory-record]"), (record) =>
    record.getAttribute("data-trajectory-record"),
  );

const pageScope = () => screen.getByText(/^Page \d+ · /);
const summary = () => pageScope().closest<HTMLParagraphElement>('[role="status"]')!;

const refresh = () => screen.getByRole<HTMLButtonElement>("button", { name: "Refresh" });
const newer = () => screen.getByRole<HTMLButtonElement>("button", { name: "Newer records" });
const older = () => screen.getByRole<HTMLButtonElement>("button", { name: "Older records" });
const runFilter = () => screen.getByRole("combobox", { name: "Run filter" });

async function chooseRun(label: string) {
  fireEvent.click(runFilter());
  const option = await screen.findByRole("option", { name: label });
  const offered = within(option.closest<HTMLElement>('[role="listbox"]')!)
    .getAllByRole("option")
    .map((choice) => choice.textContent);
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  return offered;
}

async function activate(
  control: HTMLButtonElement,
  trajectory: { mock: { calls: unknown[][] } },
  call: number,
) {
  control.focus();
  fireEvent.click(control);
  await waitFor(() => expect(trajectory).toHaveBeenCalledTimes(call));
  expect(screen.getByText("Reading recorded observations…")).toBeTruthy();
  expect(document.activeElement).toBe(control);
  expect(control.matches(":disabled")).toBe(false);
  expect(control.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(control);
  fireEvent.keyDown(control, { key: "Enter" });
  fireEvent.keyDown(control, { key: " " });
  fireEvent.keyUp(control, { key: " " });
  expect(trajectory).toHaveBeenCalledTimes(call);
  expect(signalOf(trajectory, call).aborted).toBe(false);
}

it("starts from the newest page with the negotiated scope and disables only true boundaries", async () => {
  const { discover, trajectory } = await mountTrajectory();
  expect(discover).toHaveBeenCalledOnce();
  expect(query(trajectory, 1)).toEqual({
    sessionId: VISUAL_SESSION_ID,
    includeDescendants: true,
    cursor: undefined,
    limit: 100,
  });
  expect(records()).toHaveLength(2);
  expect(summary().classList.contains("sr-only")).toBe(false);
  expect(pageScope().textContent).toBe("Page 1 · 2 records");
  expect(newer().matches(":disabled")).toBe(true);
  expect(older().matches(":disabled")).toBe(false);
  expect(older().getAttribute("aria-disabled")).toBeNull();
  expect(document.activeElement).toBe(document.body);
});

it("refreshes the accepted older page with its own cursor, scope, filters and content", async () => {
  const { discover, trajectory, hold } = await mountTrajectory();
  const firstRecords = records();
  const toSecond = hold();
  await activate(older(), trajectory, 2);
  expect(pageScope().textContent).toBe("Page 1 · 2 records");
  expect(records()).toEqual(firstRecords);
  await act(async () => toSecond.resolve(SECOND));
  expect(pageScope().textContent).toBe("Page 2 · 2 records");
  const secondRecords = records();
  expect(secondRecords).toHaveLength(2);
  expect(secondRecords.some((record) => firstRecords.includes(record))).toBe(false);
  expect(query(trajectory, 2)).toEqual({
    sessionId: VISUAL_SESSION_ID,
    includeDescendants: true,
    cursor: FIRST.nextCursor,
    limit: 100,
  });

  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter this page" });
  for (const [name, label] of [
    ["Observation type", "Runs"],
    ["Run filter", RUN_ID],
  ] as const) {
    fireEvent.click(screen.getByRole("combobox", { name }));
    const option = await screen.findByRole("option", { name: label });
    fireEvent.pointerDown(option, { pointerType: "mouse" });
    fireEvent.click(option);
  }
  fireEvent.change(search, { target: { value: RUN_ID } });
  const filtered = records();
  expect(filtered).toHaveLength(1);
  const opened = screen.getByRole("button", { name: /^Run · / });
  fireEvent.click(opened);
  expect(opened.getAttribute("aria-expanded")).toBe("true");
  const reread = hold();
  await activate(refresh(), trajectory, 3);
  expect(query(trajectory, 3)).toEqual(query(trajectory, 2));
  expect(discover).toHaveBeenCalledOnce();
  expect(pageScope().textContent).toBe("Page 2 · 1 of 2 records");
  expect(records()).toEqual(filtered);
  await act(async () => reread.resolve(SECOND));
  await waitFor(() => expect(screen.queryByText("Reading recorded observations…")).toBeNull());
  expect(pageScope().textContent).toBe("Page 2 · 1 of 2 records");
  expect(search.value).toBe(RUN_ID);
  expect(screen.getByRole("combobox", { name: "Observation type" }).textContent).toBe("Runs");
  expect(screen.getByRole("combobox", { name: "Run filter" }).textContent).toBe(RUN_ID);
  expect(records()).toEqual(filtered);
  expect(screen.getByRole("button", { name: /^Run · / })).toBe(opened);
  expect(opened.getAttribute("aria-expanded")).toBe("true");
  expect(document.activeElement).toBe(refresh());
  expect(newer().matches(":disabled")).toBe(false);
});

it("retains the previous accepted page on failure and retries from the proper controls", async () => {
  const { trajectory, hold } = await mountTrajectory();
  const firstRecords = records();
  const failedOlder = hold();
  await activate(older(), trajectory, 2);
  await act(async () => failedOlder.reject(new Error("store busy")));
  expect(screen.getByRole("alert").textContent).toBe(
    "The requested page could not be read: store busy",
  );
  expect(pageScope().textContent).toBe("Page 1 · 2 records");
  expect(within(summary()).getByText("Showing the last accepted page.")).toBeTruthy();
  expect(records()).toEqual(firstRecords);
  expect(document.activeElement).toBe(older());

  const retriedOlder = hold();
  await activate(older(), trajectory, 3);
  expect(query(trajectory, 3)).toEqual(query(trajectory, 2));
  await act(async () => retriedOlder.resolve(SECOND));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(pageScope().textContent).toBe("Page 2 · 2 records");
  const secondRecords = records();

  const failedRefresh = hold();
  await activate(refresh(), trajectory, 4);
  await act(async () => failedRefresh.reject(new Error("owner offline")));
  expect(pageScope().textContent).toBe("Page 2 · 2 records");
  expect(within(summary()).getByText("Showing the last accepted page.")).toBeTruthy();
  expect(records()).toEqual(secondRecords);
  expect(document.activeElement).toBe(refresh());

  const retriedRefresh = hold();
  await activate(refresh(), trajectory, 5);
  expect(query(trajectory, 5)).toEqual(query(trajectory, 3));
  await act(async () => retriedRefresh.resolve(SECOND));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(pageScope().textContent).toBe("Page 2 · 2 records");
  expect(document.activeElement).toBe(refresh());
});

it("moves explicit navigation to the accepted page summary through middle, last and newest pages", async () => {
  const { trajectory, hold } = await mountTrajectory();
  const toSecond = hold();
  await activate(older(), trajectory, 2);
  await act(async () => toSecond.resolve(SECOND));
  expect(document.activeElement).toBe(summary());
  expect(pageScope().textContent).toBe("Page 2 · 2 records");

  const toLast = hold();
  await activate(older(), trajectory, 3);
  expect(query(trajectory, 3)).toMatchObject({ cursor: SECOND.nextCursor });
  await act(async () => toLast.resolve(LAST));
  expect(pageScope().textContent).toBe("Page 3 · 1 record");
  expect(older().matches(":disabled")).toBe(true);
  expect(document.activeElement).toBe(summary());

  const backToSecond = hold();
  await activate(newer(), trajectory, 4);
  expect(query(trajectory, 4)).toMatchObject({ cursor: FIRST.nextCursor });
  await act(async () => backToSecond.resolve(SECOND));
  expect(pageScope().textContent).toBe("Page 2 · 2 records");
  expect(document.activeElement).toBe(summary());

  const backToNewest = hold();
  await activate(newer(), trajectory, 5);
  expect(query(trajectory, 5)).toMatchObject({ cursor: undefined, includeDescendants: true });
  await act(async () => backToNewest.resolve({ data: FIRST.data }));
  expect(pageScope().textContent).toBe("Page 1 · 2 records");
  expect(screen.queryByRole("toolbar", { name: "Trajectory pages" })).toBeNull();
  expect(document.activeElement).toBe(summary());
});

it("leaves a later outside or filter focus choice when navigation completes", async () => {
  const { trajectory, hold } = await mountTrajectory();
  const toSecond = hold();
  await activate(older(), trajectory, 2);
  const search = screen.getByRole<HTMLElement>("searchbox", { name: "Filter this page" });
  search.focus();
  await act(async () => toSecond.resolve(SECOND));
  expect(pageScope().textContent).toBe("Page 2 · 2 records");
  expect(document.activeElement).toBe(search);

  const toLast = hold();
  await activate(older(), trajectory, 3);
  const outside = screen.getByRole<HTMLElement>("button", { name: "Outside action" });
  outside.focus();
  await act(async () => toLast.resolve(LAST));
  expect(pageScope().textContent).toBe("Page 3 · 1 record");
  expect(document.activeElement).toBe(outside);
});

it("retires a navigation superseded by a new context without taking the page or focus", async () => {
  const { context, discover, trajectory, hold, rerender } = await mountTrajectory();
  const toSecond = hold();
  await activate(older(), trajectory, 2);
  const restarted = hold();
  rerender(
    <>
      <button>Outside action</button>
      <Trajectory context={{ ...context }} />
    </>,
  );
  await waitFor(() => expect(trajectory).toHaveBeenCalledTimes(3));
  expect(signalOf(trajectory, 2).aborted).toBe(true);
  expect(discover).toHaveBeenCalledTimes(2);
  expect(query(trajectory, 3)).toEqual(query(trajectory, 1));
  await act(async () => toSecond.resolve(SECOND));
  expect(pageScope().textContent).toBe("Page 1 · 2 records");
  await act(async () => restarted.resolve(FIRST));
  await waitFor(() => expect(screen.queryByText("Reading recorded observations…")).toBeNull());
  expect(pageScope().textContent).toBe("Page 1 · 2 records");
  expect(document.activeElement).toBe(older());
});

it("aborts a pending read on unmount and ignores its late result", async () => {
  const error = vi.spyOn(console, "error");
  const { trajectory, hold, unmount } = await mountTrajectory();
  const toSecond = hold();
  await activate(older(), trajectory, 2);
  unmount();
  expect(signalOf(trajectory, 2).aborted).toBe(true);
  await act(async () => toSecond.resolve(SECOND));
  expect(screen.queryByText(/recorded observations? loaded on page/)).toBeNull();
  expect(error).not.toHaveBeenCalled();
});

it("keeps an explicit Run selection on an accepted page without its records", async () => {
  const { trajectory, hold } = await mountTrajectory();
  await chooseRun(RUN_ID);
  const selected = records();
  expect(selected).not.toHaveLength(0);

  const toChild = hold();
  await activate(older(), trajectory, 2);
  await act(async () => toChild.resolve(CHILD));
  expect(runFilter().textContent).toBe(RUN_ID);
  expect(records()).toEqual([]);
  expect(pageScope().textContent).toBe("Page 2 · 0 of 2 records");
  expect(screen.getByText("No records on this page match this filter.")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Clear filters" })).toBeTruthy();
  expect(await chooseRun(RUN_ID)).toEqual(["All Runs", CHILD_RUN_ID, RUN_ID]);
  expect(runFilter().textContent).toBe(RUN_ID);
  expect(records()).toEqual([]);

  const back = hold();
  await activate(newer(), trajectory, 3);
  expect(query(trajectory, 3)).toMatchObject({ cursor: undefined });
  await act(async () => back.resolve(FIRST));
  expect(pageScope().textContent).toBe("Page 1 · 2 records");
  expect(runFilter().textContent).toBe(RUN_ID);
  expect(records()).toEqual(selected);
});

it("lets an explicit All Runs choice stand when the older Run returns", async () => {
  const { trajectory, hold } = await mountTrajectory();
  await chooseRun(RUN_ID);
  const toChild = hold();
  await activate(older(), trajectory, 2);
  await act(async () => toChild.resolve(CHILD));
  await chooseRun("All Runs");
  expect(runFilter().textContent).toBe("All Runs");
  expect(records()).toEqual(["run:run_child", "model:call_visual_child"]);

  const back = hold();
  await activate(newer(), trajectory, 3);
  await act(async () =>
    back.resolve({ data: [...FIRST.data, ...CHILD.data], nextCursor: FIRST.nextCursor }),
  );
  expect(pageScope().textContent).toBe("Page 1 · 4 records");
  expect(runFilter().textContent).toBe("All Runs");
  expect(records()).toHaveLength(4);
  expect(await chooseRun("All Runs")).toEqual(["All Runs", RUN_ID, CHILD_RUN_ID]);
});

it.each<[string, TrajectoryEntry[], string, string[], string]>([
  [
    "another Run's records",
    CHILD.data,
    "No records on this page match this filter.",
    [CHILD_RUN_ID],
    "Page 1 · 0 of 2 records",
  ],
  ["no records", [], "No records on this page.", [], "Page 1 · 0 records"],
])(
  "keeps the selected Run through a refresh that returns %s",
  async (_, data, empty, others, scope) => {
    const { trajectory, hold } = await mountTrajectory();
    await chooseRun(RUN_ID);
    const selected = records();

    const reread = hold();
    await activate(refresh(), trajectory, 2);
    expect(runFilter().textContent).toBe(RUN_ID);
    expect(records()).toEqual(selected);
    await act(async () => reread.resolve({ data }));
    await waitFor(() => expect(screen.queryByText("Reading recorded observations…")).toBeNull());
    expect(pageScope().textContent).toBe(scope);
    expect(runFilter().textContent).toBe(RUN_ID);
    expect(records()).toEqual([]);
    expect(screen.getByText(empty)).toBeTruthy();
    expect(await chooseRun(RUN_ID)).toEqual(["All Runs", ...others, RUN_ID]);

    const failed = hold();
    await activate(refresh(), trajectory, 3);
    expect(query(trajectory, 3)).toEqual(query(trajectory, 2));
    await act(async () => failed.reject(new Error("owner offline")));
    expect(screen.getByRole("alert").textContent).toBe(
      "The requested page could not be read: owner offline",
    );
    expect(pageScope().textContent).toBe(scope);
    expect(within(summary()).getByText("Showing the last accepted page.")).toBeTruthy();
    expect(runFilter().textContent).toBe(RUN_ID);
    expect(records()).toEqual([]);
    expect(document.activeElement).toBe(refresh());
  },
);

it("searches accepted tool data in place without another read", async () => {
  const { discover, trajectory, hold } = await mountTrajectory();
  const withTool = hold();
  await activate(refresh(), trajectory, 2);
  await act(async () => withTool.resolve({ data: [TOOL_DATA, ...FIRST.data] }));
  await waitFor(() => expect(screen.queryByText("Reading recorded observations…")).toBeNull());
  fireEvent.click(screen.getByRole("combobox", { name: "Observation type" }));
  const toolCalls = await screen.findByRole("option", { name: "Tool calls" });
  fireEvent.pointerDown(toolCalls, { pointerType: "mouse" });
  fireEvent.click(toolCalls);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  await chooseRun(RUN_ID);

  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter this page" });
  fireEvent.change(search, { target: { value: "ARGUMENT_PATH_DATA" } });
  expect(records()).toEqual(["item:item_tool_data"]);
  const record = within(
    document.querySelector<HTMLElement>('[data-trajectory-record="item:item_tool_data"]')!,
  );
  const opened = record.getByRole("button", { expanded: false });
  fireEvent.click(opened);
  expect(opened.getAttribute("aria-expanded")).toBe("true");
  expect(record.getByRole("region", { name: "Recorded evidence" }).textContent).toBe(
    JSON.stringify(TOOL_DATA, null, 2),
  );

  search.focus();
  fireEvent.change(search, { target: { value: "result_nested_data" } });
  expect(search.value).toBe("result_nested_data");
  expect(records()).toEqual(["item:item_tool_data"]);
  expect(record.getByRole("button", { expanded: true })).toBe(opened);
  expect(screen.getByRole("combobox", { name: "Observation type" }).textContent).toBe("Tool calls");
  expect(runFilter().textContent).toBe(RUN_ID);
  expect(document.activeElement).toBe(search);
  expect(trajectory).toHaveBeenCalledTimes(2);
  expect(discover).toHaveBeenCalledOnce();
});

it("keeps the Session only selection while its records are absent", async () => {
  const { trajectory, hold } = await mountTrajectory();
  const withSession = { data: [SESSION_ENDPOINT, ...FIRST.data] };
  const toSession = hold();
  await activate(refresh(), trajectory, 2);
  await act(async () => toSession.resolve(withSession));
  await chooseRun("Session only");
  expect(records()).toEqual(["endpoint:endpoint_visual_title"]);

  const withoutSession = hold();
  await activate(refresh(), trajectory, 3);
  await act(async () => withoutSession.resolve(FIRST));
  expect(pageScope().textContent).toBe("Page 1 · 0 of 2 records");
  expect(runFilter().textContent).toBe("Session only");
  expect(records()).toEqual([]);
  expect(await chooseRun("Session only")).toEqual(["All Runs", RUN_ID, "Session only"]);

  const returned = hold();
  await activate(refresh(), trajectory, 4);
  await act(async () => returned.resolve(withSession));
  expect(pageScope().textContent).toBe("Page 1 · 1 of 3 records");
  expect(runFilter().textContent).toBe("Session only");
  expect(records()).toEqual(["endpoint:endpoint_visual_title"]);
});

it("clears every page filter from an empty result without rereading or losing the accepted cursor", async () => {
  const { discover, trajectory, hold } = await mountTrajectory();
  const toSecond = hold();
  await activate(older(), trajectory, 2);
  await act(async () => toSecond.resolve(SECOND));
  const accepted = records();
  await chooseRun(RUN_ID);
  fireEvent.click(screen.getByRole("combobox", { name: "Observation type" }));
  const messages = await screen.findByRole("option", { name: "Messages" });
  fireEvent.pointerDown(messages, { pointerType: "mouse" });
  fireEvent.click(messages);
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter this page" });
  fireEvent.change(search, { target: { value: "missing-record" } });
  expect(records()).toEqual([]);

  const clear = screen.getByRole("button", { name: "Clear filters" });
  clear.focus();
  fireEvent.click(clear);
  expect(search.value).toBe("");
  expect(screen.getByRole("combobox", { name: "Observation type" }).textContent).toBe(
    "All observations",
  );
  expect(runFilter().textContent).toBe("All Runs");
  expect(document.activeElement).toBe(search);
  expect(records()).toEqual(accepted);
  expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
  expect(trajectory).toHaveBeenCalledTimes(2);
  expect(discover).toHaveBeenCalledOnce();

  const reread = hold();
  await activate(refresh(), trajectory, 3);
  expect(query(trajectory, 3)).toEqual(query(trajectory, 2));
  await act(async () => reread.resolve(SECOND));
  expect(records()).toEqual(accepted);
});

it("groups the accepted page and result count once above its records", async () => {
  await mountTrajectory();
  expect(screen.queryByRole("heading", { name: "Recorded activity" })).toBeNull();
  expect(screen.getByText("Page 1 · 2 records").closest('[role="status"]')).toBe(summary());
  expect(screen.queryByText("2 records on this page")).toBeNull();
  expect(screen.queryByText(/recorded observations? loaded on page/)).toBeNull();
});

it("clears local filters without dismissing a failed read or its accepted-page warning", async () => {
  const { trajectory, hold } = await mountTrajectory();
  const accepted = records();
  const search = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter this page" });
  fireEvent.change(search, { target: { value: "missing-record" } });
  const failed = hold();
  await activate(refresh(), trajectory, 2);
  await act(async () => failed.reject(new Error("owner offline")));
  const clear = screen.getByRole("button", { name: "Clear filters" });
  clear.focus();
  fireEvent.click(clear);
  expect(document.activeElement).toBe(search);
  expect(records()).toEqual(accepted);
  expect(pageScope().textContent).toBe("Page 1 · 2 records");
  expect(screen.getByRole("alert").textContent).toBe(
    "The requested page could not be read: owner offline",
  );
  expect(within(summary()).getByText("Showing the last accepted page.")).toBeTruthy();
  expect(trajectory).toHaveBeenCalledTimes(2);
});
