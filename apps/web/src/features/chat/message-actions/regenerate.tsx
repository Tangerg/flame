import { useT } from "@/lib/i18n/index";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { useCurrentMessage } from "@/features/chat/public/messageContext";
import { canRegenerateMessage } from "@/features/chat/message-actions/application/messageActionAvailability";
import { regenerateMessage } from "@/features/chat/message-actions/public/messageActions";
import { MessageActionButton } from "@/features/chat/message-actions/MessageActionButton";
import { useActiveConversationMessages } from "@/features/agent/public/conversation";
function RegenerateButton() {
  const t = useT();
  const msg = useCurrentMessage();
  const messages = useActiveConversationMessages();
  if (!canRegenerateMessage(msg, messages)) return null;

  return (
    <MessageActionButton
      icon="loop"
      aria-label={t("msgActions.regenerate")}
      role={msg.role}
      onClick={() => regenerateMessage(msg)}
    />
  );
}

export const messageRegenerate = definePlugin({
  name: "flame.builtin.message-regenerate",
  setup(ctx) {
    contributeLayout(ctx, "message.actions", {
      id: "regenerate",
      order: 10,
      component: RegenerateButton,
    });
  },
});
