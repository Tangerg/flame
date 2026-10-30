import { buildInput } from "@/features/composer/public/input";
import { type ComposerSubmission } from "@/features/composer/application/state";
import { agentTextInput, type AgentInput } from "@/features/agent/public/input";
import { createComposerSendIntent } from "@/features/composer/domain/sendIntent";
import {
  COMPOSER_SUBMIT_MODE,
  lookupSlashCommandOwner,
  SLASH_COMMAND,
} from "@/features/composer/public/extensionPoints";
import {
  lookupExtensionByKey,
  lookupExtensionOwner,
  lookupExtensionPoint,
} from "@/extensions/selectors/extensions";
import { reportPluginError } from "@/extensions/errors";
export interface SubmitDeps {
  draft: ComposerSubmission;
  sendInput: (input: AgentInput) => boolean;
  canSend: () => boolean;
  restoreFocus: () => void;
}

export function submitComposer({ draft, sendInput, canSend, restoreFocus }: SubmitDeps): void {
  if (!draft.attachmentsReady) return;
  const { value, images, pastes } = draft;
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
  let settled = false;
  const consume = (record: boolean) => {
    if (settled) return;
    settled = true;
    if (draft.consume(record ? intent.historyText || undefined : undefined)) restoreFocus();
  };
  const accept = () => consume(true);

  for (const mode of lookupExtensionPoint(COMPOSER_SUBMIT_MODE)) {
    const owner = lookupExtensionOwner(COMPOSER_SUBMIT_MODE, mode.id) ?? "unknown";
    let matches = false;
    try {
      matches = mode.matches(modeDraft);
    } catch (error) {
      reportPluginError(owner, "command", error, `composer.submitMode:${mode.id}`);
      return;
    }
    if (!matches) continue;

    const report = (error: unknown) =>
      reportPluginError(owner, "command", error, `composer.submitMode:${mode.id}`);
    try {
      void Promise.resolve(
        mode.submit({ ...modeDraft, accept, clear: () => consume(false) }),
      ).catch(report);
    } catch (error) {
      report(error);
    }
    return;
  }

  const slash = intent.slash;
  if (slash) {
    const spec = lookupExtensionByKey(SLASH_COMMAND, slash.cmd);
    if (spec?.run) {
      const owner = lookupSlashCommandOwner(slash.cmd) ?? "unknown";
      const report = (err: unknown) => {
        reportPluginError(owner, "command", err, `command: ${slash.cmd}`);
      };
      try {
        void Promise.resolve(
          spec.run({
            args: slash.args,
            accept,
            send: (text) => {
              if (!canSend() || !sendInput(agentTextInput(text))) return false;
              accept();
              return true;
            },
          }),
        ).catch(report);
      } catch (error) {
        report(error);
      }
      return;
    }
  }
  if (!canSend()) return;
  if (!sendInput(buildInput(intent.body, images))) return;
  accept();
}
