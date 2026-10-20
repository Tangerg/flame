import { type FlameClient } from "@flame/runtime-client";
import { asItemId, asRunId, asSessionId } from "@flame/runtime-client";
import {
  MessageFeedbackOwner,
  type MessageFeedbackGateway,
} from "@/features/chat/message-actions/application/feedback";
function runtimeFeedbackGateway(client: FlameClient): MessageFeedbackGateway {
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

export function installRuntimeFeedbackGateway(runtimeClient: () => FlameClient) {
  const owner = MessageFeedbackOwner.install(runtimeFeedbackGateway(runtimeClient()));
  return {
    replaceRuntimeGeneration: () =>
      owner.replaceRuntimeGeneration(() => runtimeFeedbackGateway(runtimeClient())),
    dispose: () => owner.dispose(),
  };
}
