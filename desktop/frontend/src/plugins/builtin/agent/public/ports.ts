// Most callers reach these through the module functions beside this file, since nothing
// invokes them until every plugin is up. A plugin that READS them during its own setup has
// an ordering requirement, and under a dependency graph that is a Service — the value IS the
// port surface, so declaring the requirement and using it are one act.

import { service } from "dougong";
import type { AgentOpenSessions } from "./session";

export interface AgentSessionPorts {
  activeSessionId: () => string;
  lifecycleSnapshot: () => AgentOpenSessions;
  subscribeActiveSessionId: (listener: (sessionId: string) => void) => () => void;
  subscribeLifecycle: (listener: (state: AgentOpenSessions) => void) => () => void;
}

export const AGENT_SESSION_PORTS = service<AgentSessionPorts>("flame.agent.sessionPorts");
