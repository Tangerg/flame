import { type Message } from "@/features/agent/public/sessionView";
import { type InputImage } from "@/features/composer/public/input";
import { type ComposerReference } from "@/features/composer/public/attachments";
import { flattenText } from "@/features/agent/public/messageContent";
interface MessageDraftContent {
  text: string;
  images: readonly InputImage[];
  references: readonly ComposerReference[];
}

export function messageDraftContent(message: Message): MessageDraftContent {
  const images: InputImage[] = [];
  const references: ComposerReference[] = [];
  for (const block of message.blocks) {
    if (block.kind === "image") images.push({ mime: block.mime, data: block.data });
    if (block.kind === "reference") references.push({ label: block.label, text: block.text });
  }
  return { text: flattenText(message.blocks), images, references };
}

export function messageHasDraftContent(message: Message): boolean {
  const draft = messageDraftContent(message);
  return Boolean(draft.text || draft.images.length > 0 || draft.references.length > 0);
}
