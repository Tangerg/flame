import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
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
import type {
  ComponentContext,
  TelemetrySnapshot,
  ClientTelemetry,
  SpanRow,
  MetricRow,
  LogRow,
} from "@flame/component-sdk";
import { Diagnostics } from "../../../../plugins/diagnostics/component/component";

afterEach(cleanup);

const recorded: TelemetrySnapshot = {
  spans: [
    {
      id: "span",
      traceId: "trace",
      name: "workspace.preview",
      kind: "internal",
      startMs: 1785484800000,
      durationMillis: 21.5,
      status: "ok",
      attrs: {},
    },
  ],
  metrics: [
    {
      id: "latency",
      name: "workspace.preview.latency",
      unit: "ms",
      description: "Preview decoding duration",
      kind: "histogram",
      attrs: {},
      count: 12,
      sum: 840,
      p50: 50,
      p95: 100,
      avg: 70,
    },
    {
      id: "opens",
      name: "workspace.preview.opens",
      unit: "{preview}",
      description: "Accepted preview opens",
      kind: "counter",
      attrs: {},
      count: 24,
      sum: 24,
    },
  ],
  logs: [
    {
      id: "log",
      timeMs: 1785484801000,
      severity: "ERROR",
      body: "\nPreview could not be refreshed.\n\nError: file changed.\n    at refresh (preview.ts:89:7)\n",
      attrs: { source: "preview" },
    },
  ],
};

function openDiagnostics(
  telemetry: ClientTelemetry = { read: () => structuredClone(recorded), clear() {} },
  locale = "en",
) {
  let context: ComponentContext = {
    component: {
      installationId: "diagnostics-reading",
      digest: "a".repeat(64),
      componentId: "diagnostics",
    },
    locale,
    signal: new AbortController().signal,
    telemetry,
    get runtime(): never {
      throw Error("Diagnostics must not request Runtime data");
    },
    track() {
      throw Error("Diagnostics must not start Runtime I/O");
    },
    saveFile() {
      throw new Error("This captured read must not save files");
    },
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose() {
      throw Error("Diagnostics must not compose another component");
    },
  };
  const rendered = testClient.render(<Diagnostics context={context} />);
  function changeContext({
    locale = context.locale,
    telemetry = context.telemetry,
    signal = context.signal,
  }: Partial<Pick<ComponentContext, "locale" | "telemetry" | "signal">>) {
    // Preserve the forbidden Runtime getter without invoking it during a display update.
    context = Object.freeze(
      Object.create(Object.getPrototypeOf(context), {
        ...Object.getOwnPropertyDescriptors(context),
        locale: { value: locale, enumerable: true },
        telemetry: { value: telemetry, enumerable: true },
        signal: { value: signal, enumerable: true },
      }),
    ) as ComponentContext;
    rendered.rerender(<Diagnostics context={context} />);
  }
  return { ...rendered, context, changeContext };
}

function signal(name: string) {
  return screen.getByRole("radio", { name: new RegExp("^" + name + "(?: ·|$)") });
}
function observations() {
  return screen.getByRole("region", { name: "Local observations" });
}
function openRecord(name: RegExp) {
  const button = within(observations()).getByRole("button", { name });
  fireEvent.click(button);
  return button;
}
function fact(label: string) {
  const term = within(observations()).getByText(label, { exact: true });
  return term.parentElement!.querySelector("dd")!.textContent;
}
function clearObservations() {
  fireEvent.click(screen.getByRole("button", { name: "Diagnostic actions" }));
  fireEvent.click(screen.getByRole("menuitem", { name: "Clear local observations" }));
}

it("places read counts in their signal controls without a second count dashboard", () => {
  const read = vi.fn(() => structuredClone(recorded));
  openDiagnostics({ read, clear() {} });
  expect(screen.getByRole("radio", { name: "Traces · 1" }).getAttribute("aria-checked")).toBe(
    "true",
  );
  expect(screen.getByRole("radio", { name: "Metrics · 2" })).toBeTruthy();
  expect(screen.getByRole("radio", { name: "Logs · 1" })).toBeTruthy();
  expect(screen.queryByRole("definition")).toBeNull();
  expect(screen.queryByRole("heading", { name: "Local observations" })).toBeNull();
  fireEvent.click(signal("Logs"));
  expect(read).toHaveBeenCalledTimes(1);
  expect(
    within(observations()).getByRole("button", { name: /Preview could not be refreshed/ }),
  ).toBeTruthy();
});

it("keeps histogram sample count dimensionless and its measured values in their unit", () => {
  openDiagnostics();
  fireEvent.click(signal("Metrics"));
  openRecord(/workspace.preview.latency/);
  expect(fact("Count")).toBe("12");
  expect(fact("Sum")).toBe("840 ms");
  expect(fact("p50")).toBe("50 ms");
  expect(fact("p95")).toBe("100 ms");
  expect(fact("Average")).toBe("70 ms");
});

it("preserves dimensional counter values and complete raw metric evidence", () => {
  openDiagnostics();
  fireEvent.click(signal("Metrics"));
  openRecord(/workspace.preview.opens/);
  expect(fact("Count")).toBe("24 {preview}");
  expect(fact("Sum")).toBe("24 {preview}");
  fireEvent.click(within(observations()).getByRole("button", { name: "Raw observation" }));
  expect(
    within(observations()).getByRole("region", { name: "Local observation details" }).textContent,
  ).toContain('"unit": "{preview}"');
});

it("retains accepted signal counts and expanded evidence on failure, then refreshes that signal", () => {
  const later = structuredClone(recorded);
  later.logs.push({ ...later.logs[0]!, id: "next", body: "Next accepted log" });
  const read = vi
    .fn()
    .mockReturnValueOnce(structuredClone(recorded))
    .mockImplementationOnce(() => {
      throw Error("Buffer read failed. Retry Refresh.");
    })
    .mockReturnValueOnce(later);
  openDiagnostics({ read, clear() {} });
  fireEvent.click(signal("Logs"));
  const record = openRecord(/Preview could not be refreshed/);
  const refresh = screen.getByRole("button", { name: "Refresh" });
  refresh.focus();
  fireEvent.click(refresh);
  expect(screen.getByRole("alert").textContent).toContain("Buffer read failed");
  expect(screen.getByRole("radio", { name: "Logs · 1" }).getAttribute("aria-checked")).toBe("true");
  expect(screen.getByRole("status").textContent).toBe("Showing the last accepted read.");
  expect(
    within(observations()).getByRole("button", { name: /Preview could not be refreshed/ }),
  ).toBe(record);
  expect(record.getAttribute("aria-expanded")).toBe("true");
  expect(document.activeElement).toBe(refresh);
  fireEvent.click(refresh);
  expect(read).toHaveBeenCalledTimes(3);
  expect(screen.getByRole("radio", { name: "Logs · 2" }).getAttribute("aria-checked")).toBe("true");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.getByRole("status").textContent).toBe("Last buffer read");
  expect(
    within(observations()).getByRole("button", { name: /Preview could not be refreshed/ }),
  ).toBe(record);
});

it("does not present unknown counts as zero after an initial read failure", () => {
  openDiagnostics({
    read() {
      throw Error("Read unavailable. Retry Refresh.");
    },
    clear() {},
  });
  expect(screen.getByRole("alert")).toBeTruthy();
  expect(screen.getAllByRole("radio").map((tab) => tab.textContent)).toEqual([
    "Traces",
    "Metrics",
    "Logs",
  ]);
  expect(screen.queryByRole("region", { name: "Local observations" })).toBeNull();
});

it("discards a cleared snapshot before a failed reread and shows known zero counts only after recovery", () => {
  const empty = { spans: [], metrics: [], logs: [] };
  const read = vi
    .fn()
    .mockReturnValueOnce(structuredClone(recorded))
    .mockImplementationOnce(() => {
      throw Error("Reread unavailable. Retry Refresh.");
    })
    .mockReturnValueOnce(empty);
  const clear = vi.fn();
  openDiagnostics({ read, clear });
  fireEvent.click(signal("Logs"));
  clearObservations();
  expect(clear).toHaveBeenCalledTimes(1);
  expect(read).toHaveBeenCalledTimes(2);
  expect(screen.getAllByRole("radio").map((tab) => tab.textContent)).toEqual([
    "Traces",
    "Metrics",
    "Logs",
  ]);
  expect(screen.queryByRole("region", { name: "Local observations" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  expect(screen.getByRole("radio", { name: "Traces · 0" }).getAttribute("aria-checked")).toBe(
    "true",
  );
  expect(screen.getByRole("radio", { name: "Metrics · 0" })).toBeTruthy();
  expect(screen.getByRole("radio", { name: "Logs · 0" })).toBeTruthy();
  expect(screen.getByText("No local observations in this read.")).toBeTruthy();
});

it("preserves selected accepted material when clearing itself fails", () => {
  const clear = vi.fn(() => {
    throw Error("Clear unavailable. Try again.");
  });
  const read = vi.fn(() => structuredClone(recorded));
  openDiagnostics({ read, clear });
  fireEvent.click(signal("Logs"));
  clearObservations();
  expect(clear).toHaveBeenCalledTimes(1);
  expect(read).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("radio", { name: "Logs · 1" }).getAttribute("aria-checked")).toBe("true");
  expect(screen.getByRole("alert").textContent).toContain("Clear unavailable");
  expect(
    within(observations()).getByRole("button", { name: /Preview could not be refreshed/ }),
  ).toBeTruthy();
});

it("retains selected expanded evidence across display-locale changes until explicit Refresh", () => {
  const later = structuredClone(recorded);
  later.logs[0]!.body = "Updated log accepted by explicit Refresh.";
  later.logs.push({ ...later.logs[0]!, id: "next", body: "Another accepted log." });
  const read = vi.fn().mockReturnValueOnce(structuredClone(recorded)).mockReturnValue(later);
  const clear = vi.fn();
  const { context, changeContext } = openDiagnostics({ read, clear });
  fireEvent.click(signal("Logs"));
  const record = openRecord(/Preview could not be refreshed/);
  const raw = within(observations()).getByRole("button", { name: "Raw observation" });
  fireEvent.click(raw);
  const evidence = within(observations()).getByRole("region", {
    name: "Local observation details",
  });
  const laterFocus = screen.getByRole("button", { name: "About client diagnostics" });
  act(() => laterFocus.focus());
  act(() => changeContext({ locale: "zh" }));
  expect(read).toHaveBeenCalledTimes(1);
  expect(clear).not.toHaveBeenCalled();
  expect(context.signal.aborted).toBe(false);
  expect(signal("日志").getAttribute("aria-checked")).toBe("true");
  expect(signal("日志").textContent).toBe("日志 · 1");
  const records = within(screen.getByRole("region", { name: "本地观测记录" }));
  expect(records.getByRole("button", { name: /Preview could not be refreshed/ })).toBe(record);
  expect(record.getAttribute("aria-expanded")).toBe("true");
  expect(records.getByRole("button", { name: "原始观测记录" })).toBe(raw);
  expect(raw.getAttribute("aria-expanded")).toBe("true");
  expect(records.getByRole("region", { name: "本地观测记录详情" })).toBe(evidence);
  expect(evidence.textContent).toContain(JSON.stringify(recorded.logs[0], null, 2));
  expect(
    records.getByText(
      (_, node) => node?.tagName === "P" && node.textContent === recorded.logs[0]!.body,
    ),
  ).toBeTruthy();
  expect(document.activeElement).toBe(laterFocus);
  const refresh = screen.getByRole("button", { name: "刷新" });
  act(() => refresh.focus());
  fireEvent.click(refresh);
  expect(read).toHaveBeenCalledTimes(2);
  expect(clear).not.toHaveBeenCalled();
  expect(signal("日志").textContent).toBe("日志 · 2");
  expect(signal("日志").getAttribute("aria-checked")).toBe("true");
  expect(records.getByRole("button", { name: /Updated log accepted by explicit Refresh/ })).toBe(
    record,
  );
  expect(evidence.textContent).toContain(JSON.stringify(later.logs[0], null, 2));
  expect(screen.getByRole("status").textContent).toBe("最近一次读取");
  expect(document.activeElement).toBe(refresh);
});

const diagnosticFailure = "  Local buffer unavailable.\n原始原因 · 保留  ";

it.each([
  {
    operation: "read",
    failure: "literal",
    cause: new Error(diagnosticFailure),
    english: diagnosticFailure,
    chinese: diagnosticFailure,
    reads: 2,
    clears: 0,
  },
  {
    operation: "clear",
    failure: "literal",
    cause: new Error(diagnosticFailure),
    english: diagnosticFailure,
    chinese: diagnosticFailure,
    reads: 1,
    clears: 1,
  },
  {
    operation: "read",
    failure: "fallback",
    cause: undefined,
    english: "Local observations could not be read.",
    chinese: "无法读取本地观测记录。",
    reads: 2,
    clears: 0,
  },
  {
    operation: "clear",
    failure: "fallback",
    cause: undefined,
    english: "Local observations could not be cleared.",
    chinese: "无法清除本地观测记录。",
    reads: 1,
    clears: 1,
  },
])(
  "retains a $failure $operation failure and accepted evidence across display-locale changes",
  async (copy) => {
    const read = vi.fn(() => structuredClone(recorded));
    const clear = vi.fn<() => void>();
    const { changeContext } = openDiagnostics({ read, clear });
    fireEvent.click(signal("Logs"));
    const record = openRecord(/Preview could not be refreshed/);
    const operation = copy.operation === "read" ? read : clear;
    operation.mockImplementationOnce(() => {
      throw copy.cause;
    });
    const invoke =
      copy.operation === "read"
        ? () => fireEvent.click(screen.getByRole("button", { name: "Refresh" }))
        : clearObservations;
    invoke();
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(screen.getByRole("alert").textContent).toBe(copy.english);
    const laterFocus = screen.getByRole("button", { name: "About client diagnostics" });
    act(() => laterFocus.focus());
    act(() => changeContext({ locale: "zh" }));
    expect(read).toHaveBeenCalledTimes(copy.reads);
    expect(clear).toHaveBeenCalledTimes(copy.clears);
    expect(screen.getByRole("alert").textContent).toBe(copy.chinese);
    expect(screen.getByRole("status").textContent).toBe("当前显示上次成功读取的内容。");
    expect(signal("日志").textContent).toBe("日志 · 1");
    expect(signal("日志").getAttribute("aria-checked")).toBe("true");
    expect(
      within(screen.getByRole("region", { name: "本地观测记录" })).getByRole("button", {
        name: /Preview could not be refreshed/,
      }),
    ).toBe(record);
    expect(record.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(laterFocus);
    act(() => changeContext({ locale: "en" }));
    expect(read).toHaveBeenCalledTimes(copy.reads);
    expect(clear).toHaveBeenCalledTimes(copy.clears);
    expect(screen.getByRole("alert").textContent).toBe(copy.english);
    expect(screen.getByRole("status").textContent).toBe("Showing the last accepted read.");
    expect(document.activeElement).toBe(laterFocus);
  },
);

it("reads genuine telemetry and signal replacements while retiring an already-open Clear action", async () => {
  const read = vi.fn(() => structuredClone(recorded));
  const clear = vi.fn();
  const { changeContext } = openDiagnostics({ read, clear });
  fireEvent.click(signal("Logs"));
  const replacement = structuredClone(recorded);
  replacement.logs = [{ ...replacement.logs[0]!, id: "replacement", body: "New telemetry owner." }];
  const renewed = structuredClone(replacement);
  renewed.logs.push({ ...renewed.logs[0]!, id: "renewed", body: "New signal accepted read." });
  const replacementRead = vi.fn().mockReturnValueOnce(replacement).mockReturnValue(renewed);
  const replacementClear = vi.fn();
  act(() => changeContext({ telemetry: { read: replacementRead, clear: replacementClear } }));
  expect(read).toHaveBeenCalledTimes(1);
  expect(clear).not.toHaveBeenCalled();
  expect(replacementRead).toHaveBeenCalledTimes(1);
  expect(signal("Logs").getAttribute("aria-checked")).toBe("true");
  expect(within(observations()).getByRole("button", { name: /New telemetry owner/ })).toBeTruthy();
  const owner = new AbortController();
  act(() => changeContext({ signal: owner.signal }));
  expect(replacementRead).toHaveBeenCalledTimes(2);
  expect(signal("Logs").textContent).toBe("Logs · 2");
  expect(
    within(observations()).getByRole("button", { name: /New signal accepted read/ }),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Diagnostic actions" }));
  const clearAction = screen.getByRole("menuitem", { name: "Clear local observations" });
  act(() => owner.abort(new Error("Diagnostics owner retired.")));
  fireEvent.click(clearAction);
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  expect(replacementClear).not.toHaveBeenCalled();
  expect(replacementRead).toHaveBeenCalledTimes(2);
  expect(screen.getByRole("alert").textContent).toBe("Diagnostics owner retired.");
  expect(signal("Logs").textContent).toBe("Logs · 2");
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  fireEvent.click(screen.getByRole("button", { name: "Diagnostic actions" }));
  expect(screen.queryByRole("menu")).toBeNull();
  expect(replacementRead).toHaveBeenCalledTimes(2);
  expect(replacementClear).not.toHaveBeenCalled();
});

it("formats category counts for the active locale", () => {
  const snapshot = structuredClone(recorded);
  snapshot.logs = Array.from({ length: 1000 }, (_, index) => ({
    ...recorded.logs[0]!,
    id: String(index),
  }));
  openDiagnostics({ read: () => snapshot, clear() {} }, "de");
  expect(screen.getByRole("radio", { name: /1\.000/ })).toBeTruthy();
});

const vocabulary: TelemetrySnapshot = {
  spans: (["unset", "ok", "error"] as const).map((status, index) => ({
    id: status,
    traceId: "trace",
    name: `workspace.step.${index}`,
    kind: "provider.batch",
    startMs: 1785484800000,
    durationMillis: 3,
    status,
    attrs: {},
  })),
  metrics: recorded.metrics,
  logs: [
    {
      id: "notice",
      timeMs: 1785484802000,
      severity: "PROVIDER_NOTICE",
      body: "\n  Provider notice: cache warmed.\n\n    detail: 3 entries\n",
      attrs: {},
    },
  ],
};

it.each([
  {
    locale: "en",
    region: "Local observations",
    raw: "Raw observation",
    details: "Local observation details",
    metrics: "Metrics",
    logs: "Logs",
    badges: {
      unset: "Not set",
      ok: "OK",
      error: "Error",
      histogram: "Histogram",
      counter: "Counter",
    },
  },
  {
    locale: "zh",
    region: "本地观测记录",
    raw: "原始观测记录",
    details: "本地观测记录详情",
    metrics: "指标",
    logs: "日志",
    badges: { unset: "未设置", ok: "正常", error: "错误", histogram: "直方图", counter: "计数器" },
  },
])(
  "names closed span status and metric kind in $locale while preserving original evidence",
  ({ locale, region, raw, details, metrics, logs, badges }) => {
    const snapshot = structuredClone(vocabulary);
    openDiagnostics({ read: () => snapshot, clear() {} }, locale);
    function expectRecord(row: SpanRow | MetricRow | LogRow, title: string, badge: string) {
      const records = within(screen.getByRole("region", { name: region }));
      const button = records.getByRole("button", {
        name: (name) => name.includes(title) && name.includes(badge),
      });
      expect(within(button).getByText(badge, { exact: true })).toBeTruthy();
      fireEvent.click(button);
      fireEvent.click(records.getByRole("button", { name: raw }));
      expect(records.getByRole("region", { name: details }).textContent).toContain(
        JSON.stringify(row, null, 2),
      );
      return button;
    }

    for (const span of vocabulary.spans) {
      const button = expectRecord(span, span.name, badges[span.status]);
      expect(button.textContent).toContain("provider.batch");
      fireEvent.click(button);
    }
    fireEvent.click(signal(metrics));
    for (const metric of vocabulary.metrics) {
      fireEvent.click(expectRecord(metric, metric.name, badges[metric.kind]));
    }
    fireEvent.click(signal(logs));
    const log = vocabulary.logs[0]!;
    expectRecord(log, "Provider notice: cache warmed.", "PROVIDER_NOTICE");
    expect(
      within(screen.getByRole("region", { name: region })).getByText(
        (_, node) => node?.tagName === "P" && node.textContent === log.body,
      ),
    ).toBeTruthy();
    expect(snapshot).toEqual(vocabulary);
  },
);

it("preserves the complete multiline log and its original raw evidence", () => {
  openDiagnostics();
  fireEvent.click(signal("Logs"));
  openRecord(/Preview could not be refreshed/);
  expect(
    within(observations()).getByText(
      (_, node) => node?.tagName === "P" && node.textContent === recorded.logs[0]!.body,
    ),
  ).toBeTruthy();
  fireEvent.click(within(observations()).getByRole("button", { name: "Raw observation" }));
  expect(
    within(observations()).getByRole("region", { name: "Local observation details" }).textContent,
  ).toContain(JSON.stringify(recorded.logs[0]!.body));
});
