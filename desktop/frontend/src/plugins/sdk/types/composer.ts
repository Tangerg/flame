export interface ComposerKeyContext {
  value: string;
  onChange: (next: string) => void;
  submit: () => void;
  event: KeyboardEvent;
}

export interface ComposerKeyBindingSpec {
  key: string;
  description?: string;
  handler: (ctx: ComposerKeyContext) => boolean | void;
}

export interface ComposerSubmitModeDraft {
  rawText: string;
  text: string;
  body: string;
  slash: { command: string; args: string } | null;
  hasImages: boolean;
  hasPastes: boolean;
}

export interface ComposerSubmitModeContext extends ComposerSubmitModeDraft {
  accept(): void;
  clear(): void;
}

export interface ComposerSubmitModeSpec {
  id: string;
  matches(draft: ComposerSubmitModeDraft): boolean;
  submit(context: ComposerSubmitModeContext): void;
}

export interface SlashCommandRunCtx {
  args: string;
  send: (text: string) => void;
}

export interface SlashCommandSpec {
  description: string;
  run?: (ctx: SlashCommandRunCtx) => void | Promise<void>;
}
