import {
  MutationJournalError,
  MutationLookupError,
  RpcError,
  RpcProtocolError,
  RpcTransportError,
} from "@flame/runtime-client";
import { type AgentProblem } from "@/features/agent/public/sessionView";
import { runtimeProblem } from "@/features/agent/adapters/runtimeAgentFacts";
export function agentProblemFromRpcFailure(error: unknown): AgentProblem | null {
  if (error instanceof MutationJournalError) return { message: error.message };
  if (
    error instanceof RpcTransportError ||
    error instanceof RpcProtocolError ||
    error instanceof MutationLookupError
  )
    return { code: "transport_error" };
  if (!(error instanceof RpcError)) return null;
  return runtimeProblem(error.data);
}
