import type { Message } from "@/plugins/sdk/types/agentSessionView";
import type { InputImage } from "@/plugins/builtin/chat/composer/public/input";
import { flattenText } from "@/plugins/builtin/agent/public/messageContent";

export interface MessageDraftContent {
  text: string;
  images: readonly InputImage[];
}

export interface RegenerationPrompt extends MessageDraftContent {
  runId?: string;
}

export function messageDraftContent(message: Message): MessageDraftContent {
  return { text: flattenText(message.blocks), images: messageImages(message) };
}

export function messageHasDraftContent(message: Message): boolean {
  const draft = messageDraftContent(message);
  return Boolean(draft.text || draft.images.length > 0);
}

export function regenerationPromptBefore(
  messages: Message[],
  targetMessageId: string,
): RegenerationPrompt | null {
  const index = messages.findIndex((message) => message.id === targetMessageId);
  if (index < 0) return null;
  for (let cursor = index - 1; cursor >= 0; cursor--) {
    const message = messages[cursor]!;
    if (message.role !== "user") continue;
    const images = messageImages(message);
    const text = flattenText(message.blocks).trim();
    if (!text && images.length === 0) return null;
    return { text, images, runId: message.runId ?? undefined };
  }
  return null;
}

function messageImages(message: Message): InputImage[] {
  return message.blocks
    .filter(
      (block): block is Extract<Message["blocks"][number], { kind: "image" }> =>
        block.kind === "image",
    )
    .map((block) => ({ mime: block.mime, data: block.data }));
}
