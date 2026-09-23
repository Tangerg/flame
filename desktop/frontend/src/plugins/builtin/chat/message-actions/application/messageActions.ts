import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { t } from "@/lib/i18n";
import { notifyError, notifyInfo } from "@/plugins/sdk";
import { buildInput } from "@/plugins/builtin/chat/composer/public/input";
import { agentInputToComposerDraft } from "./inputBridge";
import { describeRpcError } from "@/lib/rpcErrors";
import {
  activeAgentConversation,
  forkAgentSessionAtRun,
  rollbackSessionToBeforeRun,
  sendToAgentSession,
  type RestoreType,
} from "@/plugins/builtin/agent/public/session";
import { replaceComposerDraft } from "@/plugins/builtin/chat/composer/public/draft";
import {
  messageDraftContent,
  messageHasDraftContent,
  regenerationPromptBefore,
} from "./messageActionContent";

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
  if (!conversation || !messageHasDraftContent(msg)) return;
  if (msg.role !== "user" || !msg.runId) {
    prefillComposer(msg);
    return;
  }
  void rollbackSessionToBeforeRun(
    conversation.sessionId,
    msg.runId,
    opts?.restoreFiles ? "both" : "history",
  )
    .then((rollback) => {
      if (rollback.status === "inFlight") return;
      if (rollback.status === "committed" && rollback.userInput) {
        replaceComposerDraft(agentInputToComposerDraft(rollback.userInput));
        return;
      }
      prefillComposer(msg);
    })
    .catch(reportRollbackError);
}

export function restoreCheckpoint(msg: Message, restoreType: RestoreType): void {
  const conversation = activeAgentConversation();
  if (!conversation || msg.role !== "user" || !msg.runId) return;
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
  if (!conversation || !msg.runId) return;
  void forkAgentSessionAtRun(conversation.sessionId, msg.runId);
}
