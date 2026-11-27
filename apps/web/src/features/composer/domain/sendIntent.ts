import {
  joinDraftParts,
  parseComposerSlashCommand,
  type ComposerSlashCommand,
} from "@/features/composer/domain/draft";

interface ComposerSendIntent {
  body: string;
  slash: ComposerSlashCommand | null;
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
    body,
    slash: parseComposerSlashCommand(draft.value),
    shouldSend,
    historyText: text || null,
  };
}
