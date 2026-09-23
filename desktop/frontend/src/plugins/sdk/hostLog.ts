import type { InstanceMeta, Logger } from "dougong";
import { emitLog as emitOtelLog } from "@/lib/observability/logBridge";
import type { LogLevel } from "./types";

const CONSOLE_METHOD: Record<LogLevel, "log" | "info" | "warn" | "error"> = {
  debug: "log",
  info: "info",
  warn: "warn",
  error: "error",
};

function isMeta(value: unknown): value is InstanceMeta {
  return typeof value === "object" && value !== null && "pluginName" in value;
}

function write(level: LogLevel, message: unknown, details: unknown[]): void {
  const [head, ...rest] = details;
  const plugin = isMeta(head) ? head.pluginName : "kernel";
  const args = isMeta(head) ? rest : details;

  console[CONSOLE_METHOD[level]](`[plugin:${plugin}]`, message, ...args);
  emitOtelLog(plugin, level, [message, ...args]);
}

export const kernelLogger: Logger = {
  debug: (message, ...details) => write("debug", message, details),
  info: (message, ...details) => write("info", message, details),
  warn: (message, ...details) => write("warn", message, details),
  error: (message, ...details) => write("error", message, details),
};
