import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
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
import { ExactNumber, stringifyJSON, type JSONNumber } from "@frame/sdk/json";
import {
  createFlameClient,
  MutationLookupError,
  type PluginCommandRecovery,
} from "@flame/runtime-client";

import { type CallPluginResponse } from "@flame/runtime-client/wire";
import { type ComponentContext } from "@flame/component-sdk";
import { UsageReports } from "../../../../plugins/usagereports/component/component";
import { LIMITS } from "../../../../plugins/usagereports/contracts/limits";
import {
  CAPABILITY_JSON,
  type MethodOutputs,
} from "../../../../plugins/usagereports/component/methods.generated";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import { VISUAL_MEMORY_BINDING, VISUAL_USAGE } from "./componentSnapshots";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { REVIEW_DIFF } from "./diffSnapshot";
type Header = MethodOutputs["flame.usage-reports/list.v2"]["reports"][number];

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
  locale = "en",
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
    component: { ...binding, componentId: "usage-reports" },
    binding,
    runtime,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale,
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose: () => null,
  };
  const view = testClient.render(<UsageReports context={context} />);
  const calls = (method: string) =>
    call.mock.calls.filter(([request]) => request.method === `flame.usage-reports/${method}.v1`);
  return {
    calls,
    changeLocale: (nextLocale: string) =>
      view.rerender(<UsageReports context={{ ...context, locale: nextLocale }} />),
  };
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
        request.method === "flame.usage-reports/get.v1"
          ? report
          : { reports: saved ? [header] : [] },
      ),
    }) as never;
  });
  const context: ComponentContext = {
    component: { ...binding, componentId: "usage-reports" },
    binding,
    runtime,
    telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
    locale: "en",
    signal: owner.signal,
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose: () => null,
  };
  testClient.render(<UsageReports context={context} />);
  await screen.findByText("No saved reports");
  fireEvent.click(screen.getByRole("button", { name: "Save report" }));
  await screen.findByText(/Confirmation is unknown/);
  const save = screen.getByRole("button", { name: "Save report" });
  expect(save.getAttribute("aria-disabled")).toBe("true");
  activateRepeatedly(save);
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
    request.method === "flame.usage-reports/list.v2" ? list.promise : read.promise,
  );
  expect(screen.getByText("Reading saved reports…")).toBeTruthy();
  expect(screen.queryByText("No saved reports")).toBeNull();
  list.resolve({ output: stringifyJSON({ reports: [daily, weekly] }) });

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
  expect(screen.getByText(`2 saved reports. Capacity is ${LIMITS.maxReports}.`)).toBeTruthy();
});

function heldReads() {
  const reads: Array<ReturnType<typeof deferred<CallPluginResponse>>> = [];
  const { calls, changeLocale } = mount((request) => {
    if (request.method === "flame.usage-reports/list.v2")
      return reply({ reports: [daily, weekly] });
    const read = deferred<CallPluginResponse>();
    reads.push(read);
    return read.promise;
  });
  return { reads, calls, changeLocale };
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

it.each(["initial read", "report refresh"])(
  "keeps the original %s and later focus through a locale-only change",
  async (phase) => {
    const { reads, calls, changeLocale } = heldReads();
    const opener = await screen.findByRole("button", { name: /^View report: Weekly report · / });
    act(() => opener.focus());
    fireEvent.click(opener);
    const heading = screen.getByRole("heading", { name: /^Weekly report · / });
    const refresh = screen.getByRole<HTMLButtonElement>("button", { name: "Refresh report" });
    let index = 0;
    if (phase === "report refresh") {
      reads[0]!.resolve({ output: stringifyJSON(exact(weekly)) });
      await screen.findByText("9,007,199,254,740,993");
      act(() => refresh.focus());
      fireEvent.click(refresh);
      index = 1;
    }
    const signal = calls("get")[index]![1]!.signal!;
    const back = screen.getByRole("button", { name: "Back to archive" });
    act(() => back.focus());
    act(() => changeLocale("zh"));

    expect(screen.getByRole("heading", { name: /^每周报告 · / })).toBe(heading);
    expect(screen.getByRole("button", { name: "返回归档" })).toBe(back);
    expect(document.activeElement).toBe(back);
    expect(signal.aborted).toBe(false);
    expect(calls("get")).toHaveLength(index + 1);
    expect(calls("capture")).toHaveLength(0);

    reads[index]!.resolve({ output: stringifyJSON(exact(weekly)) });
    await waitFor(() => expect(screen.queryByText("正在读取已保存的报告…")).toBeNull());
    expect(screen.getByText("已知输入标记")).toBeTruthy();
    expect(screen.getByText("9,007,199,254,740,993")).toBeTruthy();
    expect(document.activeElement).toBe(back);
    expect(screen.getByRole("button", { name: "刷新报告" })).toBe(refresh);
    expect(refresh.getAttribute("aria-disabled")).toBeNull();
    expect(refresh.disabled).toBe(false);
    expect(calls("get").map(([request]) => reportId(request))).toEqual(
      Array(index + 1).fill(weekly.id),
    );

    act(() => refresh.focus());
    fireEvent.click(refresh);
    expect(calls("get")).toHaveLength(index + 2);
    reads[index + 1]!.resolve({ output: stringifyJSON(exact(weekly)) });
    await waitFor(() => expect(screen.queryByText("正在读取已保存的报告…")).toBeNull());
    expect(document.activeElement).toBe(refresh);
    act(() => back.focus());
    fireEvent.click(back);
    expect(screen.getByRole("button", { name: /^查看报告：每周报告 · / })).toBe(opener);
    expect(document.activeElement).toBe(opener);
    expect(calls("capture")).toHaveLength(0);
  },
);

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

const usageDay = (key: string, cost?: JSONNumber, calls: JSONNumber = 1) => ({
  key,
  total: { inputTokens: 1, outputTokens: 1, ...(cost === undefined ? {} : { costUsd: cost }) },
  ...(cost === undefined ? {} : { recordedCostUsd: cost }),
  coverage: {
    calls,
    reportedCalls: calls,
    pricedCalls: cost === undefined ? 0 : calls,
    startedCalls: 0,
    completedCalls: calls,
    failedCalls: 0,
    unknownCalls: 0,
  },
});

const withSummary = (header: Header, summary: Partial<Report["summary"]>): Report => {
  const report = saved(header);
  return { ...report, summary: { ...report.summary, ...summary } };
};

const facts = (term: string) =>
  screen
    .getAllByText(term, { selector: "dt" })
    .map((label) => label.nextElementSibling!.textContent);

it.each([
  {
    locale: "en",
    weeklyView: /^View report: Weekly report · /,
    dailyView: /^View report: Daily report · /,
    back: "Back to archive",
    breakdown: "Daily core breakdown",
    input: "Known input tokens",
    subtotal: "Known price subtotal (USD)",
    totalCost: "Total cost (USD)",
    notReported: "Not reported",
    tiny: "0.0000004",
    wide: "9,007,199,254,740,993",
    rows: [
      "1 attempt · 0.0000004 known USD",
      "1 attempt · 0 known USD",
      "1 attempt · 1.5 known USD",
      "1 attempt · 0.10000000000000000001 known USD",
    ],
  },
  {
    locale: "de",
    weeklyView: /^Bericht anzeigen: Bericht \(Wöchentlich\) · /,
    dailyView: /^Bericht anzeigen: Bericht \(Täglich\) · /,
    back: "Zurück zum Archiv",
    breakdown: "Tägliche Kernaufschlüsselung",
    input: "Bekannte Eingabe-Tokens",
    subtotal: "Zwischensumme bekannter Preise (USD)",
    totalCost: "Gesamtkosten (USD)",
    notReported: "Nicht gemeldet",
    tiny: "0,0000004",
    wide: "9.007.199.254.740.993",
    rows: [
      "1 Versuch · 0,0000004 USD bekannt",
      "1 Versuch · 0 USD bekannt",
      "1 Versuch · 1,5 USD bekannt",
      "1 Versuch · 0.10000000000000000001 USD bekannt",
    ],
  },
])(
  "shows a saved sub-micro charge beside zero, missing and exact values in $locale",
  async ({ locale, rows, ...label }) => {
    const reports: Record<string, Report> = {
      [weekly.id]: withSummary(weekly, {
        total: { inputTokens: 1200, outputTokens: 300, costUsd: 0.0000004 },
        recordedCostUsd: 0.0000004,
        byDay: [usageDay("2026-10-19", 0.0000004), usageDay("2026-10-20", 0)],
      }),
      [daily.id]: withSummary(daily, {
        total: { inputTokens: 9007199254740993n, outputTokens: 0 },
        recordedCostUsd: new ExactNumber("1.60000000000000000001"),
        byDay: [
          usageDay("2026-10-19", 1.5),
          usageDay("2026-10-20", new ExactNumber("0.10000000000000000001")),
        ],
      }),
    };
    const sent = stringifyJSON(reports);
    const { calls } = mount(
      (request) =>
        request.method === "flame.usage-reports/list.v2"
          ? reply({ reports: [daily, weekly] })
          : reply(reports[reportId(request)]),
      undefined,
      locale,
    );

    fireEvent.click(await screen.findByRole("button", { name: label.weeklyView }));
    await screen.findByText(label.totalCost);
    expect(facts(label.subtotal)).toEqual([label.tiny]);
    expect(facts(label.totalCost)).toEqual([label.tiny]);
    fireEvent.click(screen.getByRole("button", { name: label.breakdown }));
    expect(await screen.findByText(rows[0]!)).toBeTruthy();
    expect(screen.getByText(rows[1]!)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: label.back }));
    fireEvent.click(screen.getByRole("button", { name: label.dailyView }));
    await screen.findByText(label.wide);
    expect(facts(label.input)).toEqual([label.wide]);
    expect(facts(label.subtotal)).toEqual(["1.60000000000000000001"]);
    expect(facts(label.totalCost)).toEqual([label.notReported]);
    fireEvent.click(screen.getByRole("button", { name: label.breakdown }));
    expect(await screen.findByText(rows[2]!)).toBeTruthy();
    expect(screen.getByText(rows[3]!)).toBeTruthy();

    expect(stringifyJSON(reports)).toBe(sent);
    expect(calls("get").map(([request]) => reportId(request))).toEqual([weekly.id, daily.id]);
    expect(calls("capture")).toHaveLength(0);
    expect(calls("delete")).toHaveLength(0);
  },
);

it.each([
  {
    locale: "en",
    view: /^View report: Weekly report · /,
    breakdown: "Daily core breakdown",
    rows: [
      "0 attempts · 0 known USD",
      "1 attempt · 1.5 known USD",
      "2 attempts · 0.25 known USD",
      "1,000 attempts · 3 known USD",
      "1,000,000 attempts · 4 known USD",
      "9,007,199,254,740,993 attempts · 0.10000000000000000001 known USD",
    ],
  },
  {
    locale: "de",
    view: /^Bericht anzeigen: Bericht \(Wöchentlich\) · /,
    breakdown: "Tägliche Kernaufschlüsselung",
    rows: [
      "0 Versuche · 0 USD bekannt",
      "1 Versuch · 1,5 USD bekannt",
      "2 Versuche · 0,25 USD bekannt",
      "1.000 Versuche · 3 USD bekannt",
      "1.000.000 Versuche · 4 USD bekannt",
      "9.007.199.254.740.993 Versuche · 0.10000000000000000001 USD bekannt",
    ],
  },
  {
    locale: "fr",
    view: /^Voir le rapport\s:\sRapport Hebdomadaire · /,
    breakdown: "Détail principal quotidien",
    rows: [
      "0 tentative · coût connu 0 USD",
      "1 tentative · coût connu 1,5 USD",
      "2 tentatives · coût connu 0,25 USD",
      "1 000 tentatives · coût connu 3 USD",
      "1 000 000 tentatives · coût connu 4 USD",
      "9 007 199 254 740 993 tentatives · coût connu 0.10000000000000000001 USD",
    ],
  },
  {
    locale: "es",
    view: /^Ver informe: Informe Semanal · /,
    breakdown: "Desglose base diario",
    rows: [
      "0 intentos · coste conocido 0 USD",
      "1 intento · coste conocido 1,5 USD",
      "2 intentos · coste conocido 0,25 USD",
      "1000 intentos · coste conocido 3 USD",
      "1.000.000 intentos · coste conocido 4 USD",
      "9.007.199.254.740.993 intentos · coste conocido 0.10000000000000000001 USD",
    ],
  },
])(
  "selects $locale daily attempt copy while showing each saved count exactly",
  async ({ locale, view, breakdown, rows }) => {
    const report = withSummary(weekly, {
      byDay: [
        usageDay("2026-10-13", 0, 0),
        usageDay("2026-10-14", 1.5, 1),
        usageDay("2026-10-15", 0.25, 2),
        usageDay("2026-10-16", 3, 1000),
        usageDay("2026-10-17", 4, 1000000),
        usageDay("2026-10-18", new ExactNumber("0.10000000000000000001"), 9007199254740993n),
      ],
    });
    const sent = stringifyJSON(report);
    const { calls } = mount(
      (request) =>
        request.method === "flame.usage-reports/list.v2"
          ? reply({ reports: [weekly] })
          : reply(report),
      undefined,
      locale,
    );

    fireEvent.click(await screen.findByRole("button", { name: view }));
    fireEvent.click(await screen.findByRole("button", { name: breakdown }));
    expect(await screen.findByText(rows[0]!)).toBeTruthy();
    for (const row of rows) expect(screen.getByText(row)).toBeTruthy();

    expect(stringifyJSON(report)).toBe(sent);
    expect(calls("get").map(([request]) => reportId(request))).toEqual([weekly.id]);
    expect(calls("capture")).toHaveLength(0);
    expect(calls("delete")).toHaveLength(0);
  },
);

it("reads a saved zero-attempt report without an empty daily breakdown", async () => {
  const report: Report = {
    ...weekly,
    summary: {
      observedAt: weekly.observedAt,
      since: weekly.since,
      total: {
        inputTokens: 0,
        outputTokens: 0,
        cacheReadTokens: 0,
        cacheWriteTokens: 0,
        reasoningTokens: 0,
      },
      coverage: {
        calls: 0,
        reportedCalls: 0,
        pricedCalls: 0,
        startedCalls: 0,
        completedCalls: 0,
        failedCalls: 0,
        unknownCalls: 0,
      },
      byProvider: [],
      byModel: [],
      byDay: [],
      byOrigin: [],
      byScope: [],
      sessions: 0,
      runs: 0,
    },
  };
  const sent = stringifyJSON(report);
  const { calls } = mount((request) =>
    request.method === "flame.usage-reports/list.v2"
      ? reply({ reports: [daily, weekly] })
      : reply(report),
  );

  const opener = await screen.findByRole("button", { name: /^View report: Weekly report · / });
  opener.focus();
  fireEvent.click(opener);
  await screen.findByText("Total cost (USD)");
  expect(facts("Model attempts")).toEqual(["0"]);
  expect(facts("Known input tokens")).toEqual(["0"]);
  expect(facts("Known output tokens")).toEqual(["0"]);
  expect(facts("Known price subtotal (USD)")).toEqual(["Not reported"]);
  expect(facts("Total cost (USD)")).toEqual(["Not reported"]);
  expect(facts("Usage recorded")).toEqual(["0/0"]);
  expect(facts("Priced attempts")).toEqual(["0/0"]);
  expect(facts("Outcome unknown")).toEqual(["0"]);
  expect(screen.queryByText("Daily core breakdown")).toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: "Exact report identity" }));
  expect(await screen.findByText(weekly.id)).toBeTruthy();
  expect(facts("Observed at (UTC)")).toEqual([weekly.observedAt]);
  expect(facts("Lower bound (UTC)")).toEqual([weekly.since]);
  expect(screen.getByRole("button", { name: "Remove saved report" })).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "Back to archive" }));
  expect(document.activeElement).toBe(opener);
  expect(stringifyJSON(report)).toBe(sent);
  expect(calls("get").map(([request]) => reportId(request))).toEqual([weekly.id]);
  expect(calls("capture")).toHaveLength(0);
  expect(calls("delete")).toHaveLength(0);
});

it("keeps a failed archive read distinct from an empty archive and retains accepted rows", async () => {
  const outcomes: Array<() => Promise<CallPluginResponse>> = [
    () => Promise.reject(new Error("archive offline")),
    () => reply({ reports: [] }),
    () => reply({ reports: [daily] }),
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
      case "flame.usage-reports/list.v2":
        return reply({ reports: members });
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
          case "flame.usage-reports/list.v2":
            return listFails
              ? Promise.reject(new Error("archive offline"))
              : reply({ reports: members });
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

it("keeps a held save focused and single, restores it after refusal and opens an accepted save", async () => {
  let members: Header[] = [];
  const captures: Array<ReturnType<typeof deferred<CallPluginResponse>>> = [];
  const { calls } = mount((request) => {
    switch (request.method) {
      case "flame.usage-reports/list.v2":
        return reply({ reports: members });
      case "flame.usage-reports/get.v1":
        return reply(exact(daily));
      default: {
        const capture = deferred<CallPluginResponse>();
        captures.push(capture);
        return mutationResult<CallPluginResponse>(
          () => capture.promise,
          `capture-${captures.length}`,
        );
      }
    }
  });
  await screen.findByText("No saved reports");
  const save = screen.getByRole("button", { name: "Save report" });
  const refresh = screen.getByRole("button", { name: "Refresh archive" });
  save.focus();
  fireEvent.click(save);
  expect(document.activeElement).toBe(save);
  expect(save.textContent).toBe("Saving report…");
  expect(save.getAttribute("aria-busy")).toBe("true");
  expect(save.getAttribute("aria-disabled")).toBe("true");
  expect(refresh.getAttribute("aria-disabled")).toBeNull();
  activateRepeatedly(save);
  expect(calls("capture")).toHaveLength(1);
  expect(document.activeElement).toBe(save);

  captures[0]!.reject(new Error("archive full"));
  await screen.findByText(/^The archive change failed: /);
  expect(document.activeElement).toBe(save);
  expect(save.textContent).toBe("Save report");
  expect(save.getAttribute("aria-busy")).toBe("false");
  expect(save.getAttribute("aria-disabled")).toBe("false");

  members = [daily];
  fireEvent.click(save);
  captures[1]!.resolve({ output: stringifyJSON(daily) });
  const heading = await screen.findByRole("heading", { name: /^Daily report · / });
  expect(document.activeElement).toBe(heading);
  await screen.findByText("9,007,199,254,740,993");
  expect(calls("capture")).toHaveLength(2);
  expect(calls("get").map(([request]) => reportId(request))).toEqual([daily.id]);

  fireEvent.click(screen.getByRole("button", { name: "Back to archive" }));
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Refresh archive" }));
  expect(screen.getByRole("button", { name: /^View report: Daily report · / })).toBeTruthy();
});

it("settles the original held capture after a locale-only change without claiming later focus", async () => {
  let members: Header[] = [];
  const capture = deferred<CallPluginResponse>();
  const { calls, changeLocale } = mount((request) => {
    switch (request.method) {
      case "flame.usage-reports/list.v2":
        return reply({ reports: members });
      case "flame.usage-reports/get.v1":
        return reply(exact(daily));
      default:
        return mutationResult<CallPluginResponse>(() => capture.promise, "locale-capture");
    }
  });
  await screen.findByText("No saved reports");
  const save = screen.getByRole<HTMLButtonElement>("button", { name: "Save report" });
  const refresh = screen.getByRole("button", { name: "Refresh archive" });
  act(() => save.focus());
  fireEvent.click(save);
  const signal = calls("capture")[0]![1]!.signal!;
  act(() => {
    refresh.focus();
    changeLocale("zh");
  });

  expect(screen.getByRole("button", { name: "正在保存报告…" })).toBe(save);
  expect(save.getAttribute("aria-busy")).toBe("true");
  expect(save.getAttribute("aria-disabled")).toBe("true");
  expect(screen.getByRole("button", { name: "刷新归档" })).toBe(refresh);
  expect(document.activeElement).toBe(refresh);
  expect(signal.aborted).toBe(false);
  expect(calls("capture")).toHaveLength(1);
  expect(calls("get")).toHaveLength(0);

  await act(async () => {
    members = [daily];
    capture.resolve({ output: stringifyJSON(daily) });
    await capture.promise;
  });
  await waitFor(() => expect(save.getAttribute("aria-busy")).toBe("false"));
  expect(screen.getByRole("button", { name: "保存报告" })).toBe(save);
  expect(save.getAttribute("aria-disabled")).toBe("false");
  expect(save.disabled).toBe(false);
  const title = `每日报告 · ${new Date(daily.observedAt).toLocaleString("zh")}`;
  expect(screen.getByRole("button", { name: `查看报告：${title}` })).toBeTruthy();
  expect(screen.getByRole("status").textContent).toBe(`核心报告已保存：${title}。它已列入归档。`);
  expect(screen.queryByRole("heading", { name: /^每日报告 · / })).toBeNull();
  expect(document.activeElement).toBe(refresh);
  expect(signal.aborted).toBe(false);
  expect(calls("capture")).toHaveLength(1);
  expect(JSON.parse(calls("capture")[0]![0].input)).toEqual({ period: "daily" });
  expect(calls("get")).toHaveLength(0);
});

it("updates settled archive feedback with the current locale and preserves the provider error", async () => {
  const raw = "Provider refused: quota_exceeded\n  request=E-17 · 保留  x2";
  const capture = deferred<CallPluginResponse>();
  const { calls, changeLocale } = mount((request) =>
    request.method === "flame.usage-reports/list.v2"
      ? reply({ reports: [] })
      : mutationResult<CallPluginResponse>(() => capture.promise, "locale-refusal"),
  );
  await screen.findByText("No saved reports");
  const save = screen.getByRole("button", { name: "Save report" });
  const refresh = screen.getByRole("button", { name: "Refresh archive" });
  act(() => save.focus());
  fireEvent.click(save);
  act(() => refresh.focus());
  await act(async () => capture.reject(new Error(raw)));
  expect(screen.getByRole("status").textContent).toBe(`The archive change failed: ${raw}`);

  act(() => changeLocale("zh"));
  expect(screen.getByRole("status").textContent).toBe(`归档更改失败：${raw}`);
  expect(screen.getByRole("button", { name: "保存报告" })).toBe(save);
  expect(save.getAttribute("aria-busy")).toBe("false");
  expect(save.getAttribute("aria-disabled")).toBe("false");
  expect(document.activeElement).toBe(refresh);
  act(() => changeLocale("en"));
  expect(screen.getByRole("status").textContent).toBe(`The archive change failed: ${raw}`);
  expect(document.activeElement).toBe(refresh);
  expect(calls("capture")).toHaveLength(1);
  expect(calls("get")).toHaveLength(0);
});

function unsettledCapture() {
  let pending: PluginCommandRecovery[] = [];
  let members: Header[] = [];
  const recoveries: Array<ReturnType<typeof deferred<void>>> = [];
  const retirements: Array<ReturnType<typeof deferred<void>>> = [];
  const command = {
    ...original("flame.usage-reports/capture.v1", async (decode) => {
      const recovery = deferred<void>();
      recoveries.push(recovery);
      await recovery.promise;
      pending = [];
      members = [daily];
      return decode(daily);
    }),
    retire: vi.fn(async () => {
      const retirement = deferred<void>();
      retirements.push(retirement);
      await retirement.promise;
      pending = [];
    }),
  };
  const { calls } = mount(
    (request) => {
      switch (request.method) {
        case "flame.usage-reports/list.v2":
          return reply({ reports: members });
        case "flame.usage-reports/get.v1":
          return reply(exact(daily));
        default:
          return mutationResult<CallPluginResponse>(async () => {
            pending = [command];
            throw new MutationLookupError(new Error("receipt unavailable"));
          }, "original-capture");
      }
    },
    () => pending,
  );
  return { calls, command, recoveries, retirements };
}

async function saveUnknown() {
  fireEvent.click(await screen.findByRole("button", { name: "Save report" }));
  await screen.findByText(/Confirmation is unknown/);
}

it("keeps one focused original check busy and opens its accepted report", async () => {
  const { calls, command, recoveries } = unsettledCapture();
  await screen.findByText("No saved reports");
  await saveUnknown();
  const save = screen.getByRole("button", { name: "Save report" });
  const check = screen.getByRole("button", { name: "Check original outcome" });
  const retire = screen.getByRole<HTMLButtonElement>("button", {
    name: "Retire original command",
  });
  check.focus();
  fireEvent.click(check);
  expect(document.activeElement).toBe(check);
  expect(check.textContent).toBe("Checking original command…");
  expect(check.getAttribute("aria-busy")).toBe("true");
  expect(check.getAttribute("aria-disabled")).toBe("true");
  expect(retire.textContent).toBe("Retire original command");
  expect(retire.getAttribute("aria-busy")).toBe("false");
  expect(retire.disabled).toBe(true);
  expect(save.textContent).toBe("Save report");
  expect(save.getAttribute("aria-busy")).toBe("false");
  expect(save.getAttribute("aria-disabled")).toBe("true");
  activateRepeatedly(check);
  activateRepeatedly(save);
  fireEvent.click(retire);
  expect(recoveries).toHaveLength(1);
  expect(command.retire).not.toHaveBeenCalled();
  expect(calls("capture")).toHaveLength(1);

  recoveries[0]!.resolve();
  const heading = await screen.findByRole("heading", { name: /^Daily report · / });
  await waitFor(() => expect(document.activeElement).toBe(heading));
  await screen.findByText("9,007,199,254,740,993");
  expect(screen.queryByRole("button", { name: "Check original outcome" })).toBeNull();
  expect(calls("capture")).toHaveLength(1);
  expect(calls("get").map(([request]) => reportId(request))).toEqual([daily.id]);
});

it("lists an accepted original check without opening it after focus moves", async () => {
  const { recoveries } = unsettledCapture();
  await screen.findByText("No saved reports");
  await saveUnknown();
  const check = screen.getByRole("button", { name: "Check original outcome" });
  check.focus();
  fireEvent.click(check);
  const refresh = screen.getByRole("button", { name: "Refresh archive" });
  refresh.focus();
  recoveries[0]!.resolve();
  await screen.findByText(/^Core report saved: Daily report · .+\. It is listed in the archive\.$/);
  await screen.findByRole("button", { name: /^View report: Daily report · / });
  expect(screen.queryByRole("heading", { name: /^Daily report · / })).toBeNull();
  expect(screen.queryByRole("button", { name: "Check original outcome" })).toBeNull();
  expect(document.activeElement).toBe(refresh);
});

it("keeps a failed retirement focused and returns a confirmed one only while it owns focus", async () => {
  const { calls, command, retirements } = unsettledCapture();
  await screen.findByText("No saved reports");
  await saveUnknown();
  const check = screen.getByRole<HTMLButtonElement>("button", {
    name: "Check original outcome",
  });
  const retire = screen.getByRole("button", { name: "Retire original command" });
  retire.focus();
  fireEvent.click(retire);
  expect(document.activeElement).toBe(retire);
  expect(retire.textContent).toBe("Retiring original command…");
  expect(retire.getAttribute("aria-busy")).toBe("true");
  expect(retire.getAttribute("aria-disabled")).toBe("true");
  expect(check.textContent).toBe("Check original outcome");
  expect(check.getAttribute("aria-busy")).toBe("false");
  expect(check.disabled).toBe(true);
  activateRepeatedly(retire);
  fireEvent.click(check);
  expect(command.retire).toHaveBeenCalledOnce();

  retirements[0]!.reject(new Error("journal offline"));
  await screen.findByText(/^The original command could not be retired: /);
  expect(document.activeElement).toBe(retire);
  expect(retire.textContent).toBe("Retire original command");
  expect(retire.getAttribute("aria-disabled")).toBeNull();
  const save = screen.getByRole("button", { name: "Save report" });
  expect(save.getAttribute("aria-disabled")).toBe("true");

  fireEvent.click(retire);
  retirements[1]!.resolve();
  await screen.findByText(
    "Original command retired. Any report already saved remains in the archive.",
  );
  await waitFor(() => expect(document.activeElement).toBe(save));
  expect(save.getAttribute("aria-disabled")).toBe("false");
  expect(screen.queryByRole("button", { name: "Retire original command" })).toBeNull();

  await saveUnknown();
  const again = screen.getByRole("button", { name: "Retire original command" });
  again.focus();
  fireEvent.click(again);
  const refresh = screen.getByRole("button", { name: "Refresh archive" });
  refresh.focus();
  retirements[2]!.resolve();
  await screen.findByText(
    "Original command retired. Any report already saved remains in the archive.",
  );
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Retire original command" })).toBeNull(),
  );
  expect(document.activeElement).toBe(refresh);
  expect(command.retire).toHaveBeenCalledTimes(3);
  expect(calls("capture")).toHaveLength(2);
});
