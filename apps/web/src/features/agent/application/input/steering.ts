import { type AgentAccess } from "@/features/agent/public/agent";
import { t } from "@/lib/i18n/index";
import { describeRpcError } from "@/lib/rpcErrors";
import { notifyError, notifyInfo } from "@/features/notifications/public/notifications";
import { type AgentInput } from "@/features/agent/domain/input";

import {
  type AgentSteerSubmission,
  type AgentUnresolvedSteer,
} from "@/features/agent/application/runtimeGateway";

import { steerInputFeedback } from "@/features/agent/application/view/steerInputFeedback";
import { OPTIMISTIC_STEER_MESSAGE_PREFIX } from "@/features/agent/application/view/optimisticMessageIdentity";
export function steerRunningTurn(
  agentAccess: AgentAccess,
  {
    sessionId,
    runId,
    segmentId,
    input,
  }: {
    sessionId: string;
    runId: string;
    segmentId: string;
    input: AgentInput;
  },
): boolean {
  if (input.parts.length === 0) return false;
  const prepared = structuredClone(input);
  let submission: AgentSteerSubmission;
  try {
    submission = agentAccess.runtime.steerRun(runId, segmentId, prepared);
  } catch (error) {
    notifyError(describeRpcError(error) ?? t("session.error.steer"), { source: "session" });
    return false;
  }
  observeSteer(
    agentAccess,
    sessionId,
    runId,
    prepared,
    submission.idempotencyKey,
    () => submission,
  );
  return true;
}

export function restoreUnresolvedSteers(
  agentAccess: AgentAccess,
  sessionId: string,
  commands: AgentUnresolvedSteer[],
): void {
  for (const command of commands) {
    const localId =
      command.userItemId ?? `${OPTIMISTIC_STEER_MESSAGE_PREFIX}${command.idempotencyKey}`;
    if (
      agentAccess.view
        .getSession(sessionId)
        ?.view.messages.some((message) => message.id === localId)
    )
      continue;
    observeSteer(
      agentAccess,
      sessionId,
      command.runId,
      command.input,
      command.idempotencyKey,
      command.retry,
      true,
      command.userItemId,
    );
  }
}

function observeSteer(
  agentAccess: AgentAccess,
  sessionId: string,
  runId: string,
  input: AgentInput,
  key: string,
  open: () => AgentSteerSubmission,
  restored = false,
  nominatedId: string | null = null,
): void {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  const view = agentAccess.view;
  const prepared = structuredClone(input);
  let localId = nominatedId ?? `${OPTIMISTIC_STEER_MESSAGE_PREFIX}${key}`;
  if (!view.getSession(sessionId)?.view.messages.some((message) => message.id === localId))
    view.appendLocalUserMessage(sessionId, localId, prepared);
  const clearProvisional = () => {
    const message = view
      .getSession(sessionId)
      ?.view.messages.find((message) => message.id === localId);
    if (!message?.steer?.delivery) return;
    if (message.runId === null && !message.steer.receipt) view.dropMessage(sessionId, localId);
    else view.projectSteerDelivery(sessionId, localId, { runId, delivery: undefined });
  };
  const effect = owner.trackEffect(clearProvisional);

  function retainUnknown(submission: AgentSteerSubmission): void {
    view.projectSteerDelivery(sessionId, localId, { runId, delivery: "unknown" });
    owner.retainSteerRetry(localId, () => attempt(() => submission.retry(), submission));
    const current = view.getSession(sessionId)?.view;
    const message = current?.messages.find((input) => input.id === localId);
    if (message && steerInputFeedback(message) === "unknown")
      notifyInfo(t("agent.command.unknown"), { source: "session" });
  }

  function attempt(open: () => AgentSteerSubmission, previous?: AgentSteerSubmission): void {
    view.projectSteerDelivery(sessionId, localId, { runId, delivery: "pending" });
    let submission: AgentSteerSubmission;
    try {
      submission = open();
    } catch (error) {
      if (previous && runtime.isSteerReceiptUnknown(error)) {
        retainUnknown(previous);
        return;
      }
      effect.rollback();
      notifyError(describeRpcError(error) ?? t("session.error.steer"), { source: "session" });
      return;
    }
    void submission.inputIdentity
      .then(
        (userItemId) => {
          if (!owner.isCurrent()) return;
          view.reconcileMessageIdentity(sessionId, localId, userItemId);
          localId = userItemId;
        },
        () => {},
      )
      .catch((error: unknown) => {
        notifyError(describeRpcError(error) ?? t("session.error.steer"), { source: "session" });
      });
    void owner.settle(submission).then(
      (result) => {
        if (!owner.isCurrent()) return;
        if (result.type === "retired") {
          clearProvisional();
          const retiredId = `retired-steer:${result.userItemId}`;
          view.appendLocalUserMessage(sessionId, retiredId, prepared);
          view.projectSteerReceipt(sessionId, retiredId, { runId, receipt: "retired" });
          effect.settle();
        } else {
          const messages = view.getSession(sessionId)?.view.messages;
          if (
            messages &&
            !messages.some((message) => message.id === localId || message.id === result.userItemId)
          ) {
            view.appendLocalUserMessage(sessionId, localId, prepared);
          }
          view.reconcileMessageIdentity(sessionId, localId, result.userItemId, {
            runId,
            receipt: "accepted",
          });
          effect.settle();
        }
        void view
          .getSession(sessionId)
          ?.synchronize?.("after-live")
          .catch((error: unknown) => {
            console.warn("[session] steer reconciliation failed:", sessionId, error);
          });
      },
      (error: unknown) => {
        if (!owner.isCurrent()) return;
        if (runtime.isSteerReceiptUnknown(error)) {
          retainUnknown(submission);
          return;
        }
        effect.rollback();
        notifyError(describeRpcError(error) ?? t("session.error.steer"), { source: "session" });
      },
    );
  }

  if (restored) {
    view.projectSteerDelivery(sessionId, localId, { runId, delivery: "unknown" });
    owner.retainSteerRetry(localId, () => attempt(open));
  } else attempt(open);
}

export function retrySteerInput(agentAccess: AgentAccess, messageId: string): boolean {
  return agentAccess.commands.current().retrySteer(messageId);
}
