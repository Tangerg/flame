import { type Navigator } from "@/lib/navigation";
import { type RuntimeServerScope } from "@/features/connection/public/services";
import { type AgentSessionStore } from "@/features/agent/adapters/agentSessionStore";
export function installAgentSessionScope(
  navigation: Navigator,
  sessions: AgentSessionStore,
  scope: RuntimeServerScope,
  endpoint: () => string,
): () => void {
  let activeEndpoint = endpoint();
  sessions.activate(activeEndpoint);
  return scope.subscribeReplacement(() => {
    const nextEndpoint = endpoint();
    if (nextEndpoint === activeEndpoint) return;
    activeEndpoint = nextEndpoint;
    sessions.activate(nextEndpoint);
    navigation.go({ session: "", view: null, dock: null, subagent: null }, { replace: true });
  });
}
