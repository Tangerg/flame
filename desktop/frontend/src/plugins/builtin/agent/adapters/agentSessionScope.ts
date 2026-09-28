import { navigator } from "@/lib/navigation";
import type { RuntimeServerScope } from "@/plugins/builtin/runtime/public/services";
import { activateAgentSessionStorage } from "./agentSessionStore";

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
