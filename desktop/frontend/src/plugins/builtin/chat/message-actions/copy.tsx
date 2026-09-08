import * as stylex from "@stylexjs/stylex";
import { DropdownMenu, Tooltip, vocab } from "@/ui";
import { writeToClipboard } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { contributeLayout, definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { canCopyMessage } from "./application/messageActionAvailability";
import { messageCopyPayloads } from "./presentation/copyPayloads";
import { MessageActionButton } from "./MessageActionButton";
import { type as typeStep } from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../chatStyles";

function CopyButton() {
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
          hint={t("msgActions.copyMarkdownHint")}
          onSelect={() =>
            writeToClipboard(copy.markdown || copy.plain, {
              successLabel: t("msgActions.copiedMarkdown"),
            })
          }
        />
        <CopyItem
          label={t("msgActions.copyPlain")}
          hint={t("msgActions.copyPlainHint")}
          onSelect={() =>
            writeToClipboard(copy.plain, { successLabel: t("msgActions.copiedPlain") })
          }
        />
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

function CopyItem({
  label,
  hint,
  onSelect,
}: {
  label: string;
  hint: string;
  onSelect: () => void;
}) {
  return (
    <DropdownMenu.Item onClick={onSelect} className={stylex.props(ct.panelRow).className}>
      <span {...stylex.props(vocab.ink, typeStep.uiMd)}>{label}</span>
      <span {...stylex.props(vocab.faint, typeStep.uiSm)}>{hint}</span>
    </DropdownMenu.Item>
  );
}

export const messageCopy = definePlugin({
  name: "flame.builtin.message-copy",
  setup(ctx) {
    contributeLayout(ctx, "message.actions", { id: "copy", order: 0, component: CopyButton });
  },
});
