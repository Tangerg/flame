import { buildInput } from "@/features/composer/public/input";
import { type ComposerSubmission } from "@/features/composer/application/state";
import {
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
  lookupSlashCommandOwner,
  SLASH_COMMAND,
} from "@/features/composer/public/extensionPoints";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
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

  let settled = false;
  const accept = () => {
    if (settled) return;
    settled = true;
    if (draft.consume(intent.historyText ?? undefined)) restoreFocus();
  };
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

  const slash = intent.slash;
  if (slash) {
    const spec = lookupExtensionByKey(SLASH_COMMAND, slash.command);
    if (spec?.run) {
      const owner = lookupSlashCommandOwner(slash.command) ?? "unknown";
      const report = (err: unknown) => {
        reportPluginError(owner, "command", err, `command: ${slash.command}`);
      };
      try {
        void Promise.resolve(
          spec.run({
            args: slash.args,
            hasAttachments: images.length > 0 || pastes.length > 0 || references.length > 0,
            accept,
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
