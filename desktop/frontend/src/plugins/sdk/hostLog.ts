// `ctx.log` is Core's and already bound to the plugin, so attribution, the OTel bridge and
// the LOG_SUBSCRIBER fan-out all hang off ONE seam — `createHost({ logger })` — rather than
// a context wrapper that would shadow a capability Core already provides.

import type { InstanceMeta, Logger } from "dougong";
import { emitLog as emitOtelLog } from "@/lib/observability/logBridge";
import { safeCall } from "./errors";
import { LOG_SUBSCRIBER } from "./kernelPoints";
import { contributionsTo } from "./kernel";
import type { LogLevel } from "./types";

// Method name, not a reference, so vitest's `vi.spyOn(console, "info")` after
// module load still binds.
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
  // Core passes `meta` first among the details; anything the Host itself logs
  // (an onError report, say) arrives without one.
  const [head, ...rest] = details;
  const plugin = isMeta(head) ? head.pluginName : "kernel";
  const args = isMeta(head) ? rest : details;

  console[CONSOLE_METHOD[level]](`[plugin:${plugin}]`, message, ...args);
  // Third pillar: mirror into OTel logs (a no-op until a LoggerProvider is
  // installed). Correlated with the active span by the SDK.
  emitOtelLog(plugin, level, [message, ...args]);

  const event = { plugin, level, args: [message, ...args], timestamp: Date.now() };
  for (const fn of contributionsTo(LOG_SUBSCRIBER)) {
    safeCall(() => fn.item(event), "[plugin] log subscriber threw:");
  }
}

export const kernelLogger: Logger = {
  debug: (message, ...details) => write("debug", message, details),
  info: (message, ...details) => write("info", message, details),
  warn: (message, ...details) => write("warn", message, details),
  error: (message, ...details) => write("error", message, details),
};
