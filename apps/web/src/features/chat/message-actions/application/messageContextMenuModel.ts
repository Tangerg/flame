import { type Message } from "@/features/agent/public/sessionView";
import {
  canCopyMessage,
  canEditMessage,
  canForkMessage,
  canRegenerateMessage,
  canRollbackMessage,
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
  canRestoreFilesBeforeRun,
  conversation,
}: {
  msg: Message;
  copy: MessageContextMenuCopyState;
  canRestoreFilesBeforeRun: boolean;
  conversation: readonly Message[];
}): MessageContextMenuModel {
  const canEdit = canEditMessage(msg);
  const canRollback = canRollbackMessage(msg, conversation);
  const canFork = canForkMessage(msg);
  const canRegenerate = canRegenerateMessage(msg, conversation);

  return {
    copyMarkdown: canCopyMessage(copy),
    copyPlain: Boolean(copy.plain),
    user: {
      visible: canEdit || canRollback || canFork,
      editInComposer: canEdit,
      editRerun: canEdit && canRollback,
      editRerunRestore: canEdit && canRollback && canRestoreFilesBeforeRun,
      restore: canRollback,
      restoreFiles: canRollback && canRestoreFilesBeforeRun,
      restoreBoth: canRollback && canRestoreFilesBeforeRun,
      fork: canFork,
    },
    assistant: {
      visible: canRegenerate,
      regenerate: canRegenerate,
      regenerateRestore: canRegenerate && canRestoreFilesBeforeRun,
    },
  };
}
