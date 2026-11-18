import { useT } from "@/lib/i18n/index";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { useCurrentMessage, useEditCurrentMessage } from "@/features/chat/public/messageContext";
import { canEditMessage } from "@/features/chat/message-actions/application/messageActionAvailability";
import { MessageActionButton } from "@/features/chat/message-actions/MessageActionButton";
function EditButton() {
  const t = useT();
  const msg = useCurrentMessage();
  const edit = useEditCurrentMessage();
  if (!canEditMessage(msg)) return null;

  return (
    <MessageActionButton
      icon="edit"
      aria-label={t("msgActions.edit")}
      role={msg.role}
      onClick={(event) => edit(event.currentTarget)}
    />
  );
}

export const messageEdit = definePlugin({
  name: "flame.builtin.message-edit",
  setup(ctx) {
    contributeLayout(ctx.contributions, "message.actions", {
      id: "edit",
      order: 5,
      component: EditButton,
    });
  },
});
