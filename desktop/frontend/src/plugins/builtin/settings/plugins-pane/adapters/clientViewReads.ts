import type { FlameClient } from "@flame/runtime-contract/client";
import type { ReadPluginViewRequest } from "@flame/runtime-contract/wire";
import {
  useTelemetryStore,
  type SpanRow,
  type MetricRow,
  type LogRow,
} from "@/lib/observability/stores";
import type { PluginViewReads } from "../application/pluginView";

interface Rows<T> {
  rows: T[];
  omittedFromRead: number;
}

export interface ClientDiagnosticsSnapshot {
  format: "flame.client-diagnostics.v1";
  source: "currentClient";
  spans: Rows<SpanRow>;
  metrics: Rows<MetricRow>;
  logs: Rows<LogRow>;
}

const sectionBytes = 128 << 10;
const encoder = new TextEncoder();

function bounded<T>(values: readonly T[]): Rows<T> {
  const rows: T[] = [];
  let bytes = 2;
  for (const value of values) {
    const size = encoder.encode(JSON.stringify(value)).byteLength + 1;
    if (rows.length === 256 || bytes + size > sectionBytes) continue;
    bytes += size;
    rows.push(structuredClone(value));
  }
  return { rows, omittedFromRead: values.length - rows.length };
}

export function readClientDiagnostics(): ClientDiagnosticsSnapshot {
  const state = useTelemetryStore.getState();
  return {
    format: "flame.client-diagnostics.v1",
    source: "currentClient",
    spans: bounded(state.spans.toReversed()),
    logs: bounded(state.logs.toReversed()),
    metrics: bounded(Object.values(state.metrics).sort((a, b) => a.id.localeCompare(b.id))),
  };
}

export function createClientViewReads<T>(
  plugins: Pick<FlameClient["plugins"], "readView">,
  binding: Readonly<ReadPluginViewRequest>,
  snapshot: () => T,
): PluginViewReads<T> {
  const request = Object.freeze({ ...binding });
  let admittedHTML: string | undefined;
  return {
    binding: request,
    async resource(signal) {
      const resource = await plugins.readView(request, signal);
      admittedHTML = resource.html;
      return resource.html;
    },
    async initial() {
      return snapshot();
    },
    async read(cursor, signal) {
      if (cursor !== undefined) throw new Error("Client views have no cursor continuation");
      const resource = await plugins.readView(request, signal);
      if (resource.html !== admittedHTML) throw new Error("The client view resource changed");
      return snapshot();
    },
  };
}
