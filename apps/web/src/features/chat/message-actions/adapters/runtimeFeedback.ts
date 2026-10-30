import { type FlameClient } from "@flame/runtime-client";
import { asItemId, asRunId, asSessionId } from "@flame/runtime-client";
import { type MessageFeedbackGateway } from "@/features/chat/message-actions/application/feedback";
export function createRuntimeFeedbackGateway(client: FlameClient): MessageFeedbackGateway {
  return {
    async createMessageFeedback({ target, rating }) {
      await client.feedback.create({
        sessionId: asSessionId(target.sessionId),
        runId: target.runId ? asRunId(target.runId) : undefined,
        itemId: asItemId(target.messageId),
        rating,
      });
    },
  };
}
