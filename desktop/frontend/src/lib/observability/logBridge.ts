// Turns a log call into an OTel LogRecord, correlated with the active span because the SDK
// fills trace_id/span_id from `context.active()` at emit time.
//
// Before a provider is installed, `logs.getLogger` returns a no-op and `emit()` is cheap.
// `host.log` is NOT a hot path (CLAUDE.md §5), so every call is bridged and the SINK does
// the batching.

import { logs, SeverityNumber } from "@opentelemetry/api-logs";

/**
 * Declared here rather than in the plugin SDK: these four exist only as the OTel severities
 * `SEVERITY` below maps onto, and that map is exhaustive by construction only if both come
 * from one declaration. `lib` may not depend on the plugin layer, so the vocabulary lives
 * at the bottom and the SDK re-exports the name.
 */
export type LogLevel = "debug" | "info" | "warn" | "error";

const SEVERITY: Record<LogLevel, { number: SeverityNumber; text: string }> = {
  debug: { number: SeverityNumber.DEBUG, text: "DEBUG" },
  info: { number: SeverityNumber.INFO, text: "INFO" },
  warn: { number: SeverityNumber.WARN, text: "WARN" },
  error: { number: SeverityNumber.ERROR, text: "ERROR" },
};

const LOGGER_NAME = "flame-frontend";

/** Emit one frontend log line as an OTel LogRecord, attributed to `scope`
 *  (the plugin/kernel name). The active span's trace context is attached
 *  natively by the SDK. */
export function emitLog(scope: string, level: LogLevel, args: unknown[]): void {
  const sev = SEVERITY[level];
  logs.getLogger(LOGGER_NAME).emit({
    severityNumber: sev.number,
    severityText: sev.text,
    body: args.map(stringify).join(" "),
    attributes: { "scope.name": scope },
  });
}

function stringify(value: unknown): string {
  if (typeof value === "string") return value;
  if (value instanceof Error) return value.stack ?? `${value.name}: ${value.message}`;
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}
