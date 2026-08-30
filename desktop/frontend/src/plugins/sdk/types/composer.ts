/** Returning `true` tells the host to stop the browser default. */
export interface ComposerKeyContext {
  value: string;
  onChange: (next: string) => void;
  submit: () => void;
  event: KeyboardEvent;
}

export interface ComposerKeyBindingSpec {
  /** Same format as `host.shortcuts.register`. */
  key: string;
  description?: string;
  /** Return `true` to call `preventDefault` on the keypress. */
  handler: (ctx: ComposerKeyContext) => boolean | void;
}

export interface ComposerSubmitModeDraft {
  /** The exact controlled textarea value at the submit boundary. */
  rawText: string;
  /** Trimmed typed text, BEFORE staged paste material is appended. */
  text: string;
  /** The complete body, INCLUDING staged pasted text. */
  body: string;
  slash: { command: string; args: string } | null;
  hasImages: boolean;
  hasPastes: boolean;
}

export interface ComposerSubmitModeContext extends ComposerSubmitModeDraft {
  /** Commit the same history + clear transaction as an accepted normal send. */
  accept(): void;
  /** Clear without recording history, used when a command only arms a mode. */
  clear(): void;
}

/**
 * The composer remains the only draft and submit-pipeline owner. A mode may claim one
 * submit, but must call `accept` ONLY after its authoritative command has committed —
 * otherwise the draft stays available for recovery.
 */
export interface ComposerSubmitModeSpec {
  id: string;
  matches(draft: ComposerSubmitModeDraft): boolean;
  submit(context: ComposerSubmitModeContext): void;
}

/** `send` lets a command queue a real agent message AFTER running its local logic. */
export interface SlashCommandRunCtx {
  args: string;
  send: (text: string) => void;
}

export interface SlashCommandSpec {
  description: string;
  /** Absent makes the command a HINT ONLY: Enter forwards the raw text as a normal user
   *  message. */
  run?: (ctx: SlashCommandRunCtx) => void | Promise<void>;
}
