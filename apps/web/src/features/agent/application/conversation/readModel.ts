import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type Message } from "@/features/agent/public/sessionView";

import { selectRootNarrativeMessages } from "@/features/agent/application/view/runTree";
import { type TranscriptRow } from "@/features/agent/application/conversation/transcriptRows";
export function useActiveConversationMessages(): Message[] {
  const agentAccess = useAgentDependencies();

  return agentAccess.view.useRootNarrativeMessages();
}

export function useActiveConversationRows(): readonly TranscriptRow[] {
  const agentAccess = useAgentDependencies();

  return agentAccess.view.useTranscriptRows();
}

export function getActiveConversationMessages(agentAccess: AgentDependencies): Message[] {
  return selectRootNarrativeMessages(agentAccess.view.getCurrentView());
}
