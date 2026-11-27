import { type MessageKey } from "@/lib/i18n/index";
interface ComposerKeyContext {
  submit: () => void;
  event: KeyboardEvent;
}

export interface ComposerKeyBindingSpec {
  key: string;
  description?: string;
  handler: (ctx: ComposerKeyContext) => boolean | void;
}

export interface SlashCommandContext {
  args: string;
  hasAttachments: boolean;
  accept(): void;
}

export type SlashCommandHandler = (context: SlashCommandContext) => void | Promise<void>;
export type SlashCommandSpec =
  { description: MessageKey; run?: never } | { description?: MessageKey; run: SlashCommandHandler };
