import { type Message } from "@/features/agent/public/sessionView";
import { agentSessionView } from "@/features/agent/application/ports/sessionView";
import { selectRootNarrativeMessages } from "@/features/agent/application/view/runTree";
import { type TranscriptRow } from "@/features/agent/application/conversation/transcriptRows";
export function useActiveConversationMessages(): Message[] {
  return agentSessionView().useRootNarrativeMessages();
}

export function useActiveConversationRows(): readonly TranscriptRow[] {
  return agentSessionView().useTranscriptRows();
}

export function getActiveConversationMessages(): Message[] {
  return selectRootNarrativeMessages(agentSessionView().getCurrentView());
}
