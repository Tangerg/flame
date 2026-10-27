import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { useCallback } from "react";
import { resolveAgentRunStartOptions } from "@/features/agent/public/runOptions";
import { type AgentInput } from "@/features/agent/domain/input";

import {
  getActiveSessionId,
  useActiveSessionId,
} from "@/features/agent/application/session/activeSession";
import { useCurrentRootMaterial } from "@/features/agent/application/run/runReadModel";
import { submitAgentInput } from "@/features/agent/application/input/submitAgentInput";

export function useChatSend(): (input: AgentInput) => boolean {
  const agentAccess = useAgentDependencies();

  return useCallback(
    (input: AgentInput) => {
      const sessionId = getActiveSessionId(agentAccess);
      const runOptions = resolveAgentRunStartOptions();
      return Boolean(submitAgentInput(agentAccess, sessionId, input, runOptions));
    },
    [agentAccess],
  );
}

export function useCanSendToAgent(): boolean {
  const agentAccess = useAgentDependencies();

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
