import { useAgent } from "@/features/agent/public/agent";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import { useCallback } from "react";
import { resolveAgentRunStartOptions } from "@/features/agent/public/runOptions";
import { type AgentInput } from "@/features/agent/domain/input";

import {
  getActiveSessionId,
  useActiveSessionId,
} from "@/features/agent/application/session/activeSession";
import { selectCurrentRootRun } from "@/features/agent/application/view/runTree";
import { useCurrentRootMaterial } from "@/features/agent/application/run/runReadModel";
import { steerRunningTurn } from "@/features/agent/application/input/steering";
type SendToAgent = (input: AgentInput, options?: AgentRunStartOptions) => boolean;

export function useChatSend(): (input: AgentInput) => boolean {
  const agentAccess = useAgent();

  const send = agentAccess.view.useAction("send");
  return useCallback(
    (input: AgentInput) => {
      const sessionId = getActiveSessionId(agentAccess);
      const runOptions = resolveAgentRunStartOptions();
      const root = selectCurrentRootRun(agentAccess.view.getCurrentView());
      if (root?.status === "running" && sessionId && root.activeSegmentId) {
        if (
          steerRunningTurn(agentAccess, {
            sessionId,
            runId: root.id,
            segmentId: root.activeSegmentId,
            input,
          })
        ) {
          return true;
        }
      }
      if (!canAcceptChatInput(sessionId, Boolean(send), root?.status ?? "idle")) {
        return false;
      }
      return sendFreshTurn({ sessionId, send, input, runOptions });
    },
    [send, agentAccess],
  );
}

export function useCanSendToAgent(): boolean {
  const agentAccess = useAgent();

  const sessionId = useActiveSessionId();
  const send = agentAccess.view.useAction("send");
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
