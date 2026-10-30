import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ComponentContext, TelemetrySnapshot, ClientTelemetry } from "@flame/component-sdk";
import { Diagnostics } from "../../../plugins/diagnostics/component/component";

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
  const context: ComponentContext = {
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
  return testClient.render(<Diagnostics context={context} />);
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

it("formats category counts for the active locale", () => {
  const snapshot = structuredClone(recorded);
  snapshot.logs = Array.from({ length: 1000 }, (_, index) => ({
    ...recorded.logs[0]!,
    id: String(index),
  }));
  openDiagnostics({ read: () => snapshot, clear() {} }, "de");
  expect(screen.getByRole("radio", { name: /1\.000/ })).toBeTruthy();
});

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
