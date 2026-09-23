import { service } from "dougong";
import type { AgentOpenSessions } from "./session";

export interface AgentSessions {
  getActiveSessionId: () => string;
  getLifecycleSnapshot: () => AgentOpenSessions;
  subscribeActiveSessionId: (listener: (sessionId: string) => void) => () => void;
  subscribeLifecycle: (listener: (state: AgentOpenSessions) => void) => () => void;
}

export const AGENT_SESSIONS = service<AgentSessions>("flame.agent.sessions");
