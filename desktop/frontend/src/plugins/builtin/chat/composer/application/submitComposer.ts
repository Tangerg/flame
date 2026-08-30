// One submit path for the Enter key, the send button and plugin key bindings, so they
// cannot diverge. Owns slash routing and the clear-only-after-accepted invariant.

import {
  buildInput,
  textInput,
  type InputImage,
  type UserInput,
} from "@/plugins/builtin/chat/composer/public/input";
import type { PastedText } from "../domain/draft";
import { createComposerSendIntent } from "../domain/sendIntent";
import {
  COMPOSER_SUBMIT_MODE,
  lookupExtensionByKey,
  lookupExtensionOwner,
  lookupExtensionPoint,
  lookupSlashCommandOwner,
  reportPluginError,
  SLASH_COMMAND,
} from "@/plugins/sdk";

export interface SubmitDeps {
  value: string;
  clear: () => void;
  sendInput: (input: UserInput) => boolean;
  images: InputImage[];
  pastes: PastedText[];
  recordHistory: (text: string) => void;
  canSend: () => boolean;
}

/** Safe to call on empty text with no attachments. */
export function submitComposer({
  value,
  clear,
  sendInput,
  images,
  pastes,
  recordHistory,
  canSend,
}: SubmitDeps): void {
  const intent = createComposerSendIntent({ value, images, pastes });
  if (!intent.shouldSend) return;

  const modeDraft = {
    rawText: value,
    text: intent.text,
    body: intent.body,
    slash: intent.slash ? { command: intent.slash.cmd, args: intent.slash.args } : null,
    hasImages: images.length > 0,
    hasPastes: pastes.length > 0,
  };
  for (const mode of lookupExtensionPoint(COMPOSER_SUBMIT_MODE)) {
    let matches = false;
    try {
      matches = mode.matches(modeDraft);
    } catch (error) {
      reportPluginError(
        lookupExtensionOwner(COMPOSER_SUBMIT_MODE, mode.id) ?? "unknown",
        "command",
        error,
        `composer.submitMode:${mode.id}`,
      );
      return;
    }
    if (!matches) continue;

    let accepted = false;
    const accept = () => {
      if (accepted) return;
      accepted = true;
      if (intent.historyText) recordHistory(intent.historyText);
      clear();
    };
    try {
      mode.submit({ ...modeDraft, accept, clear });
    } catch (error) {
      reportPluginError(
        lookupExtensionOwner(COMPOSER_SUBMIT_MODE, mode.id) ?? "unknown",
        "command",
        error,
        `composer.submitMode:${mode.id}`,
      );
    }
    return;
  }

  // Slash routing applies only to a TEXT command: attachments are not command arguments,
  // so a "/cmd" still routes as the command and drops them.
  const slash = intent.slash;
  if (slash) {
    const spec = lookupExtensionByKey(SLASH_COMMAND, slash.cmd);
    if (spec?.run) {
      if (intent.historyText) recordHistory(intent.historyText);
      void Promise.resolve(
        spec.run({ args: slash.args, send: (text: string) => sendInput(textInput(text)) }),
      ).catch((err) => {
        console.error(`[plugin] command ${slash.cmd} threw:`, err);
        const owner = lookupSlashCommandOwner(slash.cmd) ?? "unknown";
        reportPluginError(owner, "command", err, `command: ${slash.cmd}`);
      });
      clear();
      return;
    }
  }
  if (!canSend()) return;
  if (!sendInput(buildInput(intent.body, images))) return;
  if (intent.historyText) recordHistory(intent.historyText);
  clear();
}
