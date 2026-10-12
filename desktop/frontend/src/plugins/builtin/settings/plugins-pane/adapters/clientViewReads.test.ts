import { afterEach, expect, it, vi } from "vitest";
import { useTelemetryStore, type LogRow } from "@/lib/observability/stores";
import { createClientViewReads, readClientDiagnostics } from "./clientViewReads";

afterEach(() => useTelemetryStore.getState().clear());

const log = (id: string, body = id): LogRow => ({
  id,
  body,
  timeMs: 0,
  severity: "INFO",
  attrs: {},
});

it("reads a bounded copy of local observations and reports omissions", () => {
  const rows = Array.from({ length: 300 }, (_, i) => log(String(i)));
  rows.push(log("too-large", "x".repeat(128 << 10)));
  useTelemetryStore.getState().ingestLogs(rows);
  const snapshot = readClientDiagnostics();
  expect(snapshot.source).toBe("currentClient");
  expect(snapshot.logs.rows).toHaveLength(256);
  expect(snapshot.logs.omittedFromRead).toBe(45);
  expect(snapshot.logs.rows[0]?.id).toBe("299");
  snapshot.logs.rows[0]!.body = "guest edit";
  expect(useTelemetryStore.getState().logs[299]?.body).toBe("299");
  expect(new TextEncoder().encode(JSON.stringify(snapshot.logs)).byteLength).toBeLessThan(
    129 << 10,
  );
});

it("renews exact resource admission before reading newer local observations", async () => {
  const binding = { installationId: "first", digest: "release", viewId: "diagnostics" };
  const plugins = { readView: vi.fn(async () => ({ html: "immutable" })) };
  const snapshot = vi.fn(readClientDiagnostics);
  const reads = createClientViewReads(plugins, binding, snapshot);
  binding.digest = "replacement";
  const signal = new AbortController().signal;
  await reads.resource(signal);
  expect((await reads.initial(signal)).logs.rows).toEqual([]);
  useTelemetryStore.getState().ingestLogs([log("later")]);
  expect((await reads.read(undefined, signal)).logs.rows[0]?.id).toBe("later");
  expect(plugins.readView).toHaveBeenLastCalledWith({ ...binding, digest: "release" }, signal);
  await expect(reads.read("guest cursor", signal)).rejects.toThrow("no cursor");
  expect(snapshot).toHaveBeenCalledTimes(2);
  plugins.readView.mockRejectedValueOnce(new Error("revoked"));
  await expect(reads.read(undefined, signal)).rejects.toThrow("revoked");
  expect(snapshot).toHaveBeenCalledTimes(2);
  plugins.readView.mockResolvedValueOnce({ html: "replacement" });
  await expect(reads.read(undefined, signal)).rejects.toThrow("resource changed");
  expect(snapshot).toHaveBeenCalledTimes(2);
});
