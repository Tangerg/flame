import { failureText as untranslatedFailureText } from "@flame/component-sdk";
import { isErrorType, RpcError } from "@flame/runtime-client";
import { t, isMessageKey } from "@/lib/i18n/index";

export function describeActiveRunRefusal(problem: {
  code?: string;
  activeRun?: { status: string };
}): string | undefined {
  if (problem.code !== "session_has_active_run") return undefined;
  if (problem.activeRun?.status === "running") return t("runError.activeRun.running");
  if (problem.activeRun?.status === "waiting") return t("runError.activeRun.waiting");
  return undefined;
}

// A Problem type has a user-facing message exactly when the base dictionary has one.
export function describeErrorType(type: string | undefined): string | undefined {
  const key = `rpcError.${type}`;
  return type && isMessageKey(key) ? t(key) : undefined;
}

export function describeRpcError(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  return describeErrorType(err.data.type);
}

export function rpcErrorText(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  return describeRpcError(err) ?? (err.data.detail || err.message);
}

export function failureText(err: unknown, fallback: string): string {
  return describeRpcError(err) ?? untranslatedFailureText(err, fallback);
}

export function isUnsupportedMethod(err: unknown): boolean {
  return isErrorType(err, "method_not_found");
}
