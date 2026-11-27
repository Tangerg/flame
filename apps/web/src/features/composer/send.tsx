import { useComposer } from "@/features/composer/public/composer";
import { iconGlyph } from "@/ui/icons/index";
import { IconButton } from "@flame/ui";
import { useIsCurrentRootRunning, useStopCurrentRootRun } from "@/features/agent/public/run";
import { useT } from "@/lib/i18n/index";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import {
  useComposerAttachmentsReady,
  useComposerImageAttachments,
  useComposerPastes,
  useComposerReferences,
} from "@/features/composer/public/attachments";
import { useComposerText } from "@/features/composer/public/draft";
import { captureComposerFocus } from "@/features/composer/public/focus";
import { composerActionLayout } from "@/features/composer/application/composerActionLayout";
import { hasSendableContent } from "@/features/composer/domain/sendIntent";
import { useSubmitComposer } from "@/features/composer/application/submitComposer";
import { useRuntimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { useCanSendToAgent } from "@/features/agent/public/input";
import { useActiveSessionId } from "@/features/agent/public/session";
function SendButton() {
  const composerAccess = useComposer();

  const t = useT();
  const value = useComposerText();
  const images = useComposerImageAttachments();
  const attachmentsReady = useComposerAttachmentsReady();
  const pastes = useComposerPastes();
  const references = useComposerReferences();
  const submit = useSubmitComposer();
  const deliveryPending = composerAccess.useInputDeliveryPending();
  const stop = useStopCurrentRootRun();
  const running = useIsCurrentRootRunning();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const agentCanSend = useCanSendToAgent();
  const activeSessionId = useActiveSessionId();
  const canSend = runtimeAvailable && agentCanSend && attachmentsReady && !deliveryPending;

  const hasInput = hasSendableContent({ value, images, pastes, references });
  const layout = composerActionLayout({ running, hasInput });

  const stopButton = (primary: boolean) => (
    <IconButton
      icon={iconGlyph("stop")}
      iconSize={16}
      focusableWhenDisabled
      disabled={!stop || !runtimeAvailable}
      aria-label={t("composer.action.stop")}
      onClick={() => {
        const restoreFocus = captureComposerFocus(composerAccess);
        if (stop?.()) restoreFocus();
      }}
      variant={primary ? "primary" : "ghost"}
      round
    />
  );

  const submitButton = (label: string, enabled: boolean) => (
    <IconButton
      icon={iconGlyph("arrow-up")}
      iconSize={16}
      focusableWhenDisabled
      disabled={!enabled}
      aria-busy={deliveryPending || images.some((image) => image.status === "pending")}
      aria-label={label}
      onClick={submit}
      variant="primary"
      round
    />
  );

  return (
    <>
      {layout.submit === "steer" && submitButton(t("composer.action.steer"), canSend)}
      {layout.submit === "send" &&
        submitButton(
          activeSessionId ? t("composer.action.send") : t("composer.folder.required"),
          canSend && hasInput,
        )}
      {layout.stop && stopButton(layout.stop === "emphasized")}
    </>
  );
}

export const composerSend = definePlugin({
  name: "flame.builtin.composer-send",
  setup(ctx) {
    contributeLayout(ctx.contributions, "composer.toolbar.end", {
      id: "send",
      order: 100,
      component: SendButton,
    });
  },
});
