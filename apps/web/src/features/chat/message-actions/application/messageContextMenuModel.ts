import { type Message } from "@/features/agent/public/sessionView";
import {
  canCopyMessage,
  canEditMessage,
  canRegenerateMessage,
  canUseMessageRunCheckpoint,
} from "@/features/chat/message-actions/application/messageActionAvailability";
export interface MessageContextMenuCopyState {
  canCopy: boolean;
  plain: string;
}

export interface MessageContextMenuModel {
  copyMarkdown: boolean;
  copyPlain: boolean;
  user: {
    visible: boolean;
    editInComposer: boolean;
    editRerun: boolean;
    editRerunRestore: boolean;
    restore: boolean;
    restoreFiles: boolean;
    restoreBoth: boolean;
    fork: boolean;
  };
  assistant: {
    visible: boolean;
    regenerate: boolean;
    regenerateRestore: boolean;
  };
}

export function messageContextMenuModel({
  msg,
  copy,
  canRestoreFiles,
  conversation,
}: {
  msg: Message;
  copy: MessageContextMenuCopyState;
  canRestoreFiles: boolean;
  conversation: readonly Message[];
}): MessageContextMenuModel {
  const hasRun = Boolean(msg.runId);
  const canEdit = canEditMessage(msg);
  const canUseRunCheckpoint = canUseMessageRunCheckpoint(msg);
  const canRegenerate = canRegenerateMessage(msg, conversation);

  return {
    copyMarkdown: canCopyMessage(copy),
    copyPlain: Boolean(copy.plain),
    user: {
      visible: canEdit || canUseRunCheckpoint,
      editInComposer: canEdit,
      editRerun: canEdit && hasRun,
      editRerunRestore: canEdit && hasRun && canRestoreFiles,
      restore: canUseRunCheckpoint,
      restoreFiles: canUseRunCheckpoint && canRestoreFiles,
      restoreBoth: canUseRunCheckpoint && canRestoreFiles,
      fork: canUseRunCheckpoint,
    },
    assistant: {
      visible: canRegenerate,
      regenerate: canRegenerate,
      regenerateRestore: canRegenerate && canRestoreFiles,
    },
  };
}
