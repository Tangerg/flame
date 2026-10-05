import { createSingletonPort } from "@/lib/ports/singletonPort";

import type { AgentOpenSessions } from "../session/sessionSelectionModel";

export type { AgentOpenSessions };

export interface AgentSessionStatePort {
  useActiveSessionId(): string;
  getActiveSessionId(): string;
  getLifecycleSnapshot(): AgentOpenSessions;
  subscribeActiveSessionId(onChange: (sessionId: string) => void): () => void;
  subscribeLifecycle(onChange: (snapshot: AgentOpenSessions) => void): () => void;
  selectSession(id: string): void;
  closeSession(id: string): void;
  deleteSession(id: string): void;
  subscribeDeletedSession(listener: (id: string) => void): () => void;
  reconcileSessions(liveIds: string[]): void;
  restoreLastSession(): void;
}

const port = createSingletonPort<AgentSessionStatePort>(
  "Agent session state port is not configured",
);

export const configureAgentSessionStatePort = port.configure;
export const agentSessionState = port.get;
