import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ComponentContext } from "@flame/component-sdk";
import { createFlameClient } from "@flame/runtime-client";
import { Usage } from "../../../plugins/usage/component/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { VISUAL_USAGE } from "./componentSnapshots";
import { REVIEW_DIFF } from "./diffSnapshot";
const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

async function openUsage() {
  const owner = new AbortController();
  const runtime = createFlameClient(
    createVisualRuntimeTransport(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => REVIEW_DIFF,
    ),
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const summary = vi.spyOn(runtime.usage, "summary");
  const context: ComponentContext = {
    component: { installationId: "usage-review", digest: "a".repeat(64), componentId: "usage" },
    runtime,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation,
    compose: () => null,
  };
  releases.push(async () => {
    owner.abort();
    await runtime.close();
  });
  render(<Usage context={context} />);
  await screen.findByText("Recorded usage loaded. All time.");
  return summary;
}

async function choose(name: string, label: string) {
  fireEvent.click(screen.getByRole("combobox", { name }));
  const option = await screen.findByRole("option", { name: label });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
}

it("establishes the report range before local filters and preserves total meaning while filtering", async () => {
  const summary = await openUsage();
  const scope = screen.getByRole("group", { name: "Report scope" });
  const filter = screen.getByRole("searchbox", { name: "Filter sources" });
  expect(scope.compareDocumentPosition(filter) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  const totals = screen.getByLabelText("Recorded usage totals");
  const before = totals.textContent;
  fireEvent.change(filter, { target: { value: "anthropic" } });
  expect(totals.textContent).toBe(before);
  expect(screen.queryByRole("heading", { name: "openai" })).toBeNull();
  expect(screen.getByRole("heading", { name: "anthropic" })).toBeTruthy();
  expect(summary).toHaveBeenCalledOnce();
  await choose("Period", "Last 7 days");
  await screen.findByText("Recorded usage loaded. Last 7 days.");
  expect(summary).toHaveBeenLastCalledWith({ sinceDays: 7 }, expect.any(AbortSignal));
});

it("keeps the accepted period beside the requested range after failure and recovers on refresh", async () => {
  const summary = await openUsage();
  summary.mockRejectedValueOnce(new Error("Report read failed"));
  await choose("Period", "Last 30 days");
  await screen.findByRole("alert");
  const status = screen.getByRole("status");
  expect(status.textContent).toBe("Showing all time from the last successful read.");
  expect(status.closest('[data-slot="panel-controls"]')).not.toBeNull();
  expect(screen.getByRole("combobox", { name: "Period" }).textContent).toContain("Last 30 days");
  expect(screen.getByLabelText("Recorded usage totals").textContent).toContain("4.12");
  summary.mockResolvedValueOnce(VISUAL_USAGE);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText("Recorded usage loaded. Last 30 days.");
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
});

it("keeps a pending Refresh focusable and inert while the accepted report, filter and grouping remain", async () => {
  const summary = await openUsage();
  const pending = [
    Promise.withResolvers<typeof VISUAL_USAGE>(),
    Promise.withResolvers<typeof VISUAL_USAGE>(),
    Promise.withResolvers<typeof VISUAL_USAGE>(),
  ];
  for (const deferred of pending) summary.mockImplementationOnce(() => deferred.promise);
  await choose("Group by", "Model");
  const filter = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter sources" });
  fireEvent.change(filter, { target: { value: "anthropic" } });
  const totals = screen.getByLabelText("Recorded usage totals").textContent;
  const refresh = screen.getByRole<HTMLButtonElement>("button", { name: "Refresh" });

  function expectAcceptedScope() {
    expect(screen.getByLabelText("Recorded usage totals").textContent).toBe(totals);
    expect(screen.getByRole("combobox", { name: "Period" }).textContent).toContain("All time");
    expect(screen.getByRole("combobox", { name: "Group by" }).textContent).toContain("Model");
    expect(filter.value).toBe("anthropic");
    expect(screen.getByRole("heading", { name: "anthropic/claude-opus-5" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "openai/gpt-5.6-sol" })).toBeNull();
  }

  async function startHeldRead(call: number) {
    refresh.focus();
    fireEvent.click(refresh);
    await waitFor(() => expect(summary).toHaveBeenCalledTimes(call));
    const [query, signal] = summary.mock.calls[call - 1]!;
    expect(query).toEqual({});
    expect(screen.getByText("Reading usage…")).toBeTruthy();
    expect(screen.getByText("Showing all time from the last successful read.")).toBeTruthy();
    expectAcceptedScope();
    expect(document.activeElement).toBe(refresh);
    refresh.blur();
    refresh.focus();
    expect(document.activeElement).toBe(refresh);
    expect(refresh.matches(":disabled")).toBe(false);
    expect(refresh.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(refresh);
    fireEvent.keyDown(refresh, { key: "Enter" });
    fireEvent.keyDown(refresh, { key: " " });
    fireEvent.keyUp(refresh, { key: " " });
    expect(summary).toHaveBeenCalledTimes(call);
    expect(signal!.aborted).toBe(false);
  }

  await startHeldRead(2);
  await act(async () => pending[0]!.resolve(VISUAL_USAGE));
  await screen.findByText("Recorded usage loaded. All time.");
  expect(refresh.getAttribute("aria-disabled")).not.toBe("true");
  expect(document.activeElement).toBe(refresh);
  expectAcceptedScope();

  await startHeldRead(3);
  await act(async () => pending[1]!.reject(new Error("Report read failed")));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "The current report could not be read: Report read failed",
  );
  expect(screen.getByText("Showing all time from the last successful read.")).toBeTruthy();
  expect(document.activeElement).toBe(refresh);
  expectAcceptedScope();

  await startHeldRead(4);
  filter.focus();
  await act(async () => pending[2]!.resolve(VISUAL_USAGE));
  await screen.findByText("Recorded usage loaded. All time.");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(document.activeElement).toBe(filter);
  expectAcceptedScope();
});
