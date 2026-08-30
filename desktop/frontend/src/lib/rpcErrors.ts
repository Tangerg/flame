// One home for protocol error type → copy, so the same type reads the same everywhere;
// behaviour still branches on `isErrorType` at the call site. The words live in the locale
// catalogs because a literal here is outside the dictionaries the locale guard checks. Each
// key's leaf is the wire symbol VERBATIM, keeping the table one-to-one with the protocol.

import { errorDetail, errorType, RPC_METHOD_NOT_FOUND, RpcError, RpcTransportError } from "@/rpc";
import type { ProblemData } from "@/rpc";
import { t } from "./i18n";

export const MAPPED_TYPES: readonly string[] = [
  "session_busy",
  "session_has_active_run",
  "run_not_root",
  // The two replay refusals (replay_cursor_invalid / replay_unavailable) are deliberately
  // ABSENT: a client answers those by reattaching and rebuilding, and there is nothing for
  // a person to do or be told.
  "run_waiting",
  "run_finished",
  "stale_segment",
  "checkpoint_unavailable",
  "workspace_unavailable",
  "vcs_unavailable",
  // One stable symbol per provider failure MODE (API.md §8.4), so copy and behaviour branch
  // on the symbol rather than on free-text detail.
  "rate_limited",
  "invalid_api_key",
  "timeout",
  "provider_unavailable",
  "provider_rejected",
  "provider_error",
  "agent_stuck",
  // How a run most often ends short of completing: the person declined the action, or a
  // tool or delegated run stopped. Ordinary outcomes, not protocol faults — without copy
  // the banner calls the most self-explanatory cause there is "an unknown error".
  "denied_by_user",
  "tool_failed",
  "tool_canceled",
  "child_run_canceled",
  // Carry no per-occurrence detail — an internal error must not put its internals on the
  // wire — so the words are ours to supply.
  "internal_error",
  "run_lost",
  // Inline status verdicts: they ride a result rather than failing the call, and the
  // runtime sends the symbol ALONE.
  "mcp_authorization_required",
  "mcp_authorization_failed",
  "mcp_dial_failed",
  "provider_not_configured",
  "provider_test_failed",
  // Past the published retention window the attempt is unrecoverable, so the person starts
  // a new sign-in instead of retrying the id.
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

/** For a ProblemData that rides a RESULT rather than failing the call: per-occurrence
 *  detail, then this locale's copy, then the bare symbol. Detail comes FIRST because it
 *  describes this occurrence — `rpcErrorText` orders it the other way, since an RPC
 *  failure's detail is usually the technical cause rather than something to show someone. */
export function describeProblem(problem: ProblemData | undefined): string | undefined {
  if (!problem) return undefined;
  return errorDetail(problem) || describeErrorType(problem.type) || problem.type || undefined;
}

/** `undefined` for NON-RPC errors (transport failures, programming errors). */
export function rpcErrorText(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  return describeRpcError(err) ?? errorDetail(err.data) ?? err.message;
}

/** True when the connected runtime does not implement the method. It answers an unknown one
 *  with HTTP 404 wrapping a -32601 envelope, so the HTTP transport reports a transport error
 *  while an in-process transport reports the -32601 directly — both are this. Lets a panel
 *  render a calm "unavailable on this runtime" state instead of a hard error. */
export function isUnsupportedMethod(err: unknown): boolean {
  return (
    (err instanceof RpcTransportError && err.status === 404) ||
    (err instanceof RpcError && err.code === RPC_METHOD_NOT_FOUND)
  );
}
