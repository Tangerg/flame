import { afterEach, expect, it } from "vitest";
import type { LogRow } from "@flame/plugin-sdk";
import { captureClientTelemetry } from "./clientTelemetry";
import { useTelemetryStore } from "./stores";

afterEach(() => useTelemetryStore.getState().clear());

it("reads detached current-buffer rows without the retired HTML transport cutoffs", () => {
  const logs: LogRow[] = Array.from({ length: 300 }, (_, index) => ({
    id: String(index),
    body: String(index),
    timeMs: index,
    severity: "INFO",
    attrs: {},
  }));
  logs.push({
    id: "large",
    body: "x".repeat(128 << 10),
    timeMs: 300,
    severity: "INFO",
    attrs: { source: "owner" },
  });
  useTelemetryStore.getState().ingestLogs(logs);
  const telemetry = captureClientTelemetry(new AbortController().signal);
  const snapshot = telemetry.read();
  expect(snapshot.logs).toHaveLength(301);
  expect(snapshot.logs[0]!.id).toBe("large");
  snapshot.logs[0]!.body = "caller edit";
  snapshot.logs[0]!.attrs.source = "caller edit";
  expect(useTelemetryStore.getState().logs.at(-1)!.body.length).toBe(128 << 10);
  expect(useTelemetryStore.getState().logs.at(-1)!.attrs.source).toBe("owner");
  telemetry.clear();
  expect(telemetry.read()).toEqual({ spans: [], metrics: [], logs: [] });
  expect(snapshot.logs).toHaveLength(301);
});

it("refuses reads and clearing after its component lifetime ends", () => {
  useTelemetryStore
    .getState()
    .ingestLogs([{ id: "kept", body: "kept", timeMs: 0, severity: "INFO", attrs: {} }]);
  const owner = new AbortController();
  const telemetry = captureClientTelemetry(owner.signal);
  const retired = new Error("component retired");
  owner.abort(retired);
  expect(() => telemetry.read()).toThrow(retired);
  expect(() => telemetry.clear()).toThrow(retired);
  expect(useTelemetryStore.getState().logs).toHaveLength(1);
});
