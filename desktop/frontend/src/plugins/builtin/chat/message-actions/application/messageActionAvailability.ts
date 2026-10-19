import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { messageHasDraftContent } from "./messageActionContent";

export function canCopyMessage(copy: { canCopy: boolean }): boolean {
  return copy.canCopy;
}

export function canEditMessage(message: Message): boolean {
  return message.role === "user" && messageHasDraftContent(message);
}

export function canUseMessageRunCheckpoint(message: Message): boolean {
  return message.role === "user" && Boolean(message.runId);
}

export function canRegenerateMessage(message: Message, conversation: readonly Message[]): boolean {
  return message.role === "assistant" && conversation.some(({ id }) => id === message.id);
}

export function canRateMessage(message: Message): boolean {
  return message.role === "assistant";
}
