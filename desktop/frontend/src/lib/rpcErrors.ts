import { isErrorType, RpcError } from "@flame/runtime-contract/client";
import { t } from "./i18n";

export const MAPPED_TYPES: readonly string[] = [
  "session_busy",
  "session_has_active_run",
  "run_not_root",
  "run_waiting",
  "run_finished",
  "stale_segment",
  "checkpoint_unavailable",
  "checkpoint_conflict",
  "prompt_source_too_large",
  "workspace_unavailable",
  "vcs_unavailable",
  "rate_limited",
  "invalid_api_key",
  "timeout",
  "provider_unavailable",
  "provider_rejected",
  "provider_error",
  "agent_stuck",
  "denied_by_user",
  "tool_failed",
  "tool_canceled",
  "child_run_canceled",
  "idempotency_in_progress",
  "internal_error",
  "run_lost",
  "mcp_authorization_attempt_not_found",
  "skill_not_found",
  "skill_unavailable",
  "revision_conflict",
  "mcp_server_not_found",
  "mcp_server_already_exists",
  "mcp_server_disabled",
  "schedule_not_found",
  "path_outside_root",
  "unsupported_mime",
  "interrupt_not_open",
  "idempotency_conflict",
  "plugin_changed",
];

export function describeActiveRunRefusal(problem: {
  code?: string;
  activeRun?: { status: string };
}): string | undefined {
  if (problem.code !== "session_has_active_run") return undefined;
  if (problem.activeRun?.status === "running") return t("runError.activeRun.running");
  if (problem.activeRun?.status === "waiting") return t("runError.activeRun.waiting");
  return undefined;
}

export function describeErrorType(type: string | undefined): string | undefined {
  return type && MAPPED_TYPES.includes(type) ? t(`rpcError.${type}`) : undefined;
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
  return rpcErrorText(err) ?? (err instanceof Error ? err.message : fallback);
}

export function isUnsupportedMethod(err: unknown): boolean {
  return isErrorType(err, "method_not_found");
}

export function emptyListIfUngated(error: unknown): never[] {
  if (isErrorType(error, "capability_not_negotiated")) return [];
  throw error;
}
