import { type AgentInput } from "@/features/agent/public/input";
import { type ComposerDraftInput } from "@/features/composer/public/draft";
import { type ComposerReference, type ComposerImage } from "@/features/composer/public/attachments";
export function agentInputToComposerDraft(input: AgentInput): ComposerDraftInput {
  const text: string[] = [];
  const images: Omit<ComposerImage, "id">[] = [];
  const references: ComposerReference[] = [];
  for (const part of input.parts) {
    if (part.kind === "text") text.push(part.text);
    else if (part.kind === "image") images.push({ mime: part.mime, data: part.data });
    else references.push({ label: part.label, text: part.text });
  }
  return { text: text.join("\n\n"), images, references };
}
