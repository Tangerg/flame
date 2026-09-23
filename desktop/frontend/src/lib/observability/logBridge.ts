import { logs, SeverityNumber } from "@opentelemetry/api-logs";

export type LogLevel = "debug" | "info" | "warn" | "error";

const SEVERITY: Record<LogLevel, { number: SeverityNumber; text: string }> = {
  debug: { number: SeverityNumber.DEBUG, text: "DEBUG" },
  info: { number: SeverityNumber.INFO, text: "INFO" },
  warn: { number: SeverityNumber.WARN, text: "WARN" },
  error: { number: SeverityNumber.ERROR, text: "ERROR" },
};

const LOGGER_NAME = "flame-frontend";

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
