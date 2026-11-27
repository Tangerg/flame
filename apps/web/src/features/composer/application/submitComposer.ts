import { buildInput } from "@/features/composer/public/input";
import { type ComposerSubmission } from "@/features/composer/application/state";
import {
  agentTextInput,
  type AgentInput,
  type AgentInputSettlement,
  type SubmitChatInput,
  useSubmitChatInput,
} from "@/features/agent/public/input";
import { useComposer } from "@/features/composer/public/composer";
import { useRuntime } from "@/features/connection/public/runtime";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { useCallback } from "react";
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
interface SubmitDeps {
  draft: ComposerSubmission;
  sendInput: SubmitChatInput;
  canSend: () => boolean;
  restoreFocus: () => void;
}

export function useSubmitComposer(): () => void {
  const composer = useComposer();
  const runtime = useRuntime();
  const sendInput = useSubmitChatInput();
  return useCallback(
    () =>
      submitComposer({
        draft: composer.captureSubmission(),
        restoreFocus: composer.captureFocus(),
        sendInput,
        canSend: () => runtimeCommandsAvailable(runtime),
      }),
    [composer, runtime, sendInput],
  );
}

function submitComposer({ draft, sendInput, canSend, restoreFocus }: SubmitDeps): void {
  if (!draft.attachmentsReady) return;
  const { value, images, pastes, references } = draft;
  const intent = createComposerSendIntent({ value, images, pastes, references });
  if (!intent.shouldSend) return;

  const modeDraft = {
    rawText: value,
    text: intent.text,
    body: intent.body,
    slash: intent.slash ? { command: intent.slash.cmd, args: intent.slash.args } : null,
    hasImages: images.length > 0,
    hasPastes: pastes.length > 0,
    hasReferences: references.length > 0,
  };
  let settled = false;
  const consume = (history: "record" | "skip") => {
    if (settled) return;
    settled = true;
    if (draft.consume(history === "record" ? intent.historyText || undefined : undefined))
      restoreFocus();
  };
  const accept = () => consume("record");
  const dispatch = (input: AgentInput): boolean => {
    if (!canSend()) return false;
    const delivery = draft.claimInputDelivery();
    if (!delivery) return false;
    let pending: Promise<AgentInputSettlement> | null = null;
    try {
      pending = sendInput(delivery.sessionId, input);
      if (!pending) return false;
      // A known refusal keeps the original editable draft. Uncertain delivery
      // leaves the command journal responsible for its captured input.
      void pending.then(accept, restoreFocus).finally(delivery.release);
      return true;
    } finally {
      if (!pending) delivery.release();
    }
  };

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
        mode.submit({ ...modeDraft, accept, clear: () => consume("skip") }),
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
            send: (text) => dispatch(agentTextInput(text)),
          }),
        ).catch(report);
      } catch (error) {
        report(error);
      }
      return;
    }
  }
  dispatch(buildInput(intent.body, images, references));
}
