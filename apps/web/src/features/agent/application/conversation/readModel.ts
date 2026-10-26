import { useAgent, type AgentAccess } from "@/features/agent/public/agent";
import { type Message } from "@/features/agent/public/sessionView";

import { selectRootNarrativeMessages } from "@/features/agent/application/view/runTree";
import { type TranscriptRow } from "@/features/agent/application/conversation/transcriptRows";
export function useActiveConversationMessages(): Message[] {
  const agentAccess = useAgent();

  return agentAccess.view.useRootNarrativeMessages();
}

export function useActiveConversationRows(): readonly TranscriptRow[] {
  const agentAccess = useAgent();

  return agentAccess.view.useTranscriptRows();
}

export function getActiveConversationMessages(agentAccess: AgentAccess): Message[] {
  return selectRootNarrativeMessages(agentAccess.view.getCurrentView());
}
