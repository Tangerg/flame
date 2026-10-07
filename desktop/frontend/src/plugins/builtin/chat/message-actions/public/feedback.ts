import { useCallback, useMemo, useSyncExternalStore } from "react";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import {
  messageFeedbackRating,
  submitMessageFeedback as submitMessageFeedbackIntent,
  subscribeMessageFeedback,
  type MessageFeedbackTarget,
} from "../application/feedback";
import type { FeedbackRating } from "../domain/feedback";

interface MessageFeedbackModel {
  rating: FeedbackRating | undefined;
  submit(rating: FeedbackRating): Promise<FeedbackRating>;
}

export function useMessageFeedback(sessionId: string, message: Message): MessageFeedbackModel {
  const target = useMemo<MessageFeedbackTarget>(
    () => ({
      sessionId,
      messageId: message.id,
      runId: message.runId ?? undefined,
    }),
    [message.id, message.runId, sessionId],
  );
  const subscribe = useCallback(
    (listener: () => void) => subscribeMessageFeedback(target, listener),
    [target],
  );
  const snapshot = useCallback(() => messageFeedbackRating(target), [target]);
  const rating = useSyncExternalStore(subscribe, snapshot, snapshot);
  const submit = useCallback(
    (next: FeedbackRating) => submitMessageFeedbackIntent(target, next),
    [target],
  );
  return useMemo(() => ({ rating, submit }), [rating, submit]);
}
