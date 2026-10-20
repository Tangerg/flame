import { createSingletonPort } from "@/lib/ports/singletonPort";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import { type AgentInput } from "@/features/agent/domain/input";
export interface AgentSession {
  send: (input: AgentInput, options?: AgentRunStartOptions) => boolean;
  stop: () => void;
}

interface AgentDefaultSessionPort {
  useDefaultChatSession(): AgentSession;
}

const port = createSingletonPort<AgentDefaultSessionPort>(
  "Agent default session port is not configured",
);

export const configureAgentDefaultSessionPort = port.configure;

export const agentDefaultSession = port.get;
