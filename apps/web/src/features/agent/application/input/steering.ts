import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
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
import {
  settleAgentInput,
  type AgentInputSettlement,
  type AgentOpeningAcceptance,
} from "@/features/agent/application/input/inputSubmission";
export function steerRunningTurn(
  agentAccess: AgentDependencies,
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
): Promise<AgentInputSettlement> | null {
  if (input.parts.length === 0) return null;
  agentAccess.commands.current().assertCurrent();
  const prepared = structuredClone(input);
  let submission: AgentSteerSubmission;
  try {
    submission = agentAccess.runtime.steerRun(runId, segmentId, prepared);
  } catch (error) {
    notifyError(describeRpcError(error) ?? t("session.error.steer"), { source: "session" });
    return null;
  }
  return observeSteer(
    agentAccess,
    sessionId,
    runId,
    prepared,
    submission.idempotencyKey,
    () => submission,
  );
}

export function restoreUnresolvedSteers(
  agentAccess: AgentDependencies,
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
  agentAccess: AgentDependencies,
  sessionId: string,
  runId: string,
  input: AgentInput,
  key: string,
  open: () => AgentSteerSubmission,
  restored = false,
  nominatedId: string | null = null,
): Promise<AgentInputSettlement> {
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

  function attempt(
    open: () => AgentSteerSubmission,
    previous?: AgentSteerSubmission,
  ): Promise<AgentInputSettlement> {
    owner.assertCurrent();
    view.projectSteerDelivery(sessionId, localId, { runId, delivery: "pending" });
    let submission: AgentSteerSubmission;
    try {
      submission = open();
    } catch (error) {
      if (previous && runtime.isInputReceiptUnknown(error)) {
        retainUnknown(previous);
        return Promise.resolve({
          status: "unknown",
          idempotencyKey: previous.idempotencyKey,
          recover: () => attempt(() => previous.retry(), previous),
        });
      }
      effect.rollback();
      notifyError(describeRpcError(error) ?? t("session.error.steer"), { source: "session" });
      const rejected = Promise.reject<AgentInputSettlement>(error);
      void rejected.catch(() => {});
      return rejected;
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
    const acceptance = owner.settle(submission).then(
      (result): AgentOpeningAcceptance => {
        const accepted: AgentOpeningAcceptance = {
          status: result.type,
          sessionId,
          runId,
          userItemId: result.userItemId,
        };
        if (!owner.isCurrent()) return accepted;
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
        return accepted;
      },
      (error: unknown) => {
        if (owner.isCurrent()) {
          if (runtime.isInputReceiptUnknown(error)) retainUnknown(submission);
          else {
            effect.rollback();
            notifyError(describeRpcError(error) ?? t("session.error.steer"), { source: "session" });
          }
        }
        throw error;
      },
    );
    return settleAgentInput(acceptance, {
      key: () => submission.idempotencyKey,
      isUnknown: (error) => runtime.isInputReceiptUnknown(error),
      recover: () => attempt(() => submission.retry(), submission),
    });
  }

  if (restored) {
    view.projectSteerDelivery(sessionId, localId, { runId, delivery: "unknown" });
    owner.retainSteerRetry(localId, () => attempt(open));
    return Promise.resolve({
      status: "unknown",
      idempotencyKey: key,
      recover: () => attempt(open),
    });
  }
  return attempt(open);
}

export function recoverUnresolvedSteer(
  agentAccess: AgentDependencies,
  sessionId: string,
  command: AgentUnresolvedSteer,
): Promise<AgentInputSettlement> {
  return observeSteer(
    agentAccess,
    sessionId,
    command.runId,
    command.input,
    command.idempotencyKey,
    command.retry,
    false,
    command.userItemId,
  );
}

export function retrySteerInput(agentAccess: AgentDependencies, messageId: string): boolean {
  return agentAccess.commands.current().retrySteer(messageId);
}
