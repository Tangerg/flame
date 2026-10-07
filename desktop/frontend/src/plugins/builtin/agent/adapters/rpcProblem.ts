import { RpcError, RpcTransportError } from "@flame/runtime-contract/client";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";
import { requestProblem } from "./runtimeAgentFacts";

export function agentProblemFromRpcFailure(error: unknown): AgentProblem | null {
  if (error instanceof RpcTransportError) return { code: "transport_error" };
  if (!(error instanceof RpcError)) return null;
  return requestProblem(error.data);
}
