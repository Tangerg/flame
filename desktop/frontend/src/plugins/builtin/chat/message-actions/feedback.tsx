import type { FlameClient } from "@flame/runtime-contract/client";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useT } from "@/lib/i18n";
import { describeRpcError } from "@/lib/rpcErrors";
import {
  contributeLayout,
  definePlugin,
  notifyError,
  useCurrentMessage,
  useCurrentMessageSessionId,
} from "@/plugins/sdk";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import type { FeedbackRating } from "./domain/feedback";
import { canRateMessage } from "./application/messageActionAvailability";
import { useMessageFeedback } from "./public/feedback";
import { installRuntimeFeedbackGateway } from "./adapters/runtimeFeedback";
import { MessageActionButton } from "./MessageActionButton";

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
        title={t("msgActions.good")}
        role={msg.role}
        aria-pressed={feedback.rating === "positive"}
        onClick={() => rate("positive")}
        tone={feedback.rating === "positive" ? "success" : undefined}
      />
      <MessageActionButton
        icon="thumbs-down"
        title={t("msgActions.poor")}
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
