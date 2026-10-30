import { type Message } from "@/features/agent/public/sessionView";
import { type InputImage } from "@/features/composer/public/input";
import { flattenText } from "@/features/agent/public/messageContent";
export interface MessageDraftContent {
  text: string;
  images: readonly InputImage[];
}

export function messageDraftContent(message: Message): MessageDraftContent {
  return { text: flattenText(message.blocks), images: messageImages(message) };
}

export function messageHasDraftContent(message: Message): boolean {
  const draft = messageDraftContent(message);
  return Boolean(draft.text || draft.images.length > 0);
}

function messageImages(message: Message): InputImage[] {
  return message.blocks
    .filter(
      (block): block is Extract<Message["blocks"][number], { kind: "image" }> =>
        block.kind === "image",
    )
    .map((block) => ({ mime: block.mime, data: block.data }));
}
