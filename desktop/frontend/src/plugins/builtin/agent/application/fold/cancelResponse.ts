import type { AgentCancelResult } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { foldRunSnapshot } from "./runSnapshot";

export function foldCancelRunResponse(
  state: AgentSessionView,
  response: AgentCancelResult,
): AgentSessionView {
  const withRun = foldRunSnapshot(state, response.run);
  return response.type === "child" ? foldRunSnapshot(withRun, response.rootRun) : withRun;
}
