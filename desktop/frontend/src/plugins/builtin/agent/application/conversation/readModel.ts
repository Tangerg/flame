import type { Message, TimelineEntry, ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import { selectRootNarrativeMessages } from "../view/runTree";
import type { TranscriptRow } from "./transcriptRows";

interface ActiveConversationSnapshot {
  messages: Message[];
  timeline: TimelineEntry[];
  toolCalls: Record<string, ToolCall>;
}

export function useActiveConversationMessages(): Message[] {
  return agentSessionView().useRootNarrativeMessages();
}

export function useActiveConversationRows(): readonly TranscriptRow[] {
  return agentSessionView().useTranscriptRows();
}

export function getActiveConversationSnapshot(): ActiveConversationSnapshot {
  const view = agentSessionView().getCurrentView();
  return {
    messages: selectRootNarrativeMessages(view),
    timeline: view.timeline,
    toolCalls: view.toolCalls,
  };
}
