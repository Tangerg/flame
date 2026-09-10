// One submit path for the Enter key, the send button and plugin key bindings, so they
// cannot diverge. Owns slash routing and the clear-only-after-accepted invariant.

import { buildInput, type InputImage } from "@/plugins/builtin/chat/composer/public/input";
import type { PastedText } from "../domain/draft";
import { agentTextInput, type AgentInput } from "@/plugins/builtin/agent/public/input";
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
import { focusComposer } from "./focus";

export interface SubmitDeps {
  value: string;
  clear: () => void;
  sendInput: (input: AgentInput) => boolean;
  images: readonly InputImage[];
  pastes: readonly PastedText[];
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
  // Clearing the draft is what makes the send button unavailable, and a `disabled` control
  // cannot hold focus — so sending with the keyboard left focus on `<body>`, and the next Tab
  // restarted at the top of the document instead of continuing from the composer. Measured on
  // the narrative and dock routes.
  //
  // The two belong together, which is why they are one function: the focus return was first
  // written inside `accept`, and `accept` is only reached by a submit MODE. An ordinary message
  // takes the default path at the bottom of this file and a slash command takes the middle one,
  // so the fix ran for neither and the measurement did not budge. `clear()` is the moment a
  // submit is accepted — this file's own invariant — so it is the moment focus comes back.
  const consume = () => {
    clear();
    focusComposer();
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
      consume();
    };
    try {
      mode.submit({ ...modeDraft, accept, clear: consume });
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
        spec.run({ args: slash.args, send: (text: string) => sendInput(agentTextInput(text)) }),
      ).catch((err) => {
        console.error(`[plugin] command ${slash.cmd} threw:`, err);
        const owner = lookupSlashCommandOwner(slash.cmd) ?? "unknown";
        reportPluginError(owner, "command", err, `command: ${slash.cmd}`);
      });
      consume();
      return;
    }
  }
  if (!canSend()) return;
  if (!sendInput(buildInput(intent.body, images))) return;
  if (intent.historyText) recordHistory(intent.historyText);
  consume();
}
