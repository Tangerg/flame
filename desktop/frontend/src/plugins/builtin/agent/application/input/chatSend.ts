import type { AgentRunStartOptions } from "@/plugins/sdk";
import { useCallback } from "react";
import { t } from "@/lib/i18n";
import { describeRpcError } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { resolveAgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { OPTIMISTIC_STEER_MESSAGE_PREFIX } from "../view/optimisticMessageIdentity";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionView } from "../ports/sessionView";
import { getActiveSessionId, useActiveSessionId } from "../session/activeSession";
import { selectCurrentRootRun } from "../view/runTree";
import { agentCommandOwner } from "../agentCommandOwner";
import { useCurrentRootMaterial } from "../run/runReadModel";
import { ExactSequence } from "@/foundation/exactSequence";

type SendToAgent = (input: AgentInput, options?: AgentRunStartOptions) => boolean;
export function useChatSend(): (input: AgentInput) => boolean {
  const send = agentSessionView().useAction("send");
  return useCallback(
    (input: AgentInput) => {
      const sessionId = getActiveSessionId();
      const runOptions = resolveAgentRunStartOptions();
      const root = selectCurrentRootRun(agentSessionView().getCurrentView());
      if (root?.status === "running" && sessionId && root.activeSegmentId) {
        if (
          steerRunningTurn({
            sessionId,
            runId: root.id,
            segmentId: root.activeSegmentId,
            input,
          })
        ) {
          return true;
        }
      }
      return sendFreshTurn({ sessionId, send, input, runOptions });
    },
    [send],
  );
}

export function useCanSendToAgent(): boolean {
  const sessionId = useActiveSessionId();
  const send = agentSessionView().useAction("send");
  const root = useCurrentRootMaterial();
  return canAcceptChatInput(sessionId, Boolean(send), root.status);
}

export function canAcceptChatInput(
  sessionId: string,
  mountedSendAvailable: boolean,
  rootStatus: "idle" | "running" | "waiting" | "finished",
): boolean {
  return Boolean(sessionId) && mountedSendAvailable && rootStatus !== "waiting";
}

const steerBubbleIds = new ExactSequence();

interface SteerRunningTurnInput {
  sessionId: string;
  runId: string;
  segmentId: string;
  input: AgentInput;
}

function steerRunningTurn({ sessionId, runId, segmentId, input }: SteerRunningTurnInput): boolean {
  if (input.parts.length === 0) return false;
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const view = agentSessionView();
  const localId = mintSteerBubble(view, sessionId, input);
  const effect = owner.trackEffect(() => view.dropMessage(sessionId, localId));
  void owner.settle(runtime.steerRun(runId, segmentId, input)).then(
    (result) => {
      if (!owner.isCurrent()) return;
      view.reconcileMessageIdentity(sessionId, localId, result.userItemId);
      effect.settle();
    },
    (err: unknown) => {
      if (!owner.isCurrent()) return;
      effect.rollback();
      console.error("[session] steer failed:", err);
      notifyError(describeRpcError(err) ?? t("session.error.steer"), { source: "session" });
    },
  );
  return true;
}

interface SendFreshTurnInput {
  sessionId: string;
  send: SendToAgent | null;
  input: AgentInput;
  runOptions: AgentRunStartOptions;
}

function sendFreshTurn({ sessionId, send, input, runOptions }: SendFreshTurnInput): boolean {
  if (sessionId && send) {
    return send(input, runOptions);
  }
  return false;
}

function mintSteerBubble(
  view: ReturnType<typeof agentSessionView>,
  sessionId: string,
  input: AgentInput,
): string {
  const id = `${OPTIMISTIC_STEER_MESSAGE_PREFIX}${steerBubbleIds.issue()}`;
  view.appendLocalUserMessage(sessionId, id, input);
  return id;
}
