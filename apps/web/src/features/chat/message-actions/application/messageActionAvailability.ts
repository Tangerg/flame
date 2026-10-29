import { type Message } from "@/features/agent/public/sessionView";
import { messageHasDraftContent } from "@/features/chat/message-actions/application/messageActionContent";
export function canCopyMessage(copy: { canCopy: boolean }): boolean {
  return copy.canCopy;
}

export function canEditMessage(message: Message): boolean {
  return message.role === "user" && messageHasDraftContent(message);
}

export function canRollbackMessage(
  message: Message,
  rootMessages: readonly Message[],
): message is Message & { runId: string } {
  return (
    message.role === "user" &&
    Boolean(message.runId) &&
    rootMessages.some(({ id, runId }) => id === message.id && runId === message.runId)
  );
}

export function canForkMessage(message: Message): message is Message & { runId: string } {
  return message.role === "user" && Boolean(message.runId);
}

export function canRegenerateMessage(
  message: Message,
  conversation: readonly Message[],
): message is Message & { runId: string } {
  return (
    message.role === "assistant" &&
    Boolean(message.runId) &&
    conversation.some(({ id, runId }) => id === message.id && runId === message.runId)
  );
}

export function canRateMessage(message: Message): boolean {
  return message.role === "assistant";
}
