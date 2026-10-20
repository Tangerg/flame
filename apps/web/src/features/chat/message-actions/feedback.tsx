import { type FlameClient } from "@flame/runtime-client";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useT } from "@/lib/i18n/index";
import { describeRpcError } from "@/lib/rpcErrors";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { notifyError } from "@/features/notifications/public/notifications";
import {
  useCurrentMessage,
  useCurrentMessageSessionId,
} from "@/features/chat/public/messageContext";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";
import { type Message } from "@/features/agent/public/sessionView";
import { type FeedbackRating } from "@/features/chat/message-actions/domain/feedback";
import { canRateMessage } from "@/features/chat/message-actions/application/messageActionAvailability";
import { useMessageFeedback } from "@/features/chat/message-actions/public/feedback";
import { installRuntimeFeedbackGateway } from "@/features/chat/message-actions/adapters/runtimeFeedback";
import { MessageActionButton } from "@/features/chat/message-actions/MessageActionButton";
function FeedbackButtons() {
  const msg = useCurrentMessage();
  if (!canRateMessage(msg)) return null;
  return <RateableFeedbackButtons msg={msg} />;
}

function RateableFeedbackButtons({ msg }: { msg: Message }) {
  const t = useT();
  const sessionId = useCurrentMessageSessionId();
  const feedback = useMessageFeedback(sessionId, msg);

  const rate = (rating: FeedbackRating): void => {
    if (feedback.rating === rating) return;
    void feedback.submit(rating).catch((error: unknown) => {
      if (wasGenerationRetired(error)) return;
      notifyError(describeRpcError(error) ?? t("msgActions.feedbackFailed"), { source: "session" });
    });
  };

  return (
    <>
      <MessageActionButton
        icon="thumbs-up"
        aria-label={t("msgActions.good")}
        role={msg.role}
        aria-pressed={feedback.rating === "positive"}
        onClick={() => rate("positive")}
        tone={feedback.rating === "positive" ? "success" : undefined}
      />
      <MessageActionButton
        icon="thumbs-down"
        aria-label={t("msgActions.poor")}
        role={msg.role}
        aria-pressed={feedback.rating === "negative"}
        onClick={() => rate("negative")}
        tone={feedback.rating === "negative" ? "negative" : undefined}
      />
    </>
  );
}

export function createMessageFeedbackPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.message-feedback",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      const gateway = installRuntimeFeedbackGateway(runtimeClient);
      ctx.cleanup(() => gateway.dispose());
      ctx.cleanup(followRuntimeGeneration(ctx.runtime, () => gateway.replaceRuntimeGeneration()));
      contributeLayout(ctx, "message.actions", {
        id: "feedback",
        order: 15,
        component: FeedbackButtons,
      });
    },
  });
}
