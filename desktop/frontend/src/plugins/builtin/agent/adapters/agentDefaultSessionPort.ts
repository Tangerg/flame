import type { FlameClient } from "@flame/runtime-contract/client";
import { pickAgentSource } from "@/plugins/sdk";
import { navigator } from "@/lib/navigation";
import { configureAgentDefaultSessionPort } from "../application/ports/defaultSession";
import { useAgentSession } from "./useAgentSession";

export function installAgentDefaultSessionPort(runtimeClient: () => FlameClient): () => void {
  function useDefaultChatSession() {
    const activeSessionId = navigator().use((location) => location.session);
    return useAgentSession(
      runtimeClient,
      () => {
        const source = pickAgentSource();
        if (!source) throw new Error("No agent source registered");
        return source.factory();
      },
      activeSessionId,
    );
  }
  return configureAgentDefaultSessionPort({ useDefaultChatSession });
}
