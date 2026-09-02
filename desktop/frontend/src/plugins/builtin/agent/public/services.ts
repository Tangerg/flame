// What this context PUBLISHES to other plugins, as dougong Services.
//
// A Service is for a capability whose AVAILABILITY has to be ordered: the consumer declares
// `requires`, the Host starts this provider first, and a failure rolls the whole
// installation back. Reach for one exactly when the question is "has the plugin that answers
// this been installed yet" — which, in practice, means a plugin reading another context
// during its own `setup`.
//
// It is NOT the same thing as `application/ports/`, despite both once being called ports.
// Those invert a dependency INSIDE this context: the application declares an interface, this
// context's own adapter installs it, and both ship in the same plugin, so there is no
// ordering question and no graph to resolve. Measured across the tree, none of the twenty
// singleton ports is read by a foreign plugin — turning them into Services would mint twenty
// tokens each provided and required by the same installable.

import { service } from "dougong";
import type { AgentOpenSessions } from "./session";

export interface AgentSessions {
  getActiveSessionId: () => string;
  getLifecycleSnapshot: () => AgentOpenSessions;
  subscribeActiveSessionId: (listener: (sessionId: string) => void) => () => void;
  subscribeLifecycle: (listener: (state: AgentOpenSessions) => void) => () => void;
}

export const AGENT_SESSIONS = service<AgentSessions>("flame.agent.sessions");
