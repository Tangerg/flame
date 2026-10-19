import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  createFlameClient,
  MutationLookupError,
  stringifyJSON,
  type PluginCommandRecovery,
} from "@flame/runtime-contract/client";
import { createMutationPromise } from "@flame/runtime-contract/client/mutation";
import type { CallPluginResponse } from "@flame/runtime-contract/wire";
import type { ComponentContext } from "@flame/plugin-sdk";
import { UsageReports } from "../../../plugins/usagereports/src/component";
import {
  CAPABILITY_JSON,
  type MethodOutputs,
} from "../../../plugins/usagereports/src/methods.generated";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { VISUAL_MEMORY_BINDING, VISUAL_USAGE } from "./componentSnapshots";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { REVIEW_DIFF } from "./diffSnapshot";

const releases: Array<() => Promise<void>> = [];
afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

it("recovers the original capture and preserves exact numbers, zero prices and missing totals", async () => {
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
  const header = {
    id: "a".repeat(64),
    period: "daily",
    observedAt: VISUAL_USAGE.observedAt,
    since: "2026-10-19T00:00:00Z",
  } as const;
  const report: MethodOutputs["flame.usage-reports/get.v1"] = {
    ...header,
    summary: {
      ...VISUAL_USAGE,
      since: header.since,
      total: { inputTokens: 9007199254740993n, outputTokens: 0 },
      recordedCostUsd: 0,
    },
  };
  const binding = {
    ...VISUAL_MEMORY_BINDING,
    contribution: "usage-reports",
    contract: CAPABILITY_JSON,
  };
  let pending: PluginCommandRecovery[] = [];
  let saved = false;
  const recover = vi.fn();
  const capture = vi.fn(async () => {
    pending = [
      {
        idempotencyKey: "original-capture",
        binding: {
          ...binding,
          method: "flame.usage-reports/capture.v1",
          target: { scope: "installation", reference: binding.installationId },
        },
        recover: async (decode) => {
          recover();
          pending = [];
          saved = true;
          return decode(header);
        },
        retire: vi.fn(),
      },
    ];
    throw new MutationLookupError(new Error("receipt unavailable"));
  });
  vi.spyOn(runtime.plugins, "pendingCommands").mockImplementation(() => pending);
  const call = vi.spyOn(runtime.plugins, "call").mockImplementation((request) => {
    if (request.method === "flame.usage-reports/capture.v1")
      return createMutationPromise<CallPluginResponse>(capture, "original-capture");
    return Promise.resolve({
      output: stringifyJSON(
        request.method === "flame.usage-reports/get.v1" ? report : saved ? [header] : [],
      ),
    }) as never;
  });
  const context: ComponentContext = {
    component: { ...binding, componentId: "usage-reports" },
    binding,
    runtime,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    signal: owner.signal,
    track: (operation) => operation,
    compose: () => null,
  };
  render(<UsageReports context={context} />);
  await screen.findByText("No saved reports");
  fireEvent.click(screen.getByRole("button", { name: "Save report" }));
  await screen.findByText(/Confirmation is unknown/);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save report" }).disabled).toBe(
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "Check original outcome" }));
  const detail = (await screen.findByRole("heading", { name: "Daily report" })).closest("section")!;
  await waitFor(() => expect(screen.queryByText(/Confirmation is unknown/)).toBeNull());
  expect(recover).toHaveBeenCalledOnce();
  expect(capture).toHaveBeenCalledOnce();
  expect(
    call.mock.calls.filter(([request]) => request.method === "flame.usage-reports/capture.v1"),
  ).toHaveLength(1);
  expect(within(detail).getByText("9,007,199,254,740,993")).toBeTruthy();
  const facts = within(detail).getByText("Total cost (USD)").closest("dl")!;
  expect(within(facts).getByText("Not reported")).toBeTruthy();
  expect(within(facts).getAllByText("0").length).toBeGreaterThan(0);
});
