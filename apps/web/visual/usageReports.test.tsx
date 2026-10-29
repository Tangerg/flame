import { mutationResult } from "@/test/runtimeClient";
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
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { stringifyJSON } from "@qimu/sdk/json";
import {
  createFlameClient,
  MutationLookupError,
  type PluginCommandRecovery,
} from "@flame/runtime-client";

import { type CallPluginResponse } from "@flame/runtime-client/wire";
import { type ComponentContext } from "@flame/component-sdk";
import { UsageReports } from "../../../plugins/usagereports/component/component";
import {
  CAPABILITY_JSON,
  type MethodOutputs,
} from "../../../plugins/usagereports/component/methods.generated";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { VISUAL_MEMORY_BINDING, VISUAL_USAGE } from "./componentSnapshots";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { REVIEW_DIFF } from "./diffSnapshot";
type Header = MethodOutputs["flame.usage-reports/list.v1"][number];

type Report = MethodOutputs["flame.usage-reports/get.v1"];

type Runtime = ReturnType<typeof createFlameClient>;

type Call = Parameters<Runtime["plugins"]["call"]>[0];

const releases: Array<() => Promise<void>> = [];

afterEach(async () => {
  cleanup();
  await Promise.all(releases.splice(0).map((release) => release()));
  vi.restoreAllMocks();
});

const binding = {
  ...VISUAL_MEMORY_BINDING,
  contribution: "usage-reports",
  contract: CAPABILITY_JSON,
};

const daily: Header = {
  id: "a".repeat(64),
  period: "daily",
  observedAt: "2026-10-20T08:00:00Z",
  since: "2026-10-19T08:00:00Z",
};

const weekly: Header = {
  id: "b".repeat(64),
  period: "weekly",
  observedAt: "2026-10-20T09:00:00Z",
  since: "2026-10-13T09:00:00Z",
};

const saved = (header: Header): Report => ({
  ...header,
  summary: { ...VISUAL_USAGE, observedAt: header.observedAt, since: header.since },
});

const reply = (value: unknown) =>
  Promise.resolve<CallPluginResponse>({ output: stringifyJSON(value) });

const reportId = (request: Call) => (JSON.parse(request.input) as { id: string }).id;

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
};

const original = (method: string, recover: PluginCommandRecovery["recover"]) => ({
  idempotencyKey: `original-${method}`,
  binding: {
    ...binding,
    method,
    target: { scope: "installation" as const, reference: binding.installationId },
  },
  recover,
  retire: vi.fn(),
  matchesIntent: () => false,
});

function mount(
  handle: (request: Call) => Promise<CallPluginResponse>,
  pending: () => PluginCommandRecovery[] = () => [],
) {
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
  vi.spyOn(runtime.plugins, "pendingCommands").mockImplementation(pending);
  const call = vi
    .spyOn(runtime.plugins, "call")
    .mockImplementation((request) => handle(request) as never);
  const context: ComponentContext = {
    get sourceSignal() {
      return this.signal;
    },
    component: { ...binding, componentId: "usage-reports" },
    binding,
    runtime,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    compose: () => null,
  };
  testClient.render(<UsageReports context={context} />);
  const calls = (method: string) =>
    call.mock.calls.filter(([request]) => request.method === `flame.usage-reports/${method}.v1`);
  return { calls };
}

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
        matchesIntent: () => false,
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
      return mutationResult<CallPluginResponse>(capture, "original-capture");
    return Promise.resolve({
      output: stringifyJSON(
        request.method === "flame.usage-reports/get.v1" ? report : saved ? [header] : [],
      ),
    }) as never;
  });
  const context: ComponentContext = {
    get sourceSignal() {
      return this.signal;
    },
    component: { ...binding, componentId: "usage-reports" },
    binding,
    runtime,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    compose: () => null,
  };
  testClient.render(<UsageReports context={context} />);
  await screen.findByText("No saved reports");
  fireEvent.click(screen.getByRole("button", { name: "Save report" }));
  await screen.findByText(/Confirmation is unknown/);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Save report" }).disabled).toBe(
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "Check original outcome" }));
  // The reading view names its report by period and core observation.
  const detail = (await screen.findByRole("heading", { name: /^Daily report · / })).closest(
    "section",
  )!;
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

it("reads a named report at the pane top and returns to the same archive row", async () => {
  const list = deferred<CallPluginResponse>();
  const read = deferred<CallPluginResponse>();
  mount((request) =>
    request.method === "flame.usage-reports/list.v1" ? list.promise : read.promise,
  );
  expect(screen.getByText("Reading saved reports…")).toBeTruthy();
  expect(screen.queryByText("No saved reports")).toBeNull();
  list.resolve({ output: stringifyJSON([daily, weekly]) });

  const pane = screen.getByRole("region", { name: "Saved usage reports" }).parentElement!;
  pane.style.overflowY = "auto";
  pane.scrollTop = 480;
  const opener = await screen.findByRole("button", { name: /^View report: Weekly report · / });
  opener.focus();
  fireEvent.click(opener);

  const heading = screen.getByRole("heading", { name: /^Weekly report · / });
  expect(document.activeElement).toBe(heading);
  expect(pane.scrollTop).toBe(0);
  expect(screen.queryByRole("button", { name: /^View report/ })).toBeNull();
  expect(screen.getByText("Weekly · last 7 days")).toBeTruthy();
  expect(screen.getByText("Reading saved report…")).toBeTruthy();

  const back = screen.getByRole("button", { name: "Back to archive" });
  back.focus();
  read.resolve({ output: stringifyJSON(saved(weekly)) });
  await screen.findByText("Total cost (USD)");
  expect(document.activeElement).toBe(back);

  fireEvent.click(back);
  expect(document.activeElement).toBe(opener);
  expect(pane.scrollTop).toBe(480);
  expect(screen.getAllByRole("button", { name: /^View report/ })).toHaveLength(2);
  expect(
    screen.getByText("2 saved reports. Capacity is 64; remove a report to make room."),
  ).toBeTruthy();
});

function heldReads() {
  const reads: Array<ReturnType<typeof deferred<CallPluginResponse>>> = [];
  const { calls } = mount((request) => {
    if (request.method === "flame.usage-reports/list.v1") return reply([daily, weekly]);
    const read = deferred<CallPluginResponse>();
    reads.push(read);
    return read.promise;
  });
  return { reads, calls };
}

const exact = (header: Header): Report => {
  const report = saved(header);
  return {
    ...report,
    summary: { ...report.summary, total: { inputTokens: 9007199254740993n, outputTokens: 0 } },
  };
};

function activateRepeatedly(button: HTMLElement) {
  fireEvent.pointerDown(button, { pointerType: "mouse" });
  fireEvent.click(button);
  for (const key of ["Enter", " "]) {
    fireEvent.keyDown(button, { key });
    fireEvent.keyUp(button, { key });
    fireEvent.click(button);
  }
}

it("keeps a held report refresh focusable, single and on the accepted report", async () => {
  const { reads, calls } = heldReads();
  fireEvent.click(await screen.findByRole("button", { name: /^View report: Weekly report · / }));
  const heading = screen.getByRole("heading", { name: /^Weekly report · / });
  const refresh = screen.getByRole<HTMLButtonElement>("button", { name: "Refresh report" });
  expect(refresh.disabled).toBe(false);
  expect(refresh.getAttribute("aria-disabled")).toBe("true");
  activateRepeatedly(refresh);
  expect(calls("get")).toHaveLength(1);
  reads[0]!.reject(new Error("report offline"));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "The saved report could not be read: report offline.",
  );
  expect(screen.queryByText("Total cost (USD)")).toBeNull();

  refresh.focus();
  fireEvent.click(refresh);
  expect(screen.queryByRole("alert")).toBeNull();
  reads[1]!.resolve({ output: stringifyJSON(exact(weekly)) });
  await screen.findByText("9,007,199,254,740,993");

  fireEvent.click(refresh);
  expect(document.activeElement).toBe(refresh);
  expect(refresh.disabled).toBe(false);
  expect(refresh.getAttribute("aria-disabled")).toBe("true");
  expect(screen.getByText("Reading saved report…")).toBeTruthy();
  expect(screen.getByText("9,007,199,254,740,993")).toBeTruthy();
  activateRepeatedly(refresh);
  expect(calls("get")).toHaveLength(3);
  reads[2]!.resolve({ output: stringifyJSON(exact(weekly)) });
  await waitFor(() => expect(screen.queryByText("Reading saved report…")).toBeNull());
  expect(document.activeElement).toBe(refresh);
  expect(refresh.getAttribute("aria-disabled")).toBeNull();
  expect(screen.getByText("9,007,199,254,740,993")).toBeTruthy();

  fireEvent.click(refresh);
  reads[3]!.reject(new Error("report offline again"));
  expect((await screen.findByRole("alert")).textContent).toBe(
    "The saved report could not be refreshed: report offline again. The last accepted report remains visible.",
  );
  expect(document.activeElement).toBe(refresh);
  expect(screen.getByText("9,007,199,254,740,993")).toBeTruthy();

  fireEvent.click(refresh);
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByText("9,007,199,254,740,993")).toBeTruthy();
  reads[4]!.resolve({ output: stringifyJSON(exact(weekly)) });
  await waitFor(() => expect(screen.queryByText("Reading saved report…")).toBeNull());
  expect(document.activeElement).toBe(refresh);
  expect(screen.queryByRole("alert")).toBeNull();

  fireEvent.click(refresh);
  const remove = screen.getByRole("button", { name: "Remove saved report" });
  remove.focus();
  reads[5]!.resolve({ output: stringifyJSON(exact(weekly)) });
  await waitFor(() => expect(screen.queryByText("Reading saved report…")).toBeNull());
  expect(document.activeElement).toBe(remove);

  refresh.focus();
  fireEvent.click(refresh);
  const back = screen.getByRole("button", { name: "Back to archive" });
  back.focus();
  reads[6]!.resolve({ output: stringifyJSON(exact(weekly)) });
  await waitFor(() => expect(screen.queryByText("Reading saved report…")).toBeNull());
  expect(document.activeElement).toBe(back);
  expect(screen.getByRole("heading", { name: /^Weekly report · / })).toBe(heading);
  expect(screen.getByText("Weekly · last 7 days")).toBeTruthy();
  expect(calls("get").map(([request]) => reportId(request))).toEqual(Array(7).fill(weekly.id));
});

it("retires a pending report refresh on Back and returns to the same archive row", async () => {
  const { reads, calls } = heldReads();
  const pane = screen.getByRole("region", { name: "Saved usage reports" }).parentElement!;
  pane.style.overflowY = "auto";
  mockSelectViewport(pane);
  const opener = await screen.findByRole("button", { name: /^View report: Weekly report · / });
  fireEvent.click(screen.getByRole("combobox", { name: "Capture period" }));
  const option = await screen.findByRole("option", { name: "Weekly · last 7 days" });
  fireEvent.pointerDown(option, { pointerType: "mouse" });
  fireEvent.click(option);
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "Capture period" }).textContent).toContain(
      "Weekly · last 7 days",
    ),
  );
  pane.scrollTop = 480;
  opener.focus();
  fireEvent.click(opener);
  reads[0]!.resolve({ output: stringifyJSON(exact(weekly)) });
  await screen.findByText("9,007,199,254,740,993");

  const refresh = screen.getByRole("button", { name: "Refresh report" });
  refresh.focus();
  fireEvent.click(refresh);
  expect(calls("get")).toHaveLength(2);
  const signal = calls("get")[1]![1]!.signal!;
  expect(signal.aborted).toBe(false);
  const back = screen.getByRole("button", { name: "Back to archive" });
  back.focus();
  fireEvent.click(back);
  expect(signal.aborted).toBe(true);
  expect(document.activeElement).toBe(opener);
  expect(pane.scrollTop).toBe(480);
  expect(screen.getByRole("combobox", { name: "Capture period" }).textContent).toContain(
    "Weekly · last 7 days",
  );

  await act(async () => {
    reads[1]!.resolve({ output: stringifyJSON(exact(weekly)) });
    await reads[1]!.promise;
  });
  expect(screen.queryByRole("heading", { name: /^(Daily|Weekly) report · / })).toBeNull();
  expect(document.activeElement).toBe(opener);
  fireEvent.click(screen.getByRole("button", { name: /^View report: Daily report · / }));
  reads[2]!.resolve({ output: stringifyJSON(saved(daily)) });
  await screen.findByText("Total cost (USD)");
  expect(screen.getByRole("heading", { name: /^Daily report · / })).toBeTruthy();
  expect(screen.queryByRole("heading", { name: /^Weekly report · / })).toBeNull();
  expect(screen.queryByText("9,007,199,254,740,993")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Back to archive" }));
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: /^View report: Daily report · / }),
  );
  expect(pane.scrollTop).toBe(480);
  expect(calls("get").map(([request]) => reportId(request))).toEqual([
    weekly.id,
    weekly.id,
    daily.id,
  ]);
});

it("keeps a failed archive read distinct from an empty archive and retains accepted rows", async () => {
  const outcomes: Array<() => Promise<CallPluginResponse>> = [
    () => Promise.reject(new Error("archive offline")),
    () => reply([]),
    () => reply([daily]),
    () => Promise.reject(new Error("archive offline again")),
  ];
  mount(() => outcomes.shift()!());
  expect((await screen.findByRole("alert")).textContent).toBe(
    "The archive could not be read: archive offline.",
  );
  expect(screen.queryByText("No saved reports")).toBeNull();

  const refresh = screen.getByRole("button", { name: "Refresh archive" });
  fireEvent.click(refresh);
  await screen.findByText("No saved reports");
  expect(screen.queryByRole("alert")).toBeNull();

  fireEvent.click(refresh);
  await screen.findByRole("button", { name: /^View report: Daily report · / });
  fireEvent.click(refresh);
  expect((await screen.findByRole("alert")).textContent).toBe(
    "The archive could not be read: archive offline again. The last accepted archive remains shown.",
  );
  expect(screen.getByRole("button", { name: /^View report: Daily report · / })).toBeTruthy();
  expect(screen.queryByText("No saved reports")).toBeNull();
});

it("removes only the confirmed captured report after a named decision", async () => {
  let members = [daily, weekly];
  const removal = deferred<CallPluginResponse>();
  const { calls } = mount((request) => {
    switch (request.method) {
      case "flame.usage-reports/list.v1":
        return reply(members);
      case "flame.usage-reports/get.v1":
        return reply(saved(reportId(request) === daily.id ? daily : weekly));
      default:
        return mutationResult<CallPluginResponse>(async () => {
          const result = await removal.promise;
          members = members.filter(({ id }) => id !== reportId(request));
          return result;
        }, "remove-daily");
    }
  });
  fireEvent.click(await screen.findByRole("button", { name: /^View report: Daily report · / }));
  await screen.findByText("Total cost (USD)");
  const remove = screen.getByRole("button", { name: "Remove saved report" });

  remove.focus();
  fireEvent.click(remove);
  let dialog = await screen.findByRole("alertdialog", { name: /^Remove Daily report · .*\?$/ });
  await waitFor(() =>
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Cancel" })),
  );
  fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(remove));

  fireEvent.click(remove);
  dialog = await screen.findByRole("alertdialog");
  fireEvent.keyDown(within(dialog).getByRole("button", { name: "Cancel" }), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  expect(calls("delete")).toHaveLength(0);

  fireEvent.click(remove);
  dialog = await screen.findByRole("alertdialog");
  fireEvent.click(within(dialog).getByRole("button", { name: "Remove saved report" }));
  const confirming = within(dialog).getByRole("button", { name: "Removing report…" });
  expect(confirming.getAttribute("aria-busy")).toBe("true");
  expect(confirming.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(confirming);
  fireEvent.keyDown(confirming, { key: "Escape" });
  expect(screen.getByRole("alertdialog")).toBe(dialog);
  expect(calls("delete").map(([request]) => reportId(request))).toEqual([daily.id]);
  removal.resolve({ output: stringifyJSON(true) });
  await screen.findByText("Saved report removed. Core accounting is unchanged.");
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  expect(calls("delete").map(([request]) => reportId(request))).toEqual([daily.id]);
  expect(screen.queryByRole("heading", { name: /^Daily report · / })).toBeNull();
  expect(screen.queryByRole("button", { name: /^View report: Daily report/ })).toBeNull();
  expect(screen.getByRole("button", { name: /^View report: Weekly report · / })).toBeTruthy();
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Refresh archive" })),
  );
  fireEvent.click(screen.getByRole("button", { name: /^View report: Weekly report · / }));
  await screen.findByRole("heading", { name: /^Weekly report · / });
  expect(screen.queryByText("Saved report removed. Core accounting is unchanged.")).toBeNull();
});

it.each([
  { name: "fresh membership keeps it", viewing: weekly, archiveReads: true, stays: true },
  { name: "an unread archive keeps it", viewing: weekly, archiveReads: false, stays: true },
  { name: "fresh membership retires it", viewing: daily, archiveReads: true, stays: false },
])(
  "reconciles a recovered old removal against the open report when $name",
  async ({ viewing, archiveReads, stays }) => {
    let members = [daily, weekly];
    let pending: PluginCommandRecovery[] = [];
    let listFails = false;
    const recover = vi.fn();
    const { calls } = mount(
      (request) => {
        switch (request.method) {
          case "flame.usage-reports/list.v1":
            return listFails ? Promise.reject(new Error("archive offline")) : reply(members);
          case "flame.usage-reports/get.v1":
            return reply(saved(reportId(request) === daily.id ? daily : weekly));
          default:
            return mutationResult<CallPluginResponse>(async () => {
              pending = [
                original("flame.usage-reports/delete.v1", async (decode) => {
                  recover();
                  pending = [];
                  members = members.filter(({ id }) => id !== daily.id);
                  return decode(true);
                }),
              ];
              throw new MutationLookupError(new Error("receipt unavailable"));
            }, "remove-daily");
        }
      },
      () => pending,
    );
    fireEvent.click(await screen.findByRole("button", { name: /^View report: Daily report · / }));
    await screen.findByText("Total cost (USD)");
    fireEvent.click(screen.getByRole("button", { name: "Remove saved report" }));
    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove saved report" }));
    await screen.findByText(/Confirmation is unknown/);
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(screen.getByRole("heading", { name: /^Daily report · / })).toBeTruthy();
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Remove saved report" }).disabled,
    ).toBe(true);

    if (viewing === weekly) {
      fireEvent.click(screen.getByRole("button", { name: "Back to archive" }));
      fireEvent.click(screen.getByRole("button", { name: /^View report: Weekly report · / }));
      await screen.findByRole("heading", { name: /^Weekly report · / });
    }
    listFails = !archiveReads;
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Check original outcome" }));
    });
    await screen.findByText("Original report removal confirmed. Core accounting is unchanged.");
    expect(recover).toHaveBeenCalledOnce();
    expect(calls("delete")).toHaveLength(1);
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Check original outcome" })).toBeNull(),
    );

    await waitFor(() =>
      expect(Boolean(screen.queryByRole("heading", { name: /^(Daily|Weekly) report · / }))).toBe(
        stays,
      ),
    );
    expect(Boolean(screen.queryByRole("heading", { name: /^Weekly report · / }))).toBe(stays);
    expect(screen.queryByRole("heading", { name: /^Daily report · / })).toBeNull();
    expect(Boolean(screen.queryByText("Total cost (USD)"))).toBe(stays);
    const back = screen.queryByRole("button", { name: "Back to archive" });
    if (back) fireEvent.click(back);
    expect(Boolean(screen.queryByRole("button", { name: /^View report: Daily report/ }))).toBe(
      !archiveReads,
    );
    expect(screen.queryByRole("alert")?.textContent ?? null).toBe(
      archiveReads
        ? null
        : "The archive could not be read: archive offline. The last accepted archive remains shown.",
    );
    expect(screen.queryByText("No saved reports")).toBeNull();
    expect(screen.getByRole("button", { name: /^View report: Weekly report · / })).toBeTruthy();
  },
);
