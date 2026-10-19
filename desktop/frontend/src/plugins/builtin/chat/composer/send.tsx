import { iconGlyph } from "@/ui/icons";
import { IconButton } from "@flame/ui";
import { useIsCurrentRootRunning, useStopCurrentRootRun } from "@/plugins/builtin/agent/public/run";
import { useT } from "@/lib/i18n";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { useComposerImages, useComposerPastes } from "./public/attachments";
import { captureComposerSubmission, useComposerText } from "./public/draft";
import { captureComposerFocus } from "./public/focus";
import { composerActionLayout } from "./application/composerActionLayout";
import { submitComposer } from "./application/submitComposer";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { useCanSendToAgent, useChatSend } from "@/plugins/builtin/agent/public/input";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

function SendButton() {
  const t = useT();
  const value = useComposerText();
  const images = useComposerImages();
  const pastes = useComposerPastes();
  const send = useChatSend();
  const stop = useStopCurrentRootRun();
  const running = useIsCurrentRootRunning();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const agentCanSend = useCanSendToAgent();
  const activeSessionId = useActiveSessionId();
  const canSend = runtimeAvailable && agentCanSend;

  const hasInput = Boolean(value.trim()) || images.length > 0 || pastes.length > 0;
  const layout = composerActionLayout({ running, hasInput });
  const submit = () =>
    submitComposer({
      draft: captureComposerSubmission(),
      restoreFocus: captureComposerFocus(),
      sendInput: send,
      canSend: runtimeCommandsAvailable,
    });

  const stopButton = (primary: boolean) => (
    <IconButton
      icon={iconGlyph("stop")}
      iconSize={16}
      disabled={!stop || !runtimeAvailable}
      aria-label={t("composer.action.stop")}
      onClick={() => stop?.()}
      variant={primary ? "primary" : "ghost"}
      round
    />
  );

  const submitButton = (label: string, enabled: boolean) => (
    <IconButton
      icon={iconGlyph("arrow-up")}
      iconSize={16}
      disabled={!enabled}
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
          activeSessionId ? t("composer.action.send") : t("composer.project.required"),
          canSend && hasInput,
        )}
      {layout.stop && stopButton(layout.stop === "emphasized")}
    </>
  );
}

export const composerSend = definePlugin({
  name: "flame.builtin.composer-send",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.end", {
      id: "send",
      order: 100,
      component: SendButton,
    });
  },
});
