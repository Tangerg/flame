import { type FlameClient } from "@flame/runtime-client";
import {
  HookTrustMutationOwner,
  type HookTrustGateway,
} from "@/features/hooks/application/hookTrust";
function runtimeHookTrustGateway(client: FlameClient): HookTrustGateway {
  return {
    async setProjectTrust(projectRoot, trusted) {
      await client.hooks.setTrust(projectRoot, trusted);
    },
  };
}

export function installHookTrustGateway(runtimeClient: () => FlameClient) {
  const owner = HookTrustMutationOwner.install(runtimeHookTrustGateway(runtimeClient()));
  return {
    replaceRuntimeGeneration: () =>
      owner.replaceRuntimeGeneration(() => runtimeHookTrustGateway(runtimeClient())),
    dispose() {
      owner.dispose();
    },
  };
}
