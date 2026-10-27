import { useCallback } from "react";
import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { submitCapturedAgentInput } from "@/features/agent/application/input/submitAgentInput";
import { type AgentInput } from "@/features/agent/domain/input";

export {
  agentTextInput,
  type AgentInput,
  type AgentInputPart,
} from "@/features/agent/domain/input";
export { useCanSendToAgent, useChatSend } from "@/features/agent/application/input/chatSend";
export { type AgentInputSettlement } from "@/features/agent/application/input/inputSubmission";

export function useSubmitCapturedAgentInput() {
  const agentAccess = useAgentDependencies();
  return useCallback(
    (sessionId: string, input: AgentInput, signal: AbortSignal) =>
      submitCapturedAgentInput(agentAccess, sessionId, input, signal),
    [agentAccess],
  );
}
