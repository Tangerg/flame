import { type Message } from "@/features/agent/public/sessionView";
import { t } from "@/lib/i18n/index";
import { notifyError, notifyInfo } from "@/features/notifications/public/notifications";
import { buildInput } from "@/features/composer/public/input";
import { agentInputToComposerDraft } from "@/features/chat/message-actions/application/inputBridge";
import { describeRpcError } from "@/lib/rpcErrors";
import {
  activeAgentConversation,
  forkAgentSessionAtRun,
  rollbackSessionToBeforeRun,
  sendToAgentSession,
} from "@/features/agent/public/session";
import { type RestoreType } from "@/features/agent/public/session";
import {
  beginComposerDraftReplacement,
  replaceComposerDraft,
} from "@/features/composer/public/draft";
import {
  messageDraftContent,
  messageHasDraftContent,
  regenerationPromptBefore,
} from "@/features/chat/message-actions/application/messageActionContent";
import {
  canEditMessage,
  canForkMessage,
  canRollbackMessage,
} from "@/features/chat/message-actions/application/messageActionAvailability";
export type { RestoreType };

function prefillComposer(msg: Message): void {
  replaceComposerDraft(messageDraftContent(msg));
}

function reportRollbackError(err: unknown): void {
  const copy = describeRpcError(err);
  if (!copy) console.error("[message] rollback failed:", err);
  notifyError(copy ?? t("session.error.rollback"), { source: "session" });
}

export interface RollbackActionOptions {
  restoreFiles?: boolean;
}

export function regenerateMessage(msg: Message, opts?: RollbackActionOptions): void {
  const conversation = activeAgentConversation();
  if (!conversation) return;
  const { sessionId, messages } = conversation;
  const prompt = regenerationPromptBefore(messages, msg.id);
  if (!prompt) return;
  if (!prompt.runId) {
    sendToAgentSession(sessionId, buildInput(prompt.text, prompt.images));
    return;
  }
  void rollbackSessionToBeforeRun(sessionId, prompt.runId, opts?.restoreFiles ? "both" : "history")
    .then((rollback) => {
      if (rollback.status === "inFlight") return;
      const input =
        rollback.status === "committed" && rollback.userInput
          ? rollback.userInput
          : buildInput(prompt.text, prompt.images);
      if (!sendToAgentSession(sessionId, input)) {
        notifyInfo(t("session.regenerate.switchedAway"), {
          source: "session",
        });
      }
    })
    .catch(reportRollbackError);
}

export function editMessageInComposer(msg: Message): void {
  if (!messageHasDraftContent(msg)) return;
  prefillComposer(msg);
}

export function editAndRerunMessage(msg: Message, opts?: RollbackActionOptions): void {
  const conversation = activeAgentConversation();
  if (!conversation || !canEditMessage(msg) || !canRollbackMessage(msg, conversation.messages)) {
    return;
  }
  const replaceDraft = beginComposerDraftReplacement();
  void rollbackSessionToBeforeRun(
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

export function restoreCheckpoint(msg: Message, restoreType: RestoreType): void {
  const conversation = activeAgentConversation();
  if (!conversation || !canRollbackMessage(msg, conversation.messages)) return;
  void rollbackSessionToBeforeRun(conversation.sessionId, msg.runId, restoreType)
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

export function forkFromMessage(msg: Message): void {
  const conversation = activeAgentConversation();
  if (!conversation || !canForkMessage(msg)) return;
  void forkAgentSessionAtRun(conversation.sessionId, msg.runId);
}
