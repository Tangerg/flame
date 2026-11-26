import { useNotifications } from "@/features/notifications/public/notifier";
import * as stylex from "@stylexjs/stylex";
import { DropdownMenu } from "@/ui/index";
import { Tooltip, vocab } from "@flame/ui";
import { writeToClipboard } from "@/lib/clipboard";
import { useT } from "@/lib/i18n/index";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { useCurrentMessage } from "@/features/chat/public/messageContext";
import { canCopyMessage } from "@/features/chat/message-actions/application/messageActionAvailability";
import { messageCopyPayloads } from "@/features/chat/message-actions/presentation/copyPayloads";
import { MessageActionButton } from "@/features/chat/message-actions/MessageActionButton";
function CopyButton() {
  const { messages } = useNotifications();
  const t = useT();
  const msg = useCurrentMessage();
  const copy = messageCopyPayloads(msg);
  if (!canCopyMessage(copy)) return null;

  return (
    <DropdownMenu.Root>
      <Tooltip label={t("msgActions.copy")}>
        <DropdownMenu.Trigger
          render={
            <MessageActionButton icon="copy" role={msg.role} aria-label={t("msgActions.copy")} />
          }
        />
      </Tooltip>
      <DropdownMenu.Content align="end" sideOffset={4}>
        <CopyItem
          label={t("msgActions.copyMarkdown")}
          onSelect={() =>
            writeToClipboard(messages.show, copy.markdown || copy.plain, {
              successLabel: t("msgActions.copiedMarkdown"),
            })
          }
        />
        <CopyItem
          label={t("msgActions.copyPlain")}
          onSelect={() =>
            writeToClipboard(messages.show, copy.plain, {
              successLabel: t("msgActions.copiedPlain"),
            })
          }
        />
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

function CopyItem({ label, onSelect }: { label: string; onSelect: () => void }) {
  return (
    <DropdownMenu.Item layout="pickPlain" onClick={onSelect}>
      <span {...stylex.props(vocab.truncate)}>{label}</span>
    </DropdownMenu.Item>
  );
}

export const messageCopy = definePlugin({
  name: "flame.builtin.message-copy",
  setup(ctx) {
    contributeLayout(ctx.contributions, "message.actions", {
      id: "copy",
      order: 0,
      component: CopyButton,
    });
  },
});
