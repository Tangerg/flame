import { ExactSequence } from "@/foundation/exactSequence";
import { t } from "@/lib/i18n";
import { describeRpcError } from "@/lib/rpcErrors";
import { notifyError, notifyInfo } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { agentCommandOwner } from "../agentCommandOwner";
import { agentRuntime, type AgentSteerSubmission } from "../ports/runtimeGateway";
import { agentSessionView } from "../ports/sessionView";
import { synchronizeMountedAgentSession } from "../session/refreshSessionProjection";
import { OPTIMISTIC_STEER_MESSAGE_PREFIX } from "../view/optimisticMessageIdentity";

const steerBubbleIds = new ExactSequence();

export function steerRunningTurn({
  sessionId,
  runId,
  segmentId,
  input,
}: {
  sessionId: string;
  runId: string;
  segmentId: string;
  input: AgentInput;
}): boolean {
  if (input.parts.length === 0) return false;
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const view = agentSessionView();
  const prepared = structuredClone(input);
  const localId = `${OPTIMISTIC_STEER_MESSAGE_PREFIX}${steerBubbleIds.issue()}`;
  view.appendLocalUserMessage(sessionId, localId, prepared);
  const effect = owner.trackEffect(() => view.dropMessage(sessionId, localId));

  function retainUnknown(submission: AgentSteerSubmission): void {
    view.projectSteerReceipt(sessionId, localId, { runId, receipt: "unknown" });
    owner.retainSteerRetry(localId, () => attempt(() => submission.retry(), submission));
    notifyInfo(t("agent.steer.unknown"), { source: "session" });
  }

  function attempt(open: () => AgentSteerSubmission, previous?: AgentSteerSubmission): void {
    view.projectSteerReceipt(sessionId, localId, { runId, receipt: "pending" });
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
    void owner.settle(submission).then(
      (result) => {
        if (!owner.isCurrent()) return;
        const messages = view.getSession(sessionId)?.view.messages;
        if (
          messages &&
          !messages.some((message) => message.id === localId || message.id === result.userItemId)
        ) {
          view.appendLocalUserMessage(sessionId, localId, prepared);
        }
        view.reconcileMessageIdentity(sessionId, localId, result.userItemId, runId);
        effect.settle();
        void synchronizeMountedAgentSession(sessionId, "after-live").catch((error: unknown) => {
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

  attempt(() => runtime.steerRun(runId, segmentId, prepared));
  return true;
}

export function retrySteerInput(messageId: string): boolean {
  return agentCommandOwner().retrySteer(messageId);
}
