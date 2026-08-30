import type { ContentBlock } from "@/rpc";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { OPTIMISTIC_USER_MESSAGE_PREFIX } from "../application/view/optimisticMessageIdentity";
import { userContentBlocks } from "../application/fold/projections";
import { ExactSequence } from "@/foundation/exactSequence";

const optimisticMessageIds = new ExactSequence();

export interface OptimisticUserMessage {
  localId: string;
  message: Message;
}

export function createOptimisticUserMessage(content: ContentBlock[]): OptimisticUserMessage {
  const localId = `${OPTIMISTIC_USER_MESSAGE_PREFIX}${optimisticMessageIds.issue()}`;
  return {
    localId,
    message: {
      id: localId,
      role: "user",
      runId: null,
      createdAt: new Date().toISOString(),
      blocks: userContentBlocks(content),
    },
  };
}

export function localUserMessage(messageId: string, content: ContentBlock[]): Message {
  return {
    id: messageId,
    role: "user",
    runId: null,
    createdAt: new Date().toISOString(),
    blocks: userContentBlocks(content),
  };
}
