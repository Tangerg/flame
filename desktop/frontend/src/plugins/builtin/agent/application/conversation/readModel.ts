import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import { selectRootNarrativeMessages } from "../view/runTree";
import type { TranscriptRow } from "./transcriptRows";

export function useActiveConversationMessages(): Message[] {
  return agentSessionView().useRootNarrativeMessages();
}

export function useActiveConversationRows(): readonly TranscriptRow[] {
  return agentSessionView().useTranscriptRows();
}

export function getActiveConversationMessages(): Message[] {
  return selectRootNarrativeMessages(agentSessionView().getCurrentView());
}
