import { type FlameClient } from "@flame/runtime-client";
import { type HookTrustGateway } from "@/features/hooks/application/hookTrust";

export function createRuntimeHookTrustGateway(client: FlameClient): HookTrustGateway {
  return {
    async setProjectTrust(projectRoot, trusted) {
      await client.hooks.setTrust(projectRoot, trusted);
    },
  };
}
