import type { AgentRunStartOptions } from "@/plugins/sdk";
import { useCallback } from "react";
import { resolveAgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { agentSessionView } from "../ports/sessionView";
import { getActiveSessionId, useActiveSessionId } from "../session/activeSession";
import { selectCurrentRootRun } from "../view/runTree";
import { useCurrentRootMaterial } from "../run/runReadModel";
import { steerRunningTurn } from "./steering";

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
