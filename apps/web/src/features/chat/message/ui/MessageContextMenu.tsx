import { useAgent } from "@/features/agent/public/agent";
import { useComposer } from "@/features/composer/public/composer";
import { useRuntime } from "@/features/connection/public/runtime";
import * as stylex from "@stylexjs/stylex";
import { type Message } from "@/features/agent/public/sessionView";
import { type ReactElement, type ReactNode, useRef } from "react";
import { ContextMenu } from "@/ui/index";
import { Icon } from "@/ui/index";
import { vocab } from "@flame/ui";
import {
  editAndRerunMessage,
  editMessageInComposer,
  forkFromMessage,
  regenerateMessage,
  restoreCheckpoint,
} from "@/features/chat/message-actions/public/messageActions";
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
  const agentAccess = useAgent();
  const composerAccess = useComposer();
  const runtimeAccess = useRuntime();

  const t = useT();
  const selection = useRef<{ edit: (() => void) | undefined } | null>(null);
  const copy = messageCopyPayloads(msg);
  const canRestoreFilesBeforeRun =
    useHasPrecedingRootRun(msg.runId) && runtimeCapability(runtimeAccess, "checkpoints");
  const conversation = useActiveConversationMessages();
  const menu = messageContextMenuModel({ msg, copy, canRestoreFilesBeforeRun, conversation });

  return (
    <ContextMenu.Root
      onOpenChange={(open) => {
        if (open) selection.current = null;
      }}
      onOpenChangeComplete={(open) => {
        if (open) return;
        // Keep the selection marker until the menu has settled final focus.
        const edit = selection.current?.edit;
        if (selection.current) selection.current.edit = undefined;
        edit?.();
      }}
    >
      <ContextMenu.Trigger render={children as ReactElement} />
      <ContextMenu.Content aria-label={label} finalFocus={() => selection.current === null}>
        {menu.copyMarkdown && (
          <ContextMenu.IconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(copy.markdown || copy.plain, {
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
              void writeToClipboard(copy.plain, { successLabel: t("msgActions.copiedPlain") })
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
                  selection.current = { edit: () => editMessageInComposer(composerAccess, msg) };
                }}
              >
                {t("msgActions.editInComposer")}
              </ContextMenu.IconItem>
            )}
            {menu.user.editRerun && (
              <ContextMenu.IconItem
                icon="loop"
                onSelect={() => {
                  selection.current = {
                    edit: () => editAndRerunMessage(agentAccess, composerAccess, msg),
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
                  selection.current = {
                    edit: () =>
                      editAndRerunMessage(agentAccess, composerAccess, msg, { restoreFiles: true }),
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
                    onSelect={() => restoreCheckpoint(agentAccess, msg, "history")}
                  >
                    {t("msgActions.restoreConversation")}
                  </ContextMenu.IconItem>
                  {menu.user.restoreFiles && (
                    <ContextMenu.IconItem
                      icon="folder"
                      onSelect={() => restoreCheckpoint(agentAccess, msg, "files")}
                    >
                      {t("msgActions.restoreFiles")}
                    </ContextMenu.IconItem>
                  )}
                  {menu.user.restoreBoth && (
                    <ContextMenu.IconItem
                      icon="history"
                      onSelect={() => restoreCheckpoint(agentAccess, msg, "both")}
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
                onSelect={() => regenerateMessage(agentAccess, msg)}
              >
                {t("msgActions.regenerate")}
              </ContextMenu.IconItem>
            )}
            {menu.assistant.regenerateRestore && (
              <ContextMenu.IconItem
                icon="history"
                onSelect={() => regenerateMessage(agentAccess, msg, { restoreFiles: true })}
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
