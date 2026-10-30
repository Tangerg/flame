import { type ComposerAccess } from "@/features/composer/public/composer";
import { type AgentAccess } from "@/features/agent/public/agent";
import { type Message } from "@/features/agent/public/sessionView";
import { t } from "@/lib/i18n/index";
import { notifyError, notifyInfo } from "@/features/notifications/public/notifications";
import { agentInputToComposerDraft } from "@/features/chat/message-actions/application/inputBridge";
import { describeRpcError } from "@/lib/rpcErrors";

import { type RestoreType } from "@/features/agent/public/session";
import {
  beginComposerDraftReplacement,
  replaceComposerDraft,
} from "@/features/composer/public/draft";
import {
  messageDraftContent,
  messageHasDraftContent,
} from "@/features/chat/message-actions/application/messageActionContent";
import {
  canEditMessage,
  canForkMessage,
  canRollbackMessage,
  canRegenerateMessage,
} from "@/features/chat/message-actions/application/messageActionAvailability";
export type { RestoreType };

function prefillComposer(composerAccess: ComposerAccess, msg: Message): void {
  replaceComposerDraft(composerAccess, messageDraftContent(msg));
}

function reportRollbackError(err: unknown): void {
  const copy = describeRpcError(err);
  if (!copy) console.error("[message] rollback failed:", err);
  notifyError(copy ?? t("session.error.rollback"), { source: "session" });
}

export interface RollbackActionOptions {
  restoreFiles?: boolean;
}

export function regenerateMessage(
  agentAccess: AgentAccess,
  msg: Message,
  opts?: RollbackActionOptions,
): void {
  const conversation = agentAccess.activeAgentConversation();
  if (!conversation || !canRegenerateMessage(msg, conversation.messages)) return;
  void agentAccess
    .regenerateSessionRun(
      conversation.sessionId,
      msg.runId,
      opts?.restoreFiles ? "both" : "history",
    )
    .then((result) => {
      if (result.status === "noInput" || result.status === "notReady")
        notifyInfo(t(`session.regenerate.${result.status}`), { source: "session" });
    })
    .catch(reportRollbackError);
}

export function editMessageInComposer(composerAccess: ComposerAccess, msg: Message): void {
  if (!messageHasDraftContent(msg)) return;
  prefillComposer(composerAccess, msg);
}

export function editAndRerunMessage(
  agentAccess: AgentAccess,
  composerAccess: ComposerAccess,
  msg: Message,
  opts?: RollbackActionOptions,
): void {
  const conversation = agentAccess.activeAgentConversation();
  if (!conversation || !canEditMessage(msg) || !canRollbackMessage(msg, conversation.messages)) {
    return;
  }
  const replaceDraft = beginComposerDraftReplacement(composerAccess);
  void agentAccess
    .rollbackSessionToBeforeRun(
      conversation.sessionId,
      msg.runId,
      opts?.restoreFiles ? "both" : "history",
    )
    .then((rollback) => {
      if (rollback.status !== "committed") return;
      const draft = rollback.userInput
        ? agentInputToComposerDraft(rollback.userInput)
        : messageDraftContent(msg);
      if (!replaceDraft(draft)) {
        notifyInfo(t("session.editRerun.draftRetained"), { source: "session" });
      }
    })
    .catch(reportRollbackError);
}

export function restoreCheckpoint(
  agentAccess: AgentAccess,
  msg: Message,
  restoreType: RestoreType,
): void {
  const conversation = agentAccess.activeAgentConversation();
  if (!conversation || !canRollbackMessage(msg, conversation.messages)) return;
  void agentAccess
    .rollbackSessionToBeforeRun(conversation.sessionId, msg.runId, restoreType)
    .then((rollback) => {
      if (rollback.status === "committed") {
        notifyInfo(restoreCopy(restoreType), { source: "session" });
      }
    })
    .catch(reportRollbackError);
}

function restoreCopy(restoreType: RestoreType): string {
  switch (restoreType) {
    case "files":
      return t("session.restore.files");
    case "both":
      return t("session.restore.both");
    default:
      return t("session.restore.history");
  }
}

export function forkFromMessage(agentAccess: AgentAccess, msg: Message): void {
  const conversation = agentAccess.activeAgentConversation();
  if (!conversation || !canForkMessage(msg)) return;
  void agentAccess.forkAgentSessionAtRun(conversation.sessionId, msg.runId);
}
