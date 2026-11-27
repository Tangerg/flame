import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { t } from "@/lib/i18n/index";
import { describeRpcError } from "@/lib/rpcErrors";
import { type AgentInput } from "@/features/agent/domain/input";

import {
  type AgentSteerSubmission,
  type AgentUnresolvedSteer,
} from "@/features/agent/application/runtimeGateway";

import { steerInputFeedback } from "@/features/agent/application/view/steerInputFeedback";
import {
  retiredSteerMessageId,
  steerMessageId,
} from "@/features/agent/application/view/optimisticMessageIdentity";
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
    agentAccess.notifications.notify("error", describeRpcError(error) ?? t("session.error.steer"), {
      source: "session",
    });
    return null;
  }
  return observeSteer(
    agentAccess,
    sessionId,
    {
      runId,
      input: prepared,
      idempotencyKey: submission.idempotencyKey,
      messageId: steerMessageId(submission.idempotencyKey),
      open: () => submission,
    },
    "deliver",
  );
}

function unresolvedSteer(command: AgentUnresolvedSteer): ObservedSteer {
  return {
    runId: command.runId,
    input: command.input,
    idempotencyKey: command.idempotencyKey,
    messageId: steerMessageId(command.idempotencyKey, command.userItemId),
    open: command.retry,
  };
}

export function restoreUnresolvedSteers(
  agentAccess: AgentDependencies,
  sessionId: string,
  commands: AgentUnresolvedSteer[],
): void {
  for (const command of commands) {
    const steer = unresolvedSteer(command);
    if (
      agentAccess.view
        .getSession(sessionId)
        ?.view.messages.some((message) => message.id === steer.messageId)
    )
      continue;
    void observeSteer(agentAccess, sessionId, steer, "awaitRetry");
  }
}

interface ObservedSteer {
  runId: string;
  input: AgentInput;
  idempotencyKey: string;
  messageId: string;
  open: () => AgentSteerSubmission;
}

// A restored steer's delivery stays unknown until the user retries it.
function observeSteer(
  agentAccess: AgentDependencies,
  sessionId: string,
  { runId, input, idempotencyKey, messageId, open }: ObservedSteer,
  start: "deliver" | "awaitRetry",
): Promise<AgentInputSettlement> {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  const view = agentAccess.view;
  const prepared = structuredClone(input);
  let localId = messageId;
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
      agentAccess.notifications.notify("info", t("agent.command.unknown"), { source: "session" });
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
      agentAccess.notifications.notify(
        "error",
        describeRpcError(error) ?? t("session.error.steer"),
        { source: "session" },
      );
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
        agentAccess.notifications.notify(
          "error",
          describeRpcError(error) ?? t("session.error.steer"),
          { source: "session" },
        );
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
          const retiredId = retiredSteerMessageId(result.userItemId);
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
            agentAccess.notifications.notify(
              "error",
              describeRpcError(error) ?? t("session.error.steer"),
              { source: "session" },
            );
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

  if (start === "awaitRetry") {
    view.projectSteerDelivery(sessionId, localId, { runId, delivery: "unknown" });
    owner.retainSteerRetry(localId, () => attempt(open));
    return Promise.resolve({
      status: "unknown",
      idempotencyKey,
      recover: () => attempt(open),
    });
  }
  return attempt(open);
}

export function retrySteerInput(agentAccess: AgentDependencies, messageId: string): boolean {
  return agentAccess.commands.current().retrySteer(messageId);
}
