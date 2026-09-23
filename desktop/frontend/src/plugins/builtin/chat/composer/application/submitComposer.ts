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
