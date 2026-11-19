import { useNotifications } from "@/features/notifications/public/notifier";
import { useAgent } from "@/features/agent/public/agent";
import { useComposer } from "@/features/composer/public/composer";
import { captureComposerSelectionQuote } from "@/features/composer/public/selectionQuote";
import { useRuntime } from "@/features/connection/public/runtime";
import * as stylex from "@stylexjs/stylex";
import { type Message } from "@/features/agent/public/sessionView";
import { type ReactElement, type ReactNode, useRef, useState } from "react";
import { ContextMenu } from "@/ui/index";
import { Icon } from "@/ui/index";
import { vocab } from "@flame/ui";
import {
  editAndRerunMessage,
  forkFromMessage,
  regenerateMessage,
  restoreCheckpoint,
} from "@/features/chat/message-actions/public/messageActions";
import { useMessageContext } from "@/features/chat/public/messageContext";
import { messageCopyPayloads } from "@/features/chat/message-actions/public/copyPayloads";
import { messageContextMenuModel } from "@/features/chat/message-actions/public/contextMenu";
import { writeToClipboard } from "@/lib/clipboard";
import { useT } from "@/lib/i18n/index";
import { runtimeCapability } from "@/features/connection/public/capabilities";
import {
  useActiveConversationMessages,
  useHasPrecedingRootRun,
} from "@/features/agent/public/conversation";
interface Props {
  msg: Message;
  label: string;
  children: ReactNode;
}

export function MessageContextMenu({ msg, label, children }: Props) {
  const { messages } = useNotifications();
  const agentAccess = useAgent();
  const composerAccess = useComposer();
  const runtimeAccess = useRuntime();
  const { editInComposer, confirmHistoryEdit } = useMessageContext();

  const t = useT();
  const afterClose = useRef<{ run: (() => void) | undefined } | null>(null);
  const [appendQuote, setAppendQuote] = useState<(() => boolean) | null>(null);
  const copy = messageCopyPayloads(msg);
  const canRestoreFilesBeforeRun =
    useHasPrecedingRootRun(msg.runId) && runtimeCapability(runtimeAccess, "checkpoints");
  const conversation = useActiveConversationMessages();
  const menu = messageContextMenuModel({ msg, copy, canRestoreFilesBeforeRun, conversation });

  return (
    <ContextMenu.Root
      onOpenChange={(open, { event }) => {
        if (!open) return;
        afterClose.current = null;
        const target = event.target;
        const quote =
          target instanceof Node ? captureComposerSelectionQuote(composerAccess, target) : null;
        setAppendQuote(() => quote);
      }}
      onOpenChangeComplete={(open) => {
        if (open) return;
        setAppendQuote(null);
        // Keep the action marker until the menu has settled final focus.
        const run = afterClose.current?.run;
        if (afterClose.current) afterClose.current.run = undefined;
        run?.();
      }}
    >
      <ContextMenu.Trigger render={children as ReactElement} />
      <ContextMenu.Content aria-label={label} finalFocus={() => afterClose.current === null}>
        {appendQuote && (
          <ContextMenu.IconItem
            icon="quote"
            onSelect={() => {
              afterClose.current = { run: appendQuote };
            }}
          >
            {t("quote.selection")}
          </ContextMenu.IconItem>
        )}
        {menu.copyMarkdown && (
          <ContextMenu.IconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(messages.show, copy.markdown || copy.plain, {
                successLabel: t("msgActions.copiedMarkdown"),
              })
            }
          >
            {t("msgActions.copyMarkdown")}
          </ContextMenu.IconItem>
        )}
        {menu.copyPlain && (
          <ContextMenu.IconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(messages.show, copy.plain, {
                successLabel: t("msgActions.copiedPlain"),
              })
            }
          >
            {t("msgActions.copyPlain")}
          </ContextMenu.IconItem>
        )}
        {menu.user.visible && (
          <>
            <Separator />
            {menu.user.editInComposer && (
              <ContextMenu.IconItem
                icon="edit"
                onSelect={() => {
                  afterClose.current = { run: editInComposer };
                }}
              >
                {t("msgActions.editInComposer")}
              </ContextMenu.IconItem>
            )}
            {menu.user.editRerun && (
              <ContextMenu.IconItem
                icon="loop"
                onSelect={() => {
                  afterClose.current = {
                    run: () =>
                      editAndRerunMessage(
                        messages,
                        agentAccess,
                        composerAccess,
                        msg,
                        "history",
                        confirmHistoryEdit,
                      ),
                  };
                }}
              >
                {t("msgActions.editRerun")}
              </ContextMenu.IconItem>
            )}
            {menu.user.editRerunRestore && (
              <ContextMenu.IconItem
                icon="history"
                onSelect={() => {
                  afterClose.current = {
                    run: () =>
                      editAndRerunMessage(
                        messages,
                        agentAccess,
                        composerAccess,
                        msg,
                        "both",
                        confirmHistoryEdit,
                      ),
                  };
                }}
              >
                {t("msgActions.editRerunRestore")}
              </ContextMenu.IconItem>
            )}
            {menu.user.restore && (
              <ContextMenu.SubmenuRoot>
                <ContextMenu.SubmenuTrigger layout="pick">
                  <Icon name="history" size="xs" />
                  <span {...stylex.props(vocab.truncate)}>{t("msgActions.restore")}</span>
                  <Icon
                    name="chevron-right"
                    size="xs"
                    className={stylex.props(vocab.faint).className}
                  />
                </ContextMenu.SubmenuTrigger>
                <ContextMenu.Content side="right" align="start" sideOffset={2} alignOffset={-4}>
                  <ContextMenu.IconItem
                    icon="skip-back"
                    onSelect={() => restoreCheckpoint(messages, agentAccess, msg, "history")}
                  >
                    {t("msgActions.restoreConversation")}
                  </ContextMenu.IconItem>
                  {menu.user.restoreFiles && (
                    <ContextMenu.IconItem
                      icon="folder"
                      onSelect={() => restoreCheckpoint(messages, agentAccess, msg, "files")}
                    >
                      {t("msgActions.restoreFiles")}
                    </ContextMenu.IconItem>
                  )}
                  {menu.user.restoreBoth && (
                    <ContextMenu.IconItem
                      icon="history"
                      onSelect={() => restoreCheckpoint(messages, agentAccess, msg, "both")}
                    >
                      {t("msgActions.restoreBoth")}
                    </ContextMenu.IconItem>
                  )}
                </ContextMenu.Content>
              </ContextMenu.SubmenuRoot>
            )}
            {menu.user.fork && (
              <ContextMenu.IconItem icon="fork" onSelect={() => forkFromMessage(agentAccess, msg)}>
                {t("msgActions.fork")}
              </ContextMenu.IconItem>
            )}
          </>
        )}
        {menu.assistant.visible && (
          <>
            <Separator />
            {menu.assistant.regenerate && (
              <ContextMenu.IconItem
                icon="loop"
                onSelect={() => regenerateMessage(messages, agentAccess, msg)}
              >
                {t("msgActions.regenerate")}
              </ContextMenu.IconItem>
            )}
            {menu.assistant.regenerateRestore && (
              <ContextMenu.IconItem
                icon="history"
                onSelect={() =>
                  regenerateMessage(messages, agentAccess, msg, { restoreFiles: true })
                }
              >
                {t("msgActions.regenerateRestore")}
              </ContextMenu.IconItem>
            )}
          </>
        )}
      </ContextMenu.Content>
    </ContextMenu.Root>
  );
}

function Separator() {
  return <ContextMenu.Separator />;
}
