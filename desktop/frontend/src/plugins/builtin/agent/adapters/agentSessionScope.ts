import { navigator } from "@/lib/navigation";
import type { RuntimeServerScope } from "@/plugins/builtin/runtime/public/services";
import { activateAgentSessionStorage } from "./agentSessionStore";
import { installAbandonedDraftCleanup } from "./abandonedDraftCleanup";

export function installAgentSessionScope(
  scope: RuntimeServerScope,
  endpoint: () => string,
): () => void {
  let activeEndpoint = endpoint();
  activateAgentSessionStorage(activeEndpoint);
  let currentScope: object = {};
  const disposeDraftCleanup = installAbandonedDraftCleanup(() => currentScope);
  const unsubscribe = scope.subscribeReplacement(() => {
    const nextEndpoint = endpoint();
    if (nextEndpoint === activeEndpoint) return;
    activeEndpoint = nextEndpoint;
    // The URL still names the predecessor's Session. Its ownership stays
    // retired until navigation completes, including delayed router updates.
    currentScope = {};
    activateAgentSessionStorage(nextEndpoint);
    navigator().go({ session: "", view: null, dock: null, subagent: null }, { replace: true });
  });
  return () => {
    unsubscribe();
    disposeDraftCleanup();
  };
}
