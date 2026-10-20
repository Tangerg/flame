import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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

const RUN_ID = RUNTIME_AGENT_SESSION_SNAPSHOTS.idle.runs[0]!.id;
const ENTRIES = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle);
const PAGES = [
  { data: ENTRIES.slice(0, 2), nextCursor: "trajectory-page-2" },
  { data: ENTRIES.slice(2, 4), nextCursor: "trajectory-page-3" },
  { data: ENTRIES.slice(4) },
] as const satisfies readonly Page<TrajectoryEntry>[];
const [FIRST, SECOND, LAST] = PAGES;

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
    signal: owner.signal,
    track: (operation) => operation,
    compose: () => null,
  };
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  const view = render(
    <>
      <button>Outside action</button>
      <Trajectory context={context} />
    </>,
  );
  await waitFor(() => expect(trajectory).toHaveBeenCalledOnce());
  await act(async () => initial.resolve(FIRST));
  await screen.findByText("2 recorded observations loaded on page 1.");
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

const summary = () => screen.getByText(/recorded observations? loaded on page \d+\./);

const refresh = () => screen.getByRole<HTMLButtonElement>("button", { name: "Refresh" });
const newer = () => screen.getByRole<HTMLButtonElement>("button", { name: "Newer records" });
const older = () => screen.getByRole<HTMLButtonElement>("button", { name: "Older records" });

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
  expect(summary().textContent).toBe("2 recorded observations loaded on page 1.");
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
  expect(summary().textContent).toBe("2 recorded observations loaded on page 1.");
  expect(records()).toEqual(firstRecords);
  await act(async () => toSecond.resolve(SECOND));
  expect(summary().textContent).toBe("2 recorded observations loaded on page 2.");
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
  expect(summary().textContent).toBe("2 recorded observations loaded on page 2.");
  expect(records()).toEqual(filtered);
  await act(async () => reread.resolve(SECOND));
  await waitFor(() => expect(screen.queryByText("Reading recorded observations…")).toBeNull());
  expect(summary().textContent).toBe("2 recorded observations loaded on page 2.");
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
  expect(summary().textContent).toBe(
    "2 recorded observations loaded on page 1. Showing the last accepted page.",
  );
  expect(records()).toEqual(firstRecords);
  expect(document.activeElement).toBe(older());

  const retriedOlder = hold();
  await activate(older(), trajectory, 3);
  expect(query(trajectory, 3)).toEqual(query(trajectory, 2));
  await act(async () => retriedOlder.resolve(SECOND));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(summary().textContent).toBe("2 recorded observations loaded on page 2.");
  const secondRecords = records();

  const failedRefresh = hold();
  await activate(refresh(), trajectory, 4);
  await act(async () => failedRefresh.reject(new Error("owner offline")));
  expect(summary().textContent).toBe(
    "2 recorded observations loaded on page 2. Showing the last accepted page.",
  );
  expect(records()).toEqual(secondRecords);
  expect(document.activeElement).toBe(refresh());

  const retriedRefresh = hold();
  await activate(refresh(), trajectory, 5);
  expect(query(trajectory, 5)).toEqual(query(trajectory, 3));
  await act(async () => retriedRefresh.resolve(SECOND));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(summary().textContent).toBe("2 recorded observations loaded on page 2.");
  expect(document.activeElement).toBe(refresh());
});

it("moves explicit navigation to the accepted page summary through middle, last and newest pages", async () => {
  const { trajectory, hold } = await mountTrajectory();
  const toSecond = hold();
  await activate(older(), trajectory, 2);
  await act(async () => toSecond.resolve(SECOND));
  expect(document.activeElement).toBe(summary());
  expect(summary().textContent).toBe("2 recorded observations loaded on page 2.");

  const toLast = hold();
  await activate(older(), trajectory, 3);
  expect(query(trajectory, 3)).toMatchObject({ cursor: SECOND.nextCursor });
  await act(async () => toLast.resolve(LAST));
  expect(summary().textContent).toBe("1 recorded observation loaded on page 3.");
  expect(older().matches(":disabled")).toBe(true);
  expect(document.activeElement).toBe(summary());

  const backToSecond = hold();
  await activate(newer(), trajectory, 4);
  expect(query(trajectory, 4)).toMatchObject({ cursor: FIRST.nextCursor });
  await act(async () => backToSecond.resolve(SECOND));
  expect(summary().textContent).toBe("2 recorded observations loaded on page 2.");
  expect(document.activeElement).toBe(summary());

  const backToNewest = hold();
  await activate(newer(), trajectory, 5);
  expect(query(trajectory, 5)).toMatchObject({ cursor: undefined, includeDescendants: true });
  await act(async () => backToNewest.resolve({ data: FIRST.data }));
  expect(summary().textContent).toBe("2 recorded observations loaded on page 1.");
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
  expect(summary().textContent).toBe("2 recorded observations loaded on page 2.");
  expect(document.activeElement).toBe(search);

  const toLast = hold();
  await activate(older(), trajectory, 3);
  const outside = screen.getByRole<HTMLElement>("button", { name: "Outside action" });
  outside.focus();
  await act(async () => toLast.resolve(LAST));
  expect(summary().textContent).toBe("1 recorded observation loaded on page 3.");
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
  expect(summary().textContent).toBe("2 recorded observations loaded on page 1.");
  await act(async () => restarted.resolve(FIRST));
  await waitFor(() => expect(screen.queryByText("Reading recorded observations…")).toBeNull());
  expect(summary().textContent).toBe("2 recorded observations loaded on page 1.");
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
