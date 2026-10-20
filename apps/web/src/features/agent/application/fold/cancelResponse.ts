import { type AgentCancelResult } from "@/features/agent/public/events";
import { type AgentSessionView } from "@/features/agent/public/sessionView";
import { foldRunSnapshot } from "@/features/agent/application/fold/runSnapshot";
export function foldCancelRunResponse(
  state: AgentSessionView,
  response: AgentCancelResult,
): AgentSessionView {
  const withRun = foldRunSnapshot(state, response.run);
  return response.type === "child" ? foldRunSnapshot(withRun, response.rootRun) : withRun;
}
