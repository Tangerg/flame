// Protocol error type → locale key. Each key's leaf is the wire symbol VERBATIM, keeping the
// table one-to-one with the protocol; behaviour still branches on `isErrorType`.

import {
  errorDetail,
  errorType,
  isErrorType,
  RPC_METHOD_NOT_FOUND,
  RpcError,
  RpcTransportError,
} from "@/rpc";
import type { ProblemData } from "@/rpc";
import { t } from "./i18n";

export const MAPPED_TYPES: readonly string[] = [
  "session_busy",
  "session_has_active_run",
  "run_not_root",
  // The two replay refusals are deliberately ABSENT: a client answers those by reattaching.
  "run_waiting",
  "run_finished",
  "stale_segment",
  "checkpoint_unavailable",
  "workspace_unavailable",
  "vcs_unavailable",
  // One symbol per provider failure MODE (API.md §8.4), never free-text detail.
  "rate_limited",
  "invalid_api_key",
  "timeout",
  "provider_unavailable",
  "provider_rejected",
  "provider_error",
  "agent_stuck",
  // Ordinary run outcomes, not protocol faults: without copy the banner calls the person's
  // own Deny "an unknown error".
  "denied_by_user",
  "tool_failed",
  "tool_canceled",
  "child_run_canceled",
  // Carry no per-occurrence detail, so the words are ours to supply.
  "internal_error",
  "run_lost",
  // Inline verdicts: they ride a result rather than failing the call, symbol only.
  "mcp_authorization_required",
  "mcp_authorization_failed",
  "mcp_dial_failed",
  "provider_not_configured",
  "provider_test_failed",
  "mcp_authorization_attempt_not_found",
];

/** `undefined` for an unmapped type; callers append their own context-specific fallback. */
export function describeErrorType(type: string | undefined): string | undefined {
  return type && MAPPED_TYPES.includes(type) ? t(`rpcError.${type}`) : undefined;
}

export function describeRpcError(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  return describeErrorType(errorType(err.data));
}

/** For a ProblemData riding a RESULT: detail, then locale copy, then the bare symbol. Detail
 *  comes FIRST here and LAST in `rpcErrorText`, where it is usually the technical cause. */
export function describeProblem(problem: ProblemData | undefined): string | undefined {
  if (!problem) return undefined;
  return errorDetail(problem) || describeErrorType(problem.type) || problem.type || undefined;
}

/** `undefined` for NON-RPC errors (transport failures, programming errors). */
export function rpcErrorText(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  return describeRpcError(err) ?? errorDetail(err.data) ?? err.message;
}

/** True when the runtime does not implement the method. It answers an unknown one with HTTP
 *  404 wrapping a -32601, so HTTP reports a transport error and in-process reports the
 *  -32601 directly — both are this. */
export function isUnsupportedMethod(err: unknown): boolean {
  return (
    (err instanceof RpcTransportError && err.status === 404) ||
    (err instanceof RpcError && err.code === RPC_METHOD_NOT_FOUND)
  );
}

/** A collection the connected Runtime does not implement is EMPTY, not broken. */
export function emptyListIfUngated(error: unknown): never[] {
  if (isErrorType(error, "capability_not_negotiated")) return [];
  throw error;
}
