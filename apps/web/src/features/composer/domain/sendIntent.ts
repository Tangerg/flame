import { joinDraftParts } from "@/features/composer/domain/draft";
interface SlashIntent {
  cmd: string;
  args: string;
}

interface ComposerSendIntent {
  text: string;
  body: string;
  slash: SlashIntent | null;
  shouldSend: boolean;
  historyText: string | null;
}

interface SendableDraft {
  value: string;
  images: readonly unknown[];
  pastes: ReadonlyArray<{ text: string }>;
  references: readonly unknown[];
}

export function hasSendableContent({ value, images, pastes, references }: SendableDraft): boolean {
  return Boolean(value.trim() || images.length > 0 || pastes.length > 0 || references.length > 0);
}

export function createComposerSendIntent(draft: SendableDraft): ComposerSendIntent {
  const text = draft.value.trim();
  const body = joinDraftParts([text, ...draft.pastes.map((paste) => paste.text)]);
  const shouldSend = hasSendableContent(draft);
  return {
    text,
    body,
    slash: text ? parseSlash(text) : null,
    shouldSend,
    historyText: text || null,
  };
}

function parseSlash(text: string): SlashIntent | null {
  if (!text.startsWith("/")) return null;
  const space = text.search(/\s/);
  if (space === -1) return { cmd: text, args: "" };
  return { cmd: text.slice(0, space), args: text.slice(space + 1) };
}
