import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type Message } from "@/features/agent/public/sessionView";

import { type TranscriptRow } from "@/features/agent/application/conversation/transcriptRows";
export function useActiveConversationMessages(): Message[] {
  const agentAccess = useAgentDependencies();

  return agentAccess.view.useRootNarrativeMessages();
}

export function useHasPrecedingRootRun(runId: string | null): boolean {
  return useAgentDependencies().view.useHasPrecedingRootRun(runId);
}

export function useActiveConversationRows(): readonly TranscriptRow[] {
  const agentAccess = useAgentDependencies();

  return agentAccess.view.useTranscriptRows();
}
