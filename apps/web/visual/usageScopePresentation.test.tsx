import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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

async function period(label: string) {
  fireEvent.click(screen.getByRole("combobox", { name: "Period" }));
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
  await period("Last 7 days");
  await screen.findByText("Recorded usage loaded. Last 7 days.");
  expect(summary).toHaveBeenLastCalledWith({ sinceDays: 7 }, expect.any(AbortSignal));
});

it("keeps the accepted period beside the requested range after failure and recovers on refresh", async () => {
  const summary = await openUsage();
  summary.mockRejectedValueOnce(new Error("Report read failed"));
  await period("Last 30 days");
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
