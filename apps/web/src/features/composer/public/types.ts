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

interface ComposerSubmitModeDraft {
  rawText: string;
  text: string;
  body: string;
  slash: { command: string; args: string } | null;
  hasImages: boolean;
  hasPastes: boolean;
  hasReferences: boolean;
}

export interface ComposerSubmitModeContext extends ComposerSubmitModeDraft {
  accept(): void;
  clear(): void;
}

export interface ComposerSubmitModeSpec {
  id: string;
  matches(draft: ComposerSubmitModeDraft): boolean;
  submit(context: ComposerSubmitModeContext): void | Promise<void>;
}

interface SlashCommandRunCtx {
  args: string;
  accept(): void;
  send: (text: string) => boolean;
}

export interface SlashCommandSpec {
  description: MessageKey;
  run?: (ctx: SlashCommandRunCtx) => void | Promise<void>;
}
