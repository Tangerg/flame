import {
  createContext,
  useContext,
  useCallback,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { type Message } from "@/features/agent/public/sessionView";
import {
  type MessageFeedbackAccess,
  type MessageFeedbackTarget,
} from "@/features/chat/message-actions/application/feedback";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { type FeedbackRating } from "@/features/chat/message-actions/domain/feedback";
interface MessageFeedbackModel {
  rating: FeedbackRating | undefined;
  submit(rating: FeedbackRating): Promise<FeedbackRating>;
}

const MessageFeedbackContext = createContext<MessageFeedbackAccess | null>(null);

export function MessageFeedbackProvider({
  value,
  children,
}: {
  value: MessageFeedbackAccess;
  children: ReactNode;
}) {
  return <MessageFeedbackContext value={value}>{children}</MessageFeedbackContext>;
}

export function useMessageFeedback(
  sessionId: string,
  message: Pick<Message, "id" | "runId">,
): MessageFeedbackModel {
  const access = useContext(MessageFeedbackContext);
  if (!access) throw new Error("MessageFeedbackProvider is missing");
  const generation = useSyncExternalStore(
    access.subscribeGeneration,
    access.capture,
    access.capture,
  );
  const target = useMemo<MessageFeedbackTarget>(
    () => ({
      sessionId,
      messageId: message.id,
      runId: message.runId ?? undefined,
    }),
    [message.id, message.runId, sessionId],
  );
  const subscribe = useCallback(
    (listener: () => void) => access.subscribe(target, listener),
    [access, target],
  );
  const snapshot = useCallback(() => access.rating(target), [access, target]);
  const rating = useSyncExternalStore(subscribe, snapshot, snapshot);
  const submit = useCallback(
    async (next: FeedbackRating) => {
      if (!generation) throw new GenerationRetiredError("message_feedback_generation");
      return generation.submit(target, next);
    },
    [generation, target],
  );
  return useMemo(() => ({ rating, submit }), [rating, submit]);
}
