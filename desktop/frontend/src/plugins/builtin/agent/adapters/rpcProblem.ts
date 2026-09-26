import {
  errorActiveRun,
  errorDetail,
  errorRetryAfterSeconds,
  errorType,
  RpcError,
  RpcTransportError,
} from "@flame/runtime-contract/client";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";

export function agentProblemFromRpcFailure(error: unknown): AgentProblem | null {
  if (error instanceof RpcTransportError) return { code: "transport_error" };
  if (!(error instanceof RpcError)) return null;
  const retryAfterSeconds = errorRetryAfterSeconds(error.data);
  const activeRun = errorActiveRun(error.data);
  return {
    message: errorDetail(error.data),
    code: errorType(error.data),
    ...(retryAfterSeconds !== undefined ? { retryAfterSeconds } : {}),
    ...(activeRun ? { activeRun } : {}),
  };
}
