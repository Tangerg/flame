import type { ClientTelemetry } from "@flame/plugin-sdk";
import { useTelemetryStore } from "./stores";

export function captureClientTelemetry(signal: AbortSignal): ClientTelemetry {
  return Object.freeze({
    read() {
      signal.throwIfAborted();
      const state = useTelemetryStore.getState();
      return structuredClone({
        spans: state.spans.toReversed(),
        logs: state.logs.toReversed(),
        metrics: Object.values(state.metrics).sort((a, b) => a.id.localeCompare(b.id)),
      });
    },
    clear() {
      signal.throwIfAborted();
      useTelemetryStore.getState().clear();
    },
  });
}
