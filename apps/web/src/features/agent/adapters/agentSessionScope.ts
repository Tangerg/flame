import { navigator } from "@/lib/navigation";
import { type RuntimeServerScope } from "@/features/connection/public/services";
import { activateAgentSessionStorage } from "@/features/agent/adapters/agentSessionStore";
export function installAgentSessionScope(
  scope: RuntimeServerScope,
  endpoint: () => string,
): () => void {
  let activeEndpoint = endpoint();
  activateAgentSessionStorage(activeEndpoint);
  return scope.subscribeReplacement(() => {
    const nextEndpoint = endpoint();
    if (nextEndpoint === activeEndpoint) return;
    activeEndpoint = nextEndpoint;
    activateAgentSessionStorage(nextEndpoint);
    navigator().go({ session: "", view: null, dock: null, subagent: null }, { replace: true });
  });
}
